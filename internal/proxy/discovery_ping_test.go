package proxy

import (
	"bytes"
	"net"
	"testing"
	"time"

	"github.com/jhead/phantom/internal/proto"
	"github.com/stretchr/testify/require"
)

// bareUnconnectedPing builds a minimal RakNet Unconnected Ping (0x01).
func bareUnconnectedPing(pingTime [8]byte) []byte {
	return proto.BuildUnconnectedPing(pingTime[:], nil)
}

// startFakeBedrockPongServer stands in for a strict RakNet server: like
// RakLib/PocketMine it validates the offline magic and ignores anything that
// fails, which is what makes a wrongly-ordered ping observable in tests.
func startFakeBedrockPongServer(t *testing.T) *net.UDPAddr {
	t.Helper()
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	go func() {
		buf := make([]byte, 2048)
		for {
			n, addr, err := conn.ReadFromUDP(buf)
			if err != nil {
				return
			}
			if !proto.IsUnconnectedPing(buf[:n]) {
				continue
			}
			pong := proto.UnconnectedPing{
				PingTime: append([]byte(nil), buf[1:9]...),
				ID:       []byte{1, 2, 3, 4, 5, 6, 7, 8},
				Magic:    []byte{0x00, 0xff, 0xff, 0x00, 0xfe, 0xfe, 0xfe, 0xfe, 0xfd, 0xfd, 0xfd, 0xfd, 0x12, 0x34, 0x56, 0x78},
				Pong: proto.PongData{
					Edition:         "MCPE",
					MOTD:            "test",
					ProtocolVersion: "390",
					Version:         "1.14.60",
					Players:         "0",
					MaxPlayers:      "10",
					ServerID:        "99",
					SubMOTD:         "",
					GameType:        "Survival",
					NintendoLimited: "1",
					Port4:           "19132",
					Port6:           "19132",
				},
			}.Build()
			_, _ = conn.WriteToUDP(pong.Bytes(), addr)
		}
	}()

	return conn.LocalAddr().(*net.UDPAddr)
}

func TestDiscoveryPingDoesNotReuseStaleSession(t *testing.T) {
	remoteAddr := startFakeBedrockPongServer(t)

	// Stale "gameplay" peer: accepts datagrams but never replies with a pong.
	staleRemote, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	require.NoError(t, err)
	t.Cleanup(func() { _ = staleRemote.Close() })

	proxyServer, err := New(ProxyPrefs{
		BindAddress:  "127.0.0.1",
		BindPort:     0,
		RemoteServer: remoteAddr.String(),
		IdleTimeout:  time.Minute,
		NumWorkers:   1,
	})
	require.NoError(t, err)

	dataConn, err := net.ListenUDP("udp", proxyServer.bindAddress)
	require.NoError(t, err)
	proxyServer.server.Store(dataConn)
	t.Cleanup(func() {
		proxyServer.dead.Set()
		_ = dataConn.Close()
		proxyServer.clientMap.Close()
	})

	clientConn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	require.NoError(t, err)
	t.Cleanup(func() { _ = clientConn.Close() })
	clientAddr := clientConn.LocalAddr().(*net.UDPAddr)

	// Plant a stale gameplay session for this console address. Pre-fix code
	// would reuse this DialUDP for LAN pings and hang without a pong.
	_, err = proxyServer.clientMap.Get(clientAddr, staleRemote.LocalAddr().(*net.UDPAddr), func(*net.UDPConn) {})
	require.NoError(t, err)
	require.Equal(t, 1, proxyServer.clientMap.Len())

	pingTime := [8]byte{9, 8, 7, 6, 5, 4, 3, 2}
	require.NoError(t, proxyServer.handleDiscoveryPing(clientAddr, bareUnconnectedPing(pingTime)))

	_ = clientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 2048)
	n, _, err := clientConn.ReadFromUDP(buf)
	require.NoError(t, err)
	require.True(t, n >= 1)
	require.Equal(t, proto.UnconnectedPongID, buf[0])
	require.True(t, bytes.Equal(buf[1:9], pingTime[:]), "pong must echo ping time")

	// Discovery must not consume/replace the gameplay session entry.
	require.Equal(t, 1, proxyServer.clientMap.Len())
}

func TestBuildOfflinePongEchoesPingTime(t *testing.T) {
	proxyServer, err := New(ProxyPrefs{
		BindAddress:  "127.0.0.1",
		BindPort:     19000,
		RemoteServer: "127.0.0.1:19001",
		IdleTimeout:  time.Minute,
		NumWorkers:   1,
	})
	require.NoError(t, err)

	pingTime := [8]byte{1, 2, 3, 4, 5, 6, 7, 8}
	pong := proxyServer.buildOfflinePong(bareUnconnectedPing(pingTime))
	require.Equal(t, proto.UnconnectedPongID, pong[0])
	require.True(t, bytes.Equal(pong[1:9], pingTime[:]))
}

// TestOfflineDiscoveryPingRepliesImmediately guards the LAN-list bug where
// phantom waited on a recovery probe before sending OfflinePong. Blackholed
// remotes take the full deadline, so consoles timed out and never showed the
// offline entry even though a pong was eventually sent.
func TestOfflineDiscoveryPingRepliesImmediately(t *testing.T) {
	blackhole, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	require.NoError(t, err)
	t.Cleanup(func() { _ = blackhole.Close() })

	proxyServer, err := New(ProxyPrefs{
		BindAddress:              "127.0.0.1",
		BindPort:                 0,
		RemoteServer:             blackhole.LocalAddr().String(),
		IdleTimeout:              time.Minute,
		NumWorkers:               1,
		DisableDiscoveryListener: true,
	})
	require.NoError(t, err)

	dataConn, err := net.ListenUDP("udp", proxyServer.bindAddress)
	require.NoError(t, err)
	proxyServer.server.Store(dataConn)
	t.Cleanup(func() {
		proxyServer.dead.Set()
		_ = dataConn.Close()
		proxyServer.clientMap.Close()
	})

	// If handleDiscoveryPing waited for recovery, this would delay the reply.
	prev := discoveryRecoveryProbeTimeoutNanos.Swap(int64(2 * time.Second))
	t.Cleanup(func() { discoveryRecoveryProbeTimeoutNanos.Store(prev) })

	proxyServer.markServerOffline()

	clientConn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	require.NoError(t, err)
	t.Cleanup(func() { _ = clientConn.Close() })
	clientAddr := clientConn.LocalAddr().(*net.UDPAddr)

	pingTime := [8]byte{9, 8, 7, 6, 5, 4, 3, 2}
	started := time.Now()
	require.NoError(t, proxyServer.handleDiscoveryPing(clientAddr, bareUnconnectedPing(pingTime)))
	elapsed := time.Since(started)
	if elapsed >= 200*time.Millisecond {
		t.Fatalf("offline pong took %v; must not wait on upstream recovery probe", elapsed)
	}

	_ = clientConn.SetReadDeadline(time.Now().Add(time.Second))
	buf := make([]byte, 2048)
	n, _, err := clientConn.ReadFromUDP(buf)
	require.NoError(t, err)
	require.Equal(t, proto.UnconnectedPongID, buf[0])
	require.True(t, bytes.Equal(buf[1:9], pingTime[:]), "pong must echo ping time")

	packet, err := proto.ReadUnconnectedPing(buf[:n])
	require.NoError(t, err)
	require.Contains(t, packet.Pong.MOTD, "offline")
}
