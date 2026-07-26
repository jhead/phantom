package proxy

import (
	"fmt"
	"net"
	"syscall"
	"testing"
	"time"
)

func TestIsOfflineError(t *testing.T) {
	t.Parallel()

	if !isOfflineError(syscall.ECONNREFUSED) {
		t.Fatal("expected syscall.ECONNREFUSED to be offline")
	}
	if !isOfflineError(fmt.Errorf("write udp: %w", syscall.ECONNREFUSED)) {
		t.Fatal("expected wrapped ECONNREFUSED to be offline")
	}
	if !isOfflineError(&net.OpError{Op: "write", Err: syscall.ECONNREFUSED}) {
		t.Fatal("expected net.OpError ECONNREFUSED to be offline")
	}
	if !isOfflineError(timeoutError{}) {
		t.Fatal("expected net.Error timeout to be offline")
	}
	if !isOfflineError(fmt.Errorf("read udp: i/o timeout")) {
		t.Fatal("expected legacy timeout string to be offline")
	}
	if !isOfflineError(fmt.Errorf("write: connection refused")) {
		t.Fatal("expected legacy connection refused string to be offline")
	}
	if isOfflineError(nil) {
		t.Fatal("nil should not be offline")
	}
	if isOfflineError(fmt.Errorf("something else")) {
		t.Fatal("unrelated error should not be offline")
	}
}

type timeoutError struct{}

func (timeoutError) Error() string   { return "i/o timeout" }
func (timeoutError) Timeout() bool   { return true }
func (timeoutError) Temporary() bool { return true }

// Ensure timeoutError satisfies net.Error at compile time.
var _ net.Error = timeoutError{}

// Keep the idle deadline behavior documented: a timeout longer than the
// per-write read deadline should still classify as offline.
func TestIsOfflineErrorDeadline(t *testing.T) {
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
	if !isOfflineError(err) {
		t.Fatalf("expected deadline error to be offline, got %v", err)
	}
}
