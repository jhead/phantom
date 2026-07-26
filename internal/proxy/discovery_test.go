package proxy

import (
	"encoding/binary"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/jhead/phantom/internal/proto"
)

func TestHandleUnconnectedPingOfflineReply(t *testing.T) {
	remote, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	defer remote.Close()

	client, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	p, err := New(ProxyPrefs{
		BindAddress:              "127.0.0.1",
		BindPort:                 0,
		RemoteServer:             remote.LocalAddr().String(),
		IdleTimeout:              time.Minute,
		NumWorkers:               1,
		DisableDiscoveryListener: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := p.StartAsync(); err != nil {
		t.Fatal(err)
	}
	defer p.Close()

	p.serverOffline = true

	ping := []byte{proto.UnconnectedPingID, 0, 0, 0, 0, 0, 0, 0, 0}
	if err := p.HandleUnconnectedPing(ping, client.LocalAddr()); err != nil {
		t.Fatalf("HandleUnconnectedPing: %v", err)
	}

	_ = client.SetReadDeadline(time.Now().Add(time.Second))
	buf := make([]byte, 2048)
	n, from, err := client.ReadFrom(buf)
	if err != nil {
		t.Fatalf("client ReadFrom: %v", err)
	}
	if n < 1 || buf[0] != proto.UnconnectedPongID {
		t.Fatalf("expected UnconnectedPong, got n=%d id=%v", n, buf[:n])
	}
	fromUDP, ok := from.(*net.UDPAddr)
	if !ok {
		t.Fatalf("unexpected from type %T", from)
	}
	if uint16(fromUDP.Port) != p.boundPort {
		t.Fatalf("pong from port %d, want boundPort %d", fromUDP.Port, p.boundPort)
	}

	// Ping is still forwarded to the remote even when offline.
	_ = remote.SetReadDeadline(time.Now().Add(time.Second))
	rn, _, err := remote.ReadFrom(buf)
	if err != nil {
		t.Fatalf("remote did not receive forwarded ping: %v", err)
	}
	if rn < 1 || buf[0] != proto.UnconnectedPingID {
		t.Fatalf("remote expected ping, got n=%d id=%v", rn, buf[:rn])
	}
}

func TestHandleUnconnectedPingRejectsBadInput(t *testing.T) {
	p, err := New(ProxyPrefs{
		BindAddress:              "127.0.0.1",
		BindPort:                 0,
		RemoteServer:             "127.0.0.1:19132",
		IdleTimeout:              time.Minute,
		DisableDiscoveryListener: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()

	client := &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 9}
	if err := p.HandleUnconnectedPing([]byte{proto.UnconnectedPingID}, client); err == nil {
		t.Fatal("expected error when proxy not started")
	}

	if err := p.StartAsync(); err != nil {
		t.Fatal(err)
	}

	if err := p.HandleUnconnectedPing([]byte{proto.UnconnectedPongID}, client); err == nil {
		t.Fatal("expected error for non-ping packet")
	}
	if err := p.HandleUnconnectedPing([]byte{proto.UnconnectedPingID}, nil); err == nil {
		t.Fatal("expected error for nil from")
	}
}

func TestPerProxyServerIDsDistinct(t *testing.T) {
	a, err := New(ProxyPrefs{
		BindAddress:              "127.0.0.1",
		BindPort:                 0,
		RemoteServer:             "127.0.0.1:19132",
		IdleTimeout:              time.Minute,
		DisableDiscoveryListener: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	b, err := New(ProxyPrefs{
		BindAddress:              "127.0.0.1",
		BindPort:                 0,
		RemoteServer:             "127.0.0.1:19133",
		IdleTimeout:              time.Minute,
		DisableDiscoveryListener: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if a.serverID == b.serverID {
		t.Fatalf("expected distinct serverIDs, both %d", a.serverID)
	}

	rewritten := a.rewriteUnconnectedPong(proto.OfflinePong.Bytes())
	parsed, err := proto.ReadUnconnectedPing(rewritten)
	if err != nil {
		t.Fatal(err)
	}
	gotGUID := binary.BigEndian.Uint64(parsed.ID)
	if gotGUID != uint64(a.serverID) {
		t.Fatalf("binary GUID %d, want %d", gotGUID, uint64(a.serverID))
	}
	want := fmt.Sprintf("%d", a.serverID)
	if parsed.Pong.ServerID != want {
		t.Fatalf("MOTD ServerID %q, want %q", parsed.Pong.ServerID, want)
	}
}
