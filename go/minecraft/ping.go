// Package minecraft pings Minecraft Java Edition servers with the Server List
// Ping protocol (the one the multiplayer screen uses) and shows the result
// as a matrix.Scene.
//
// The protocol code is based on https://gist.github.com/rbrick/f390b19d0287a4ae01d56b6ce57aa764,
// with VarInts encoded by encoding/binary.
package minecraft

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"strconv"
	"strings"
	"time"
)

// DefaultPort is used when an address has no port and no SRV record.
const DefaultPort = 25565

// Status is what a server reports about itself.
type Status struct {
	Version struct {
		Name     string `json:"name"`
		Protocol int32  `json:"protocol"`
	} `json:"version"`
	Players struct {
		Max    int `json:"max"`
		Online int `json:"online"`
		Sample []struct {
			Name string `json:"name"`
			ID   string `json:"id"`
		} `json:"sample,omitempty"`
	} `json:"players"`
	Description json.RawMessage `json:"description"` // a string or a chat component
	Favicon     string          `json:"favicon"`     // "data:image/png;base64,..."

	Latency time.Duration `json:"-"` // round trip of the ping packet
}

// appendVarInt appends n as a Minecraft VarInt: LEB128 of the 32-bit two's
// complement value, so negative numbers take 5 bytes. That is
// binary.AppendUvarint once n is widened as unsigned (binary.AppendVarint
// would zigzag-encode it instead, which Minecraft doesn't use).
func appendVarInt(b []byte, n int32) []byte {
	return binary.AppendUvarint(b, uint64(uint32(n)))
}

// readVarInt reads a VarInt of at most 5 bytes.
func readVarInt(r io.ByteReader) (int32, error) {
	v, err := binary.ReadUvarint(r)
	if err != nil {
		return 0, err
	}
	if v > math.MaxUint32 {
		return 0, errors.New("minecraft: VarInt too big")
	}
	return int32(uint32(v)), nil
}

func appendString(b []byte, s string) []byte {
	return append(appendVarInt(b, int32(len(s))), s...)
}

// packet frames a packet: VarInt length (of id and data), VarInt id, data.
func packet(id int32, data []byte) []byte {
	body := append(appendVarInt(nil, id), data...)
	return append(appendVarInt(nil, int32(len(body))), body...)
}

// maxPacket bounds what we'll read; status responses with a favicon are a
// few tens of kilobytes.
const maxPacket = 1 << 21

// readPacket reads one framed packet and returns its id and data.
func readPacket(r *bufio.Reader) (int32, []byte, error) {
	n, err := readVarInt(r)
	if err != nil {
		return 0, nil, err
	}
	if n <= 0 || n > maxPacket {
		return 0, nil, fmt.Errorf("minecraft: bad packet length %d", n)
	}
	body := make([]byte, n)
	if _, err := io.ReadFull(r, body); err != nil {
		return 0, nil, err
	}
	br := bytes.NewReader(body)
	id, err := readVarInt(br)
	if err != nil {
		return 0, nil, err
	}
	return id, body[len(body)-br.Len():], nil
}

// resolve turns an address as typed into a host and port to dial. Without
// a port it looks for a _minecraft._tcp SRV record, as the game does.
func resolve(ctx context.Context, addr string) (host string, port int, err error) {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return "", 0, errors.New("minecraft: empty address")
	}
	if h, p, err := net.SplitHostPort(addr); err == nil {
		n, err := strconv.Atoi(p)
		if err != nil || n < 1 || n > 65535 {
			return "", 0, fmt.Errorf("minecraft: bad port %q", p)
		}
		return h, n, nil
	}
	if _, srvs, err := net.DefaultResolver.LookupSRV(ctx, "minecraft", "tcp", addr); err == nil && len(srvs) > 0 {
		return strings.TrimSuffix(srvs[0].Target, "."), int(srvs[0].Port), nil
	}
	return addr, DefaultPort, nil
}

// Ping asks the server at addr ("host", "host:port" or an IP) for its
// status. The context bounds the whole exchange.
func Ping(ctx context.Context, addr string) (*Status, error) {
	host, port, err := resolve(ctx, addr)
	if err != nil {
		return nil, err
	}
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", net.JoinHostPort(host, strconv.Itoa(port)))
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	if deadline, ok := ctx.Deadline(); ok {
		conn.SetDeadline(deadline)
	}

	// Handshake (protocol version -1: "any", next state 1: status), then
	// the status request, sent together.
	hs := appendVarInt(nil, -1)
	hs = appendString(hs, host)
	hs = binary.BigEndian.AppendUint16(hs, uint16(port))
	hs = appendVarInt(hs, 1)
	if _, err := conn.Write(append(packet(0x00, hs), packet(0x00, nil)...)); err != nil {
		return nil, err
	}

	r := bufio.NewReader(conn)
	id, data, err := readPacket(r)
	if err != nil {
		return nil, fmt.Errorf("minecraft: reading status: %w", err)
	}
	if id != 0x00 {
		return nil, fmt.Errorf("minecraft: unexpected packet 0x%02x", id)
	}
	dr := bytes.NewReader(data)
	n, err := readVarInt(dr)
	if err != nil || int(n) != dr.Len() {
		return nil, errors.New("minecraft: malformed status string")
	}
	var st Status
	if err := json.Unmarshal(data[len(data)-dr.Len():], &st); err != nil {
		return nil, fmt.Errorf("minecraft: status JSON: %w", err)
	}

	// Ping/pong for latency. Some servers hang up instead; that's fine.
	start := time.Now()
	payload := binary.BigEndian.AppendUint64(nil, uint64(start.UnixMilli()))
	if _, err := conn.Write(packet(0x01, payload)); err == nil {
		if id, data, err := readPacket(r); err == nil && id == 0x01 && bytes.Equal(data, payload) {
			st.Latency = time.Since(start)
		}
	}
	return &st, nil
}

// VersionName returns the server's version text without formatting codes;
// servers often use it for a message such as "Requires MC 1.8 / 1.21".
func (s *Status) VersionName() string { return stripCodes(s.Version.Name) }

// MOTD returns the server description as plain text, without formatting
// codes.
func (s *Status) MOTD() string {
	var text string
	if err := json.Unmarshal(s.Description, &text); err != nil {
		var comp chatComponent
		if json.Unmarshal(s.Description, &comp) == nil {
			text = comp.plain()
		}
	}
	return stripCodes(text)
}

type chatComponent struct {
	Text  string          `json:"text"`
	Extra []chatComponent `json:"extra"`
}

// UnmarshalJSON accepts plain strings inside "extra" too.
func (c *chatComponent) UnmarshalJSON(b []byte) error {
	var s string
	if json.Unmarshal(b, &s) == nil {
		c.Text = s
		return nil
	}
	type plain chatComponent
	return json.Unmarshal(b, (*plain)(c))
}

func (c chatComponent) plain() string {
	var b strings.Builder
	b.WriteString(c.Text)
	for _, e := range c.Extra {
		b.WriteString(e.plain())
	}
	return b.String()
}

// stripCodes removes § formatting codes such as §a or §l.
func stripCodes(s string) string {
	var b strings.Builder
	skip := false
	for _, r := range s {
		switch {
		case skip:
			skip = false
		case r == '§':
			skip = true
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}
