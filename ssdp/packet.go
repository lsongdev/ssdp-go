package ssdp

import (
	"bytes"
	"fmt"
	"net"
	"net/http"
	"strings"
)

// Packet is the common HTTP-over-UDP envelope used by SSDP searches,
// responses, and advertisements. Vendor-specific fields remain in Header.
type Packet struct {
	StartLine string
	Header    http.Header
	Source    *net.UDPAddr
}

// ParsePacket decodes the start line and headers of an SSDP datagram.
// The UDP source address is supplied by the caller, not read from untrusted headers.
func ParsePacket(data []byte, source *net.UDPAddr) (Packet, error) {
	if len(data) == 0 || len(data) > 65507 {
		return Packet{}, fmt.Errorf("ssdp: invalid datagram size")
	}
	head, _, _ := bytes.Cut(data, []byte("\r\n\r\n"))
	lines := bytes.Split(head, []byte("\r\n"))
	if len(lines) == 0 || len(lines[0]) == 0 {
		return Packet{}, fmt.Errorf("ssdp: missing start line")
	}
	start := string(lines[0])
	if !strings.HasSuffix(start, " HTTP/1.1") && !strings.HasPrefix(start, "HTTP/1.1 ") {
		return Packet{}, fmt.Errorf("ssdp: unsupported start line %q", start)
	}
	packet := Packet{StartLine: start, Header: make(http.Header), Source: source}
	for _, raw := range lines[1:] {
		if len(raw) == 0 {
			break
		}
		name, value, ok := bytes.Cut(raw, []byte(":"))
		if !ok || len(name) == 0 {
			return Packet{}, fmt.Errorf("ssdp: malformed header")
		}
		packet.Header.Add(string(name), strings.TrimSpace(string(value)))
	}
	return packet, nil
}
