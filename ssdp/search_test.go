package ssdp

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const goodResponse = "HTTP/1.1 200 OK\r\nLocation: yeelight://127.0.0.1:55443\r\nid: bulb\r\n\r\n"

// responder replies to each datagram with the given messages and counts requests.
type responder struct {
	config   Config
	requests atomic.Int32
	last     atomic.Value
}

func newResponder(t *testing.T, replies ...string) *responder {
	t.Helper()
	server, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { server.Close() })
	r := &responder{config: Config{Port: server.LocalAddr().(*net.UDPAddr).Port, Address: "127.0.0.1"}}
	go func() {
		buf := make([]byte, 2048)
		for {
			n, remote, err := server.ReadFromUDP(buf)
			if err != nil {
				return
			}
			r.last.Store(string(buf[:n]))
			r.requests.Add(1)
			for _, reply := range replies {
				server.WriteToUDP([]byte(reply), remote)
			}
		}
	}()
	return r
}

func TestSearchDeadlineEndsSearchWithoutError(t *testing.T) {
	r := newResponder(t, goodResponse)
	client := NewClient(r.config)
	for i := 0; i < 100; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
		results, err := client.Search(ctx, "wifi_bulb")
		cancel()
		if err != nil {
			t.Fatalf("run %d: err = %v", i, err)
		}
		if len(results) == 1 && results[0].Header.Get("Location") != "yeelight://127.0.0.1:55443" {
			t.Fatalf("results = %+v", results)
		}
	}
	if got := r.last.Load().(string); !strings.Contains(got, "ST: wifi_bulb\r\n") {
		t.Fatalf("request = %q", got)
	}
}

func TestSearchWindowEndsSearch(t *testing.T) {
	r := newResponder(t, goodResponse)
	r.config.Window = 50 * time.Millisecond
	start := time.Now()
	results, err := NewClient(r.config).Search(context.Background(), "wifi_bulb")
	if err != nil || len(results) != 1 {
		t.Fatalf("results=%+v err=%v", results, err)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("took %v", elapsed)
	}
}

func TestSearchCancelReturnsResponsesAndError(t *testing.T) {
	r := newResponder(t, goodResponse)
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(50*time.Millisecond, cancel)
	results, err := NewClient(r.config).Search(ctx, "wifi_bulb")
	if !errors.Is(err, context.Canceled) || len(results) != 1 {
		t.Fatalf("results=%+v err=%v", results, err)
	}
}

func TestSearchEndedContext(t *testing.T) {
	r := newResponder(t, goodResponse)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := NewClient(r.config).Search(ctx, ""); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v", err)
	}
	if n := r.requests.Load(); n != 0 {
		t.Fatalf("sent %d requests", n)
	}
}

func TestSearchRetransmitsAndDeduplicates(t *testing.T) {
	r := newResponder(t,
		goodResponse, goodResponse,
		"HTTP/1.1 200\r\nLocation: no-reason-phrase\r\n\r\n",
		"HTTP/1.1 200 OK\nLocation: lf-only\n\n",
		"HTTP/1.1 404 Not Found\r\nLocation: not-found\r\n\r\n",
		"NOTIFY * HTTP/1.1\r\nLocation: notify\r\n\r\n",
		"garbage",
	)
	r.config.Window = 5 * sendInterval
	results, err := NewClient(r.config).Search(context.Background(), "wifi_bulb")
	if err != nil {
		t.Fatal(err)
	}
	if n := r.requests.Load(); n != DefaultSends {
		t.Fatalf("sent %d requests, want %d", n, DefaultSends)
	}
	var locations []string
	for _, p := range results {
		locations = append(locations, p.Header.Get("Location"))
	}
	if got := strings.Join(locations, ","); got != "yeelight://127.0.0.1:55443,no-reason-phrase,lf-only" {
		t.Fatalf("locations = %s", got)
	}
}

func TestSearchFuncStopsEarly(t *testing.T) {
	r := newResponder(t, goodResponse)
	start := time.Now()
	calls := 0
	err := NewClient(r.config).SearchFunc(context.Background(), "wifi_bulb", func(Packet) bool {
		calls++
		return true
	})
	if err != nil || calls != 1 {
		t.Fatalf("calls=%d err=%v", calls, err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("took %v", elapsed)
	}
	if err := NewClient(r.config).SearchFunc(context.Background(), "", nil); err == nil {
		t.Fatal("accepted nil callback")
	}
}

func TestSearchRejectsInvalidInput(t *testing.T) {
	r := newResponder(t)
	client := NewClient(r.config)
	if _, err := client.Search(context.Background(), "x\r\nEVIL: 1"); err == nil {
		t.Fatal("accepted header injection in search target")
	}
	config := r.config
	config.Headers = map[string]string{"USER-AGENT": "a\r\nEVIL: 1"}
	if _, err := NewClient(config).Search(context.Background(), "x"); err == nil {
		t.Fatal("accepted header injection in header value")
	}
	if n := r.requests.Load(); n != 0 {
		t.Fatalf("sent %d requests", n)
	}
	for _, address := range []string{"not-an-ip", "::1"} {
		if _, err := NewClient(Config{Address: address}).Search(context.Background(), "x"); err == nil {
			t.Errorf("accepted address %q", address)
		}
	}
}

func TestSearchRequestWireFormat(t *testing.T) {
	r := newResponder(t)
	r.config.MX = 2
	r.config.Sends = 1
	r.config.Window = 20 * time.Millisecond
	r.config.Headers = map[string]string{"user-agent": "os/1 UPnP/2.0 test/1", "MAN": "overridden"}
	if _, err := NewClient(r.config).Search(context.Background(), ""); err != nil {
		t.Fatal(err)
	}
	want := fmt.Sprintf("M-SEARCH * HTTP/1.1\r\nHOST: 127.0.0.1:%d\r\nMAN: \"ssdp:discover\"\r\nMX: 2\r\nST: ssdp:all\r\nUSER-AGENT: os/1 UPnP/2.0 test/1\r\n\r\n", r.config.Port)
	if got := r.last.Load().(string); got != want {
		t.Fatalf("got  %q\nwant %q", got, want)
	}
}

func TestDefaults(t *testing.T) {
	config := NewClient(Config{}).config
	if config.Address != "239.255.255.250" || config.Port != 1900 || config.MX != 3 || config.TTL != 2 || config.Sends != 3 || config.Window != 5*time.Second {
		t.Fatalf("config = %+v", config)
	}
	if got := NewClient(Config{MX: 5}).config.Window; got != 7*time.Second {
		t.Fatalf("window = %v", got)
	}
}

func TestDefaultInterfacesAreUsable(t *testing.T) {
	for _, ifi := range NewClient(Config{}).multicastInterfaces() {
		if ifi.Flags&net.FlagUp == 0 || ifi.Flags&net.FlagMulticast == 0 || ifi.Flags&net.FlagLoopback != 0 || !hasIPv4(ifi) {
			t.Errorf("unusable interface selected: %+v", ifi)
		}
	}
}
