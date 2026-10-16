// Package matrix drives a 64x32 HUB75 panel attached to a Raspberry Pi Pico
// running matrix_fw (see ../../matrix_fw.c) over USB serial.
//
// Low level: Open a Display, draw on a Canvas and call Show. High level: a
// Player keeps text or an image on screen, scrolls long text and reconnects
// if the Pico is unplugged.
package matrix

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"go.bug.st/serial"
	"go.bug.st/serial/enumerator"
)

const raspberryPiVID = "2E8A"

// ErrNotFound is returned by FindPort when no Pico serial port is present.
var ErrNotFound = errors.New("matrix: no Raspberry Pi Pico serial port found")

// Display is a connection to the firmware. Its methods are safe for
// concurrent use.
type displayPort interface {
	Read([]byte) (int, error)
	Write([]byte) (int, error)
	Close() error
	ResetInputBuffer() error
	SetReadTimeout(time.Duration) error
}

type Display struct {
	mu   sync.Mutex
	port displayPort
	name string
	buf  []byte
}

type piPort struct {
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	bytes   chan byte
	timeout time.Duration
}

func openPi() (*piPort, error) {
	program := os.Getenv("MATRIX_PI_DRIVER")
	if program == "" {
		program = "matrix-pi"
	}
	cmd := exec.Command(program)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("matrix: start Raspberry Pi driver: %w", err)
	}
	p := &piPort{cmd: cmd, stdin: stdin, bytes: make(chan byte, 8192)}
	go func() {
		r := bufio.NewReader(stdout)
		for {
			b, err := r.ReadByte()
			if err != nil {
				close(p.bytes)
				return
			}
			p.bytes <- b
		}
	}()
	return p, nil
}

func (p *piPort) Write(b []byte) (int, error) { return p.stdin.Write(b) }
func (p *piPort) Read(b []byte) (int, error) {
	var timer <-chan time.Time
	if p.timeout > 0 {
		t := time.NewTimer(p.timeout)
		defer t.Stop()
		timer = t.C
	}
	select {
	case v, ok := <-p.bytes:
		if !ok {
			return 0, io.EOF
		}
		b[0] = v
		return 1, nil
	case <-timer:
		return 0, nil
	}
}
func (p *piPort) SetReadTimeout(d time.Duration) error { p.timeout = d; return nil }
func (p *piPort) ResetInputBuffer() error {
	for {
		select {
		case _, ok := <-p.bytes:
			if !ok {
				return io.EOF
			}
		default:
			return nil
		}
	}
}
func (p *piPort) Close() error {
	_ = p.stdin.Close()
	if p.cmd.Process != nil {
		_ = p.cmd.Process.Kill()
	}
	return p.cmd.Wait()
}

// FindPort returns the first serial port belonging to a Raspberry Pi Pico.
func FindPort() (string, error) {
	ports, err := enumerator.GetDetailedPortsList()
	if err != nil {
		return "", err
	}
	for _, p := range ports {
		if p.IsUSB && strings.EqualFold(p.VID, raspberryPiVID) {
			return p.Name, nil
		}
	}
	return "", ErrNotFound
}

// Open connects to the firmware on the named serial port, or finds the Pico
// automatically when name is empty.
func Open(name string) (*Display, error) {
	if name == "" {
		var err error
		if name, err = FindPort(); err != nil {
			return nil, err
		}
	}
	var port displayPort
	var err error
	if name == "pi" {
		port, err = openPi()
	} else {
		// The baud rate is ignored by USB CDC, but must not be 1200: that asks
		// the Pico to reboot into its bootloader.
		port, err = serial.Open(name, &serial.Mode{BaudRate: 115200})
	}
	if err != nil {
		return nil, fmt.Errorf("matrix: open %s: %w", name, err)
	}
	if err := port.SetReadTimeout(50 * time.Millisecond); err != nil {
		port.Close()
		return nil, err
	}
	d := &Display{port: port, name: name, buf: make([]byte, 1+Width*Height*2)}
	if err := d.handshake(); err != nil {
		port.Close()
		return nil, err
	}
	return d, nil
}

// Port returns the serial port name.
func (d *Display) Port() string { return d.name }

// Close releases the serial port. The panel keeps showing the last frame.
func (d *Display) Close() error { return d.port.Close() }

// Show sends the canvas to the panel.
func (d *Display) Show(c *Canvas) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.buf[0] = 'F'
	c.encode(d.buf[1:])
	return d.command(d.buf)
}

// SetBrightness sets the panel brightness, 0-255.
func (d *Display) SetBrightness(b uint8) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.command([]byte{'B', b})
}

func (d *Display) command(cmd []byte) error {
	if _, err := d.port.Write(cmd); err != nil {
		return err
	}
	line, err := d.readLine(time.Second)
	if err != nil {
		return err
	}
	if line != "K" {
		return fmt.Errorf("matrix: firmware replied %q", line)
	}
	return nil
}

func (d *Display) handshake() error {
	d.port.ResetInputBuffer()
	// A previous client may have died mid-frame; the firmware gives up on a
	// partial command after 500ms, so wait that out before pinging.
	time.Sleep(600 * time.Millisecond)
	d.port.ResetInputBuffer()
	if _, err := d.port.Write([]byte{'P'}); err != nil {
		return err
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		line, err := d.readLine(time.Until(deadline))
		if err != nil {
			break
		}
		if strings.HasPrefix(line, "PICO-MATRIX ") {
			return nil
		}
	}
	return fmt.Errorf("matrix: %s display driver did not answer", d.name)
}

func (d *Display) readLine(timeout time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)
	var line []byte
	b := make([]byte, 1)
	for time.Now().Before(deadline) {
		n, err := d.port.Read(b)
		if err != nil {
			return "", err
		}
		if n == 0 {
			continue
		}
		if b[0] == '\n' {
			return strings.TrimRight(string(line), "\r"), nil
		}
		line = append(line, b[0])
	}
	return "", errors.New("matrix: timed out waiting for firmware")
}
