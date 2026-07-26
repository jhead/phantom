package proxy

import (
	"fmt"
	"net"
	"syscall"
	"testing"
	"time"

	"github.com/jhead/phantom/internal/proto"
)

func TestIsTimeoutError(t *testing.T) {
	t.Parallel()

	if !isTimeoutError(timeoutError{}) {
		t.Fatal("expected net.Error timeout")
	}
	if !isTimeoutError(fmt.Errorf("read udp: i/o timeout")) {
		t.Fatal("expected legacy timeout string")
	}
	if isTimeoutError(syscall.ECONNREFUSED) {
		t.Fatal("connection refused is not a timeout")
	}
	if isTimeoutError(nil) {
		t.Fatal("nil should not be a timeout")
	}
}

func TestIsConnRefusedError(t *testing.T) {
	t.Parallel()

	if !isConnRefusedError(syscall.ECONNREFUSED) {
		t.Fatal("expected syscall.ECONNREFUSED")
	}
	if !isConnRefusedError(fmt.Errorf("write udp: %w", syscall.ECONNREFUSED)) {
		t.Fatal("expected wrapped ECONNREFUSED")
	}
	if !isConnRefusedError(fmt.Errorf("write: connection refused")) {
		t.Fatal("expected legacy connection refused string")
	}
	if isConnRefusedError(timeoutError{}) {
		t.Fatal("timeout is not connection refused")
	}
}

func TestSingleTimeoutDoesNotMarkOffline(t *testing.T) {
	t.Parallel()

	p := &ProxyServer{}
	p.noteUpstreamReadError(fmt.Errorf("read udp: i/o timeout"))
	if p.serverOffline {
		t.Fatal("a single timeout must not advertise OfflinePong (#104)")
	}
	if p.offlineTimeouts != 1 {
		t.Fatalf("offlineTimeouts = %d, want 1", p.offlineTimeouts)
	}
}

func TestSustainedTimeoutsMarkOffline(t *testing.T) {
	t.Parallel()

	p := &ProxyServer{}
	for i := 0; i < offlineTimeoutThreshold-1; i++ {
		p.noteUpstreamReadError(timeoutError{})
		if p.serverOffline {
			t.Fatalf("marked offline after %d timeouts", i+1)
		}
	}
	p.noteUpstreamReadError(timeoutError{})
	if !p.serverOffline {
		t.Fatalf("expected offline after %d timeouts", offlineTimeoutThreshold)
	}
}

func TestConnRefusedMarksOfflineImmediately(t *testing.T) {
	t.Parallel()

	p := &ProxyServer{}
	p.noteUpstreamReadError(syscall.ECONNREFUSED)
	if !p.serverOffline {
		t.Fatal("expected connection refused to mark offline")
	}
}

func TestReachableClearsTimeoutStreak(t *testing.T) {
	t.Parallel()

	p := &ProxyServer{}
	p.noteUpstreamReadError(timeoutError{})
	p.noteUpstreamReadError(timeoutError{})
	p.noteUpstreamReachable()
	if p.offlineTimeouts != 0 {
		t.Fatalf("offlineTimeouts = %d, want 0 after success", p.offlineTimeouts)
	}
	p.noteUpstreamReadError(timeoutError{})
	if p.serverOffline {
		t.Fatal("streak should have reset; one timeout after success must not mark offline")
	}
}

func TestOfflinePongRewriteInjectsBindPort(t *testing.T) {
	t.Parallel()

	p := &ProxyServer{boundPort: 54321}
	rewritten := p.rewriteUnconnectedPong(proto.OfflinePong.Bytes())
	packet, err := proto.ReadUnconnectedPing(rewritten)
	if err != nil {
		t.Fatalf("parse offline pong: %v", err)
	}
	if packet.Pong.Port4 != "54321" {
		t.Fatalf("Port4 = %q, want 54321 (clients must not fall back to :19132)", packet.Pong.Port4)
	}
	if packet.Pong.Port6 != "54321" {
		t.Fatalf("Port6 = %q, want 54321", packet.Pong.Port6)
	}
}

func TestReadDeadlineClassifiesAsTimeout(t *testing.T) {
	t.Parallel()

	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Millisecond))
	buf := make([]byte, 16)
	_, _, err = conn.ReadFrom(buf)
	if err == nil {
		t.Fatal("expected read deadline error")
	}
	if !isTimeoutError(err) {
		t.Fatalf("expected deadline error to be timeout, got %v", err)
	}
	if isConnRefusedError(err) {
		t.Fatal("deadline error should not be connection refused")
	}
}

type timeoutError struct{}

func (timeoutError) Error() string   { return "i/o timeout" }
func (timeoutError) Timeout() bool   { return true }
func (timeoutError) Temporary() bool { return true }

var _ net.Error = timeoutError{}
