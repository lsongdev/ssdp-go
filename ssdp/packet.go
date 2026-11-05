package ssdp

import (
	"bytes"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
)

// Packet is the common HTTP-over-UDP envelope used by SSDP searches,
// responses, and advertisements. Vendor-specific fields remain in Header.
type Packet struct {
	StartLine string
	// Method and Target are set for requests such as NOTIFY.
	Method string
	Target string
	// StatusCode is set for responses.
	StatusCode int
	Header     http.Header
	Source     *net.UDPAddr
}

// ParsePacket decodes the start line and headers of an SSDP datagram.
// The UDP source address is supplied by the caller, not read from untrusted headers.
//
// Devices are not always strict, so bare LF line endings are accepted, and header
// lines that cannot be parsed are skipped rather than losing the whole packet.
func ParsePacket(data []byte, source *net.UDPAddr) (Packet, error) {
	if len(data) == 0 || len(data) > maxDatagram {
		return Packet{}, errors.New("ssdp: invalid datagram size")
	}
	lines := bytes.Split(data, []byte("\n"))
	packet := Packet{StartLine: string(bytes.TrimSuffix(lines[0], []byte("\r"))), Header: make(http.Header), Source: source}
	if packet.StartLine == "" {
		return Packet{}, errors.New("ssdp: missing start line")
	}
	if err := packet.parseStartLine(); err != nil {
		return Packet{}, err
	}
	for _, raw := range lines[1:] {
		line := bytes.TrimSuffix(raw, []byte("\r"))
		if len(line) == 0 {
			break
		}
		if line[0] == ' ' || line[0] == '\t' {
			continue // obsolete line folding
		}
		name, value, ok := bytes.Cut(line, []byte(":"))
		name = bytes.TrimSpace(name)
		if !ok || len(name) == 0 {
			continue
		}
		packet.Header.Add(string(name), strings.TrimSpace(string(value)))
	}
	return packet, nil
}

func (p *Packet) parseStartLine() error {
	fields := strings.SplitN(p.StartLine, " ", 3)
	if isHTTPVersion(fields[0]) && len(fields) >= 2 {
		code, err := strconv.Atoi(fields[1])
		if err == nil && code >= 100 && code <= 999 {
			p.StatusCode = code
			return nil
		}
	}
	if len(fields) == 3 && fields[0] != "" && fields[1] != "" && isHTTPVersion(fields[2]) {
		p.Method, p.Target = fields[0], fields[1]
		return nil
	}
	return fmt.Errorf("ssdp: unsupported start line %q", p.StartLine)
}

func isHTTPVersion(s string) bool {
	return s == "HTTP/1.1" || s == "HTTP/1.0"
}
