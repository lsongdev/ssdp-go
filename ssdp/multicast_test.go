package ssdp

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/ipv4"
)

// A group outside the SSDP addresses, kept on loopback, so these tests stay off
// the real network.
const testGroup = "239.255.255.97"

func loopback(t *testing.T) net.Interface {
	t.Helper()
	interfaces, err := net.Interfaces()
	if err != nil {
		t.Fatal(err)
	}
	for _, ifi := range interfaces {
		if ifi.Flags&net.FlagLoopback != 0 && ifi.Flags&net.FlagMulticast != 0 {
			return ifi
		}
	}
	t.Skip("no multicast-capable loopback interface")
	return net.Interface{}
}

func freePort(t *testing.T) int {
	t.Helper()
	probe, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer probe.Close()
	return probe.LocalAddr().(*net.UDPAddr).Port
}

func TestMulticastSearchUsesInterfaceAndTTL(t *testing.T) {
	lo := loopback(t)
	group := &net.UDPAddr{IP: net.ParseIP(testGroup), Port: freePort(t)}
	server, err := net.ListenMulticastUDP("udp4", &lo, group)
	if err != nil {
		t.Skipf("cannot join on loopback: %v", err)
	}
	defer server.Close()
	packetConn := ipv4.NewPacketConn(server)
	if err := packetConn.SetControlMessage(ipv4.FlagTTL, true); err != nil {
		t.Skipf("TTL control messages unavailable: %v", err)
	}
	ttl := make(chan int, 8)
	go func() {
		buf := make([]byte, 2048)
		for {
			_, cm, remote, err := packetConn.ReadFrom(buf)
			if err != nil {
				return
			}
			if cm != nil {
				ttl <- cm.TTL
			}
			server.WriteToUDP([]byte(goodResponse), remote.(*net.UDPAddr))
		}
	}()
	client := NewClient(Config{
		Address:    testGroup,
		Port:       group.Port,
		Interfaces: []net.Interface{lo},
		Sends:      1,
		Window:     300 * time.Millisecond,
	})
	results, err := client.Search(context.Background(), "x")
	if err != nil || len(results) != 1 {
		t.Fatalf("results=%+v err=%v", results, err)
	}
	select {
	case got := <-ttl:
		if got != DefaultTTL {
			t.Fatalf("TTL = %d, want %d", got, DefaultTTL)
		}
	default:
		t.Log("platform did not report the TTL")
	}
}

func TestMulticastListenNotifications(t *testing.T) {
	lo := loopback(t)
	port := freePort(t)
	client := NewClient(Config{Address: testGroup, Port: port, Interfaces: []net.Interface{lo}})

	sender, err := net.ListenUDP("udp4", &net.UDPAddr{})
	if err != nil {
		t.Fatal(err)
	}
	defer sender.Close()
	if err := ipv4.NewPacketConn(sender).SetMulticastInterface(&lo); err != nil {
		t.Skipf("cannot send on loopback: %v", err)
	}
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		for {
			select {
			case <-stop:
				return
			case <-time.After(10 * time.Millisecond):
				sender.WriteToUDP([]byte("NOTIFY * HTTP/1.1\r\nNTS: ssdp:alive\r\nUSN: test\r\n\r\n"), &net.UDPAddr{IP: net.ParseIP(testGroup), Port: port})
			}
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	var got Packet
	err = client.ListenNotifications(ctx, func(p Packet) bool { got = p; return true })
	if errors.Is(err, context.DeadlineExceeded) {
		t.Skip("loopback multicast is not delivered on this platform")
	}
	if err != nil || got.Header.Get("USN") != "test" {
		t.Fatalf("packet=%+v err=%v", got, err)
	}
}

func TestMulticastSearchFailsWhenNoInterfaceCanSend(t *testing.T) {
	client := NewClient(Config{
		Address:    testGroup,
		Port:       freePort(t),
		Interfaces: []net.Interface{{Index: 1 << 20, Name: "missing"}},
	})
	_, err := client.Search(context.Background(), "x")
	if err == nil || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("err = %v", err)
	}
}
