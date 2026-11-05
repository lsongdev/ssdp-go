package ssdp

import (
	"strings"
	"testing"
)

func TestRequestEncodingIsStable(t *testing.T) {
	want := "M-SEARCH * HTTP/1.1\r\nHOST: h:1\r\nMAN: \"ssdp:discover\"\r\nMX: 3\r\nST: x\r\nA-VENDOR: 1\r\nUSER-AGENT: u\r\n\r\n"
	for i := 0; i < 50; i++ {
		r := NewRequest(MethodSearch, "*")
		r.Host = "h:1"
		r.AddHeader("user-agent", "u")
		r.AddHeader("ST", "x")
		r.AddHeader("a-vendor", "1")
		r.AddHeader("mx", "3")
		if got := r.String(); got != want {
			t.Fatalf("got %q", got)
		}
	}
}

func TestOnlySearchRequestsHaveMAN(t *testing.T) {
	r := NewRequest(MethodNotify, "*")
	if strings.Contains(strings.ToUpper(r.String()), "MAN:") {
		t.Fatalf("NOTIFY has MAN: %q", r.String())
	}
}

func TestRequestValidate(t *testing.T) {
	valid := func() *Request {
		r := NewRequest(MethodSearch, "*")
		r.Host = "239.255.255.250:1900"
		return r
	}
	if err := valid().Validate(); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*Request){
		"method":     func(r *Request) { r.Method = "M\r\nX: 1" },
		"method-sp":  func(r *Request) { r.Method = "M X" },
		"path":       func(r *Request) { r.Path = "* HTTP/1.1\r\nX: 1\r\n" },
		"path-sp":    func(r *Request) { r.Path = "a b" },
		"host":       func(r *Request) { r.Host = "h\r\nX: 1" },
		"value":      func(r *Request) { r.AddHeader("ST", "x\r\nX: 1") },
		"value-lf":   func(r *Request) { r.AddHeader("ST", "x\nX: 1") },
		"name":       func(r *Request) { r.AddHeader("X\r\nY", "1") },
		"name-colon": func(r *Request) { r.AddHeader("X:Y", "1") },
		"name-empty": func(r *Request) { r.AddHeader("", "1") },
	} {
		r := valid()
		mutate(r)
		if r.Validate() == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestRequestAddress(t *testing.T) {
	r := NewRequest(MethodSearch, "*")
	r.Host = "239.255.255.250:1900"
	address, err := r.Address()
	if err != nil || address.Port != 1900 {
		t.Fatalf("address=%v err=%v", address, err)
	}
	r.Host = "bad host:::"
	if _, err := r.Address(); err == nil {
		t.Fatal("expected error")
	}
}
