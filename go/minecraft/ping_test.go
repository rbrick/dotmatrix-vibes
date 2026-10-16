package minecraft

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"net"
	"strconv"
	"testing"
	"time"
)

// The examples from the protocol documentation (wiki.vg / minecraft.wiki).
func TestVarInt(t *testing.T) {
	for _, c := range []struct {
		n   int32
		hex string
	}{
		{0, "00"}, {1, "01"}, {2, "02"}, {127, "7f"}, {128, "8001"}, {255, "ff01"},
		{25565, "ddc701"}, {2097151, "ffff7f"}, {2147483647, "ffffffff07"},
		{-1, "ffffffff0f"}, {-2147483648, "8080808008"},
	} {
		got := appendVarInt(nil, c.n)
		if hex.EncodeToString(got) != c.hex {
			t.Errorf("appendVarInt(%d) = %x, want %s", c.n, got, c.hex)
		}
		back, err := readVarInt(bytes.NewReader(got))
		if err != nil || back != c.n {
			t.Errorf("readVarInt(%s) = %d, %v", c.hex, back, err)
		}
	}
	if _, err := readVarInt(bytes.NewReader([]byte{0xff, 0xff, 0xff, 0xff, 0x7f})); err == nil {
		t.Error("a VarInt over 32 bits should be rejected")
	}
}

const fakeStatus = `{"version":{"name":"§f1.21.4","protocol":769},"players":{"max":100,"online":7},` +
	`"description":{"text":"§aHello ","extra":["§lworld",{"text":"!"}]}}`

// fakeServer answers one status ping like a vanilla server and records the
// handshake.
func fakeServer(t *testing.T, pong bool) (addr string, handshake chan []byte) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	handshake = make(chan []byte, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		r := bufio.NewReader(conn)
		id, hs, err := readPacket(r)
		if err != nil || id != 0 {
			return
		}
		handshake <- hs
		if id, _, err := readPacket(r); err != nil || id != 0 {
			return
		}
		conn.Write(packet(0x00, appendString(nil, fakeStatus)))
		if id, payload, err := readPacket(r); err == nil && id == 1 && pong {
			conn.Write(packet(0x01, payload))
		}
	}()
	return ln.Addr().String(), handshake
}

func TestPing(t *testing.T) {
	addr, handshake := fakeServer(t, true)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	st, err := Ping(ctx, addr)
	if err != nil {
		t.Fatal(err)
	}
	if st.Players.Online != 7 || st.Players.Max != 100 || st.VersionName() != "1.21.4" {
		t.Errorf("status = %+v", st)
	}
	if st.MOTD() != "Hello world!" {
		t.Errorf("MOTD = %q", st.MOTD())
	}
	if st.Latency <= 0 {
		t.Error("no latency measured")
	}

	// Handshake: protocol -1, host, port, next state 1 (status).
	hs := bytes.NewReader(<-handshake)
	if v, _ := readVarInt(hs); v != -1 {
		t.Errorf("protocol version %d", v)
	}
	n, _ := readVarInt(hs)
	host := make([]byte, n)
	hs.Read(host)
	var port uint16
	binary.Read(hs, binary.BigEndian, &port)
	next, _ := readVarInt(hs)
	_, wantPort, _ := net.SplitHostPort(addr)
	if string(host) != "127.0.0.1" || strconv.Itoa(int(port)) != wantPort || next != 1 {
		t.Errorf("handshake: host %q port %d next %d", host, port, next)
	}
}

func TestPingWithoutPong(t *testing.T) {
	addr, _ := fakeServer(t, false)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	st, err := Ping(ctx, addr)
	if err != nil || st.Players.Online != 7 || st.Latency != 0 {
		t.Errorf("status %+v, err %v", st, err)
	}
}

func TestPingErrors(t *testing.T) {
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	addr := ln.Addr().String()
	ln.Close() // nothing listening any more
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, err := Ping(ctx, addr); err == nil {
		t.Error("closed port should fail")
	}
	if _, err := Ping(ctx, "localhost:99999"); err == nil {
		t.Error("bad port should fail")
	}
	if _, err := Ping(ctx, ""); err == nil {
		t.Error("empty address should fail")
	}
}
