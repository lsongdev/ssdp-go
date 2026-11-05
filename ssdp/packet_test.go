package ssdp

import (
	"net"
	"testing"
)

func TestParsePacketProfiles(t *testing.T) {
	source := &net.UDPAddr{IP: net.ParseIP("192.168.1.5"), Port: 1900}
	for _, tc := range []struct{ name, wire, start, usn string }{
		{"upnp", "NOTIFY * HTTP/1.1\r\nHOST: 239.255.255.250:1900\r\nNT: upnp:rootdevice\r\nNTS: ssdp:alive\r\nUSN: uuid:test::upnp:rootdevice\r\n\r\n", "NOTIFY * HTTP/1.1", "uuid:test::upnp:rootdevice"},
		{"bambu", "NOTIFY * HTTP/1.1\r\nHOST: 239.255.255.250:1900\r\nNT: urn:bambulab-com:device:3dprinter:1\r\nUSN: SERIAL\r\nLocation: 192.168.1.5\r\n\r\n", "NOTIFY * HTTP/1.1", "SERIAL"},
		{"yeelight", "HTTP/1.1 200 OK\r\nLocation: yeelight://192.168.1.5:55443\r\nid: bulb\r\n\r\n", "HTTP/1.1 200 OK", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, e := ParsePacket([]byte(tc.wire), source)
			if e != nil || p.StartLine != tc.start || p.Header.Get("USN") != tc.usn || p.Source != source {
				t.Fatalf("packet=%+v err=%v", p, e)
			}
		})
	}
}

func TestParsePacketStartLine(t *testing.T) {
	for _, tc := range []struct {
		wire, method, target string
		status               int
	}{
		{"HTTP/1.1 200 OK\r\n\r\n", "", "", 200},
		{"HTTP/1.1 200\r\n\r\n", "", "", 200},
		{"HTTP/1.0 200 OK\r\n\r\n", "", "", 200},
		{"HTTP/1.1 404 Not Found\r\n\r\n", "", "", 404},
		{"HTTP/1.1 200 OK", "", "", 200},
		{"HTTP/1.1 200 OK\n\n", "", "", 200},
		{"NOTIFY * HTTP/1.1\r\n\r\n", "NOTIFY", "*", 0},
		{"M-SEARCH * HTTP/1.1\r\n\r\n", "M-SEARCH", "*", 0},
	} {
		p, err := ParsePacket([]byte(tc.wire), nil)
		if err != nil || p.Method != tc.method || p.Target != tc.target || p.StatusCode != tc.status {
			t.Errorf("%q: packet=%+v err=%v", tc.wire, p, err)
		}
	}
	for _, wire := range []string{
		"", "\r\n", "garbage\r\n", "http/1.1 200 ok\r\n\r\n", "HTTP/1.1\r\n\r\n", "HTTP/1.1 abc\r\n\r\n",
		"HTTP/1.1 99 Low\r\n\r\n", "HTTP/2 200 OK\r\n\r\n", "NOTIFY *\r\n\r\n", " * HTTP/1.1\r\n\r\n", "NOTIFY * HTTP/2\r\n\r\n",
	} {
		if p, err := ParsePacket([]byte(wire), nil); err == nil {
			t.Errorf("%q accepted: %+v", wire, p)
		}
	}
}

func TestParsePacketHeaders(t *testing.T) {
	wire := "HTTP/1.1 200 OK\nbad line\r\n: no name\r\nfw_ver: 18\r\nSpaced : value\r\n folded: continuation\r\nLOCATION:  a:b:c  \r\nExt:\r\n\r\nBody: ignored\r\n"
	p, err := ParsePacket([]byte(wire), nil)
	if err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{"FW_VER": "18", "Spaced": "value", "Location": "a:b:c", "Body": "", "folded": ""} {
		if got := p.Header.Get(name); got != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}
	if _, present := p.Header["Ext"]; !present {
		t.Error("empty header dropped")
	}
	if len(p.Header) != 4 {
		t.Errorf("headers = %v", p.Header)
	}
}
