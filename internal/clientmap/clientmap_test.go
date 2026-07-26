package clientmap

import (
	"net"
	"testing"
	"time"
)

func startUDPServer(t *testing.T) (*net.UDPConn, *net.UDPAddr) {
	t.Helper()
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatalf("ListenUDP: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn, conn.LocalAddr().(*net.UDPAddr)
}

func TestIsOwnAddress(t *testing.T) {
	_, remote := startUDPServer(t)
	cm := New(time.Minute, time.Minute)
	defer cm.Close()

	clientAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:34567")
	if err != nil {
		t.Fatal(err)
	}

	serverConn, err := cm.Get(clientAddr, remote, func(*net.UDPConn) {})
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	if !cm.IsOwnAddress(serverConn.LocalAddr()) {
		t.Fatalf("expected outbound local addr %s to be recognized as own", serverConn.LocalAddr())
	}

	other, err := net.ResolveUDPAddr("udp", "127.0.0.1:34568")
	if err != nil {
		t.Fatal(err)
	}
	if cm.IsOwnAddress(other) {
		t.Fatalf("did not expect unrelated addr %s to be own", other)
	}
	if cm.IsOwnAddress(nil) {
		t.Fatal("nil addr should not be own")
	}
}

// Documents the failure mode behind issue #184: if an echoed outbound packet is
// keyed as a new client, Get opens another UDP socket. Repeating that (as the
// ping listener can when -server is this host:19132) exhausts file descriptors.
func TestGetOnOwnLocalAddrCreatesExtraConnection(t *testing.T) {
	_, remote := startUDPServer(t)
	cm := New(time.Minute, time.Minute)
	defer cm.Close()

	clientAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:34569")
	if err != nil {
		t.Fatal(err)
	}

	conn1, err := cm.Get(clientAddr, remote, func(*net.UDPConn) {})
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if cm.Len() != 1 {
		t.Fatalf("Len=%d, want 1", cm.Len())
	}

	conn2, err := cm.Get(conn1.LocalAddr(), remote, func(*net.UDPConn) {})
	if err != nil {
		t.Fatalf("Get(own local addr): %v", err)
	}
	if cm.Len() != 2 {
		t.Fatalf("Len=%d, want 2 after treating own local addr as a client", cm.Len())
	}
	if conn1.LocalAddr().String() == conn2.LocalAddr().String() {
		t.Fatal("expected a distinct outbound socket for the echoed addr")
	}
}

func TestSkippingOwnAddressPreventsConnectionStorm(t *testing.T) {
	_, remote := startUDPServer(t)
	cm := New(time.Minute, time.Minute)
	defer cm.Close()

	clientAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:34570")
	if err != nil {
		t.Fatal(err)
	}

	conn, err := cm.Get(clientAddr, remote, func(*net.UDPConn) {})
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	// Proxy fix: never call Get for IsOwnAddress sources.
	for i := 0; i < 64; i++ {
		echo := conn.LocalAddr()
		if !cm.IsOwnAddress(echo) {
			t.Fatalf("iteration %d: expected own address", i)
		}
		// skip Get — connection count must stay flat
	}

	if cm.Len() != 1 {
		t.Fatalf("Len=%d, want 1 after ignoring own-address echoes", cm.Len())
	}
}
