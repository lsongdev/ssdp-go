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
	if _, err := ParsePacket([]byte("garbage\r\n"), source); err == nil {
		t.Fatal("accepted bad start line")
	}
}
