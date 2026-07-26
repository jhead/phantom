package proxy

import (
	"bytes"
	"net"
	"testing"
	"time"
)

func TestUDPRecvBufferCoversOversizedRakNetDatagrams(t *testing.T) {
	// Historical phantom buffer was 1472. go-raknet reads with 1492; some
	// servers have been observed sending ≥1474-byte UDP payloads.
	if udpRecvBufferSize < 1492 {
		t.Fatalf("udpRecvBufferSize=%d; want at least 1492 to avoid truncating RakNet MTU probes", udpRecvBufferSize)
	}

	pc, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer pc.Close()

	client, err := net.DialUDP("udp4", nil, pc.LocalAddr().(*net.UDPAddr))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	for _, size := range []int{1464, 1472, 1474, 1492, 2048, 4096} {
		payload := make([]byte, size)
		for i := range payload {
			payload[i] = byte(i % 251)
		}
		payload[0] = 0xFE
		payload[size-1] = 0xAA

		if _, err := client.Write(payload); err != nil {
			t.Fatalf("write %d: %v", size, err)
		}

		_ = pc.SetReadDeadline(time.Now().Add(time.Second))
		buf := make([]byte, udpRecvBufferSize)
		n, _, err := pc.ReadFrom(buf)
		if err != nil {
			t.Fatalf("read %d: %v", size, err)
		}
		if n != size {
			t.Fatalf("size %d: got n=%d (truncated)", size, n)
		}
		if !bytes.Equal(buf[:n], payload) {
			t.Fatalf("size %d: payload mismatch", size)
		}
	}
}

func TestProxyForwardsLargeDatagramsBothWays(t *testing.T) {
	remote, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	defer remote.Close()

	// Exclusive :19132 bind in Start() — skip cleanly if another phantom owns it.
	probe, err := net.ListenPacket("udp4", "127.0.0.1:19132")
	if err != nil {
		t.Skipf("port 19132 unavailable: %v", err)
	}
	probe.Close()

	p, err := New(ProxyPrefs{
		BindAddress:  "127.0.0.1",
		BindPort:     0,
		RemoteServer: remote.LocalAddr().String(),
		IdleTimeout:  time.Minute,
		NumWorkers:   1,
	})
	if err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() { done <- p.Start() }()
	defer p.Close()

	// Wait until data plane is listening.
	deadline := time.Now().Add(2 * time.Second)
	for p.server == nil && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if p.server == nil {
		select {
		case err := <-done:
			t.Fatalf("Start failed: %v", err)
		default:
			t.Fatal("proxy server not listening")
		}
	}

	client, err := net.DialUDP("udp4", nil, p.bindAddress)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	sizes := []int{1400, 1474, 1492, 2048}
	for _, size := range sizes {
		up := make([]byte, size)
		up[0] = 0x8C // non-pong game/control byte
		up[size-1] = 0x11
		if _, err := client.Write(up); err != nil {
			t.Fatalf("client write %d: %v", size, err)
		}

		_ = remote.SetReadDeadline(time.Now().Add(time.Second))
		rbuf := make([]byte, udpRecvBufferSize)
		rn, from, err := remote.ReadFrom(rbuf)
		if err != nil {
			t.Fatalf("remote read %d: %v", size, err)
		}
		if rn != size || !bytes.Equal(rbuf[:rn], up) {
			t.Fatalf("upstream got n=%d want %d (truncated or corrupt)", rn, size)
		}

		down := make([]byte, size)
		down[0] = 0x8D
		down[size-1] = 0x22
		if _, err := remote.WriteTo(down, from); err != nil {
			t.Fatalf("remote write %d: %v", size, err)
		}

		_ = client.SetReadDeadline(time.Now().Add(time.Second))
		cbuf := make([]byte, udpRecvBufferSize)
		cn, _, err := client.ReadFrom(cbuf)
		if err != nil {
			t.Fatalf("client read %d: %v", size, err)
		}
		if cn != size || !bytes.Equal(cbuf[:cn], down) {
			t.Fatalf("client got n=%d want %d (truncated or corrupt)", cn, size)
		}
	}
}
