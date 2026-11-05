package ssdp

import (
	"errors"
	"fmt"
	"net"
	"sort"
	"strings"
)

// Request is an SSDP request message.
type Request struct {
	Method  string
	Path    string
	Host    string
	Headers map[string]string
}

// NewRequest creates a request. M-SEARCH requests include the required MAN header.
func NewRequest(method, path string) *Request {
	req := &Request{
		Method:  method,
		Path:    path,
		Headers: map[string]string{},
	}
	if method == MethodSearch {
		req.AddHeader("MAN", `"ssdp:discover"`)
	}
	return req
}

// AddHeader sets a header. Header names are case-insensitive, so names are
// stored in upper case, the form used by the SSDP specification.
func (p *Request) AddHeader(name, value string) {
	p.Headers[strings.ToUpper(name)] = value
}

// Validate reports whether the request can be encoded safely. Call it before
// sending values from untrusted sources: line breaks would otherwise inject
// headers into the message.
func (p *Request) Validate() error {
	for _, field := range []string{p.Method, p.Path, p.Host} {
		if strings.ContainsAny(field, "\r\n\x00") {
			return fmt.Errorf("ssdp: invalid request field %q", field)
		}
	}
	if strings.ContainsAny(p.Method, " ") || strings.ContainsAny(p.Path, " ") {
		return errors.New("ssdp: request method and path cannot contain spaces")
	}
	for name, value := range p.Headers {
		if name == "" || strings.ContainsAny(name, ": \t\r\n\x00") {
			return fmt.Errorf("ssdp: invalid header name %q", name)
		}
		if strings.ContainsAny(value, "\r\n\x00") {
			return fmt.Errorf("ssdp: invalid value for header %s", name)
		}
	}
	return nil
}

// String encodes the request. HOST comes first, followed by the headers in a
// fixed order, so that a request always has the same encoding.
func (p *Request) String() string {
	names := make([]string, 0, len(p.Headers))
	for name := range p.Headers {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool {
		ri, rj := headerRank(names[i]), headerRank(names[j])
		if ri != rj {
			return ri < rj
		}
		return names[i] < names[j]
	})
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s HTTP/1.1\r\nHOST: %s\r\n", p.Method, p.Path, p.Host)
	for _, name := range names {
		fmt.Fprintf(&b, "%s: %s\r\n", name, p.Headers[name])
	}
	b.WriteString("\r\n")
	return b.String()
}

// headerRank orders the standard search headers as the specification lists them.
func headerRank(name string) int {
	for i, known := range []string{"MAN", "MX", "ST"} {
		if strings.EqualFold(name, known) {
			return i
		}
	}
	return 3
}

func (p *Request) Bytes() []byte {
	return []byte(p.String())
}

// Address resolves Host to a UDP address.
func (p *Request) Address() (*net.UDPAddr, error) {
	address, err := net.ResolveUDPAddr("udp4", p.Host)
	if err != nil {
		return nil, fmt.Errorf("ssdp: resolve host: %w", err)
	}
	return address, nil
}
