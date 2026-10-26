//go:build linux

package gamepad

import (
	"context"
	"encoding/binary"
	"log"
	"path/filepath"
	"sync"
	"time"

	"golang.org/x/sys/unix"
)

const (
	evKey = 0x01
	evAbs = 0x03

	btnSouth = 304 // Xbox A, Switch B
	btnEast  = 305 // Xbox B, Switch A
	btnNorth = 307 // Xbox Y, Switch X
	btnWest  = 308 // Xbox X, Switch Y
	btnStart = 315 // Start/Menu/+

	btnDpadUp    = 544
	btnDpadDown  = 545
	btnDpadLeft  = 546
	btnDpadRight = 547

	absHatX = 16
)

// input_event is 24 bytes on 64-bit Raspberry Pi OS.
const inputEventSize = 24

type watcher struct {
	ctx    context.Context
	action func(Action)
	mu     sync.Mutex
	open   map[string]struct{}
}

func listen(ctx context.Context, action func(Action)) {
	w := &watcher{ctx: ctx, action: action, open: make(map[string]struct{})}
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		w.scan()
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (w *watcher) scan() {
	paths, _ := filepath.Glob("/dev/input/event*")
	for _, path := range paths {
		w.mu.Lock()
		_, exists := w.open[path]
		if !exists {
			w.open[path] = struct{}{}
		}
		w.mu.Unlock()
		if !exists {
			go w.read(path)
		}
	}
}

func (w *watcher) read(path string) {
	defer func() {
		w.mu.Lock()
		delete(w.open, path)
		w.mu.Unlock()
	}()
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_NONBLOCK, 0)
	if err != nil {
		return
	}
	defer unix.Close(fd)
	poll := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
	buf := make([]byte, inputEventSize*16)
	for {
		select {
		case <-w.ctx.Done():
			return
		default:
		}
		n, err := unix.Poll(poll, 1000)
		if err != nil && err != unix.EINTR {
			return
		}
		if n == 0 {
			continue
		}
		if poll[0].Revents&(unix.POLLERR|unix.POLLHUP|unix.POLLNVAL) != 0 {
			return
		}
		n, err = unix.Read(fd, buf)
		if err == unix.EAGAIN || err == unix.EINTR {
			continue
		}
		if err != nil || n == 0 {
			return
		}
		for off := 0; off+inputEventSize <= n; off += inputEventSize {
			e := buf[off : off+inputEventSize]
			typ := binary.LittleEndian.Uint16(e[16:18])
			code := binary.LittleEndian.Uint16(e[18:20])
			value := int32(binary.LittleEndian.Uint32(e[20:24]))
			if a, ok := translate(typ, code, value); ok {
				log.Printf("gamepad %s: %v", filepath.Base(path), a)
				w.action(a)
			}
		}
	}
}

func translate(typ, code uint16, value int32) (Action, bool) {
	if typ == evAbs && code == absHatX {
		switch {
		case value < 0:
			return Left, true
		case value > 0:
			return Right, true
		}
	}
	if typ != evKey || value != 1 { // act once on press, not repeat/release
		return 0, false
	}
	switch code {
	case btnDpadLeft:
		return Left, true
	case btnDpadRight:
		return Right, true
	case btnSouth, btnEast, btnNorth, btnWest, btnDpadDown:
		return Select, true
	case btnStart, btnDpadUp:
		return Start, true
	}
	return 0, false
}
