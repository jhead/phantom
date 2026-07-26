package proxy

// Tests for the packets phantom *sends* upstream, and for how often it sends them.
//
// Everything here is about the discovery probe rather than the pong rewrite.
// The probe is the one place phantom originates protocol rather than relaying
// it, and a malformed probe fails silently: strict servers just ignore it, so
// the only symptom is phantom deciding a healthy server is offline.

import (
	"bytes"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/jhead/phantom/internal/corpus"
	"github.com/jhead/phantom/internal/proto"
	"github.com/stretchr/testify/require"
)

// strictServer answers only well-formed Unconnected Pings, the way RakLib
// (PocketMine) and Nukkit do: they validate the offline magic and drop
// anything that fails. A permissive fake would have happily answered the
// malformed probe this file guards against.
type strictServer struct {
	conn *net.UDPConn
	motd string

	mu       sync.Mutex
	accepted int
	rejected int
}

func startStrictServer(t *testing.T, motd string) *strictServer {
	t.Helper()

	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	s := &strictServer{conn: conn, motd: motd}

	go func() {
		buf := make([]byte, 2048)
		for {
			n, addr, err := conn.ReadFromUDP(buf)
			if err != nil {
				return
			}
			if !proto.IsUnconnectedPing(buf[:n]) {
				s.mu.Lock()
				s.rejected++
				s.mu.Unlock()
				continue
			}

			s.mu.Lock()
			s.accepted++
			s.mu.Unlock()

			pong := proto.UnconnectedPing{
				PingTime: append([]byte(nil), buf[1:9]...),
				ID:       []byte{1, 2, 3, 4, 5, 6, 7, 8},
				Magic:    append([]byte(nil), buf[9:25]...),
				Pong:     parseMOTD(s.motd),
			}.Build()
			_, _ = conn.WriteToUDP(pong.Bytes(), addr)
		}
	}()

	return s
}

func (s *strictServer) addr() string { return s.conn.LocalAddr().String() }

func (s *strictServer) counts() (accepted, rejected int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.accepted, s.rejected
}

// parseMOTD turns a MOTD string into PongData through phantom's own reader,
// which keeps these fixtures readable as the wire strings servers actually send.
func parseMOTD(motd string) proto.PongData {
	frame := corpus.BuildPong(make([]byte, 8), make([]byte, 8), motd)
	packet, err := proto.ReadUnconnectedPing(frame)
	if err != nil {
		panic(err)
	}
	return packet.Pong
}

// newUnstartedProxy builds a proxy with a bound data socket but no listen(),
// so no background health check competes with what a test is measuring.
func newUnstartedProxy(t *testing.T, remote string) *ProxyServer {
	t.Helper()

	p, err := New(ProxyPrefs{
		BindAddress:              "127.0.0.1",
		BindPort:                 0,
		RemoteServer:             remote,
		IdleTimeout:              time.Minute,
		NumWorkers:               1,
		DisableDiscoveryListener: true,
	})
	require.NoError(t, err)

	dataConn, err := net.ListenUDP("udp", p.bindAddress)
	require.NoError(t, err)
	p.server.Store(dataConn)

	t.Cleanup(func() {
		p.dead.Set()
		_ = dataConn.Close()
		p.clientMap.Close()
	})
	return p
}

func TestHealthCheckPingIsWellFormed(t *testing.T) {
	t.Parallel()

	p := &ProxyServer{serverID: 0x0123456789ABCDEF}
	ping := p.healthCheckPing()

	require.True(t, proto.IsUnconnectedPing(ping),
		"background probe must be a valid Unconnected Ping or magic-validating servers drop it")
	require.Len(t, ping, proto.UnconnectedPingLen)
	require.Equal(t, proto.UnconnectedPingID, ping[0])
}

// TestHealthCheckReachesStrictServer is the regression for the probe that used
// pong field order. Against a server that validates the magic, every health
// check was dropped, so phantom latched "offline" and answered every LAN
// discovery ping with OfflinePong while the server was up the whole time.
func TestHealthCheckReachesStrictServer(t *testing.T) {
	srv := startStrictServer(t, "MCPE;Strict;800;1.21.80;0;10;123;Sub;Survival;1;19132;19133;")

	p, err := New(ProxyPrefs{
		BindAddress:              "127.0.0.1",
		BindPort:                 0,
		RemoteServer:             srv.addr(),
		IdleTimeout:              time.Minute,
		NumWorkers:               1,
		DisableDiscoveryListener: true,
	})
	require.NoError(t, err)
	require.NoError(t, p.StartAsync())
	defer p.Close()

	deadline := time.Now().Add(5 * time.Second)
	for {
		accepted, _ := srv.counts()
		if accepted > 0 && !p.isServerOffline() {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("health check never got a pong from a magic-validating server "+
				"(accepted=%d offline=%v)", accepted, p.isServerOffline())
		}
		time.Sleep(20 * time.Millisecond)
	}

	_, rejected := srv.counts()
	require.Zero(t, rejected, "phantom sent %d malformed pings upstream", rejected)
}

// TestDiscoveryPongsServedFromCache pins the fan-in behaviour: consoles re-ping
// about once a second and each one pings independently, so answering every ping
// with its own upstream probe made phantom's rate at the remote scale with the
// number of clients on the LAN.
func TestDiscoveryPongsServedFromCache(t *testing.T) {
	srv := startStrictServer(t, "MCPE;Cached;800;1.21.80;3;10;123;Sub;Survival;1;19132;19133;")
	p := newUnstartedProxy(t, srv.addr())

	client, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })

	const pings = 5
	for i := 0; i < pings; i++ {
		pingTime := [8]byte{byte(i), 2, 3, 4, 5, 6, 7, 8}
		require.NoError(t, p.handleDiscoveryPing(client.LocalAddr(), bareUnconnectedPing(pingTime)))

		// Every reply must still be addressed to the ping that asked for it.
		_ = client.SetReadDeadline(time.Now().Add(2 * time.Second))
		buf := make([]byte, 2048)
		n, _, err := client.ReadFromUDP(buf)
		require.NoError(t, err)
		require.Equal(t, proto.UnconnectedPongID, buf[0])
		require.True(t, bytes.Equal(buf[1:9], pingTime[:]),
			"ping %d: cached pong must carry this client's ping time", i)

		packet, err := proto.ReadUnconnectedPing(buf[:n])
		require.NoError(t, err)
		require.Equal(t, "Cached", packet.Pong.MOTD)
	}

	accepted, _ := srv.counts()
	require.Equal(t, 1, accepted,
		"%d discovery pings produced %d upstream probes; a fresh pong should answer them all", pings, accepted)
}

func TestDiscoveryProbesAgainAfterCacheExpires(t *testing.T) {
	srv := startStrictServer(t, "MCPE;Expiry;800;1.21.80;0;10;123;Sub;Survival;1;19132;19133;")
	p := newUnstartedProxy(t, srv.addr())

	prev := discoveryPongCacheTTLNanos.Swap(int64(20 * time.Millisecond))
	t.Cleanup(func() { discoveryPongCacheTTLNanos.Store(prev) })

	client, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })

	require.NoError(t, p.handleDiscoveryPing(client.LocalAddr(), bareUnconnectedPing([8]byte{1})))
	time.Sleep(50 * time.Millisecond)
	require.NoError(t, p.handleDiscoveryPing(client.LocalAddr(), bareUnconnectedPing([8]byte{2})))

	accepted, _ := srv.counts()
	require.Equal(t, 2, accepted, "a stale cache entry must not be reused")
}

// TestOfflinePongInheritsUpstreamIdentity covers the offline advertisement
// after the remote has been seen at least once. Falling back to a hardcoded
// protocol number would make the LAN entry claim a version unrelated to the
// server behind it, and one that ages out as Mojang ships new releases.
func TestOfflinePongInheritsUpstreamIdentity(t *testing.T) {
	t.Parallel()

	p := &ProxyServer{boundPort: 54321, serverID: 7}

	upstream := proto.UnconnectedPing{
		PingTime: make([]byte, 8),
		ID:       []byte{9, 9, 9, 9, 9, 9, 9, 9},
		Magic:    []byte{0x00, 0xff, 0xff, 0x00, 0xfe, 0xfe, 0xfe, 0xfe, 0xfd, 0xfd, 0xfd, 0xfd, 0x12, 0x34, 0x56, 0x78},
		Pong:     parseMOTD("MCPE;Real Server;818;1.21.100;4;20;123;Bedrock level;Survival;1;19132;19133;"),
	}.Build()
	p.recordPong(upstream.Bytes())

	pingTime := [8]byte{5, 5, 5, 5, 5, 5, 5, 5}
	offline := p.buildOfflinePong(bareUnconnectedPing(pingTime))

	packet, err := proto.ReadUnconnectedPing(offline)
	require.NoError(t, err)

	require.True(t, bytes.Equal(offline[1:9], pingTime[:]), "offline pong must echo the ping time")
	require.Equal(t, proto.OfflineMOTD, packet.Pong.MOTD)
	require.Equal(t, "818", packet.Pong.ProtocolVersion, "offline entry must keep the real server's protocol")
	require.Equal(t, "1.21.100", packet.Pong.Version)
	require.Equal(t, "MCPE", packet.Pong.Edition)
	require.Equal(t, "0", packet.Pong.Players)
	require.Equal(t, "20", packet.Pong.MaxPlayers)
	require.Equal(t, "54321", packet.Pong.Port4, "clients must not fall back to :19132")
}

func TestOfflinePongFallsBackWhenUpstreamNeverSeen(t *testing.T) {
	t.Parallel()

	p := &ProxyServer{boundPort: 54321, serverID: 7}
	offline := p.buildOfflinePong(bareUnconnectedPing([8]byte{1}))

	packet, err := proto.ReadUnconnectedPing(offline)
	require.NoError(t, err)
	require.Equal(t, proto.OfflineMOTD, packet.Pong.MOTD)
	require.NotEmpty(t, packet.Pong.ProtocolVersion)
	require.NotEqual(t, "0", packet.Pong.MaxPlayers,
		"some consoles omit zero-slot entries from the Friends list")
}

// TestCachedPongIsNotAliased guards the cache against its callers. Every
// discovery reply stamps the requesting client's ping time into the frame it
// was handed, so handing out the stored slice would let one client's write land
// in the cache - and race every concurrent reader of it.
func TestCachedPongIsNotAliased(t *testing.T) {
	t.Parallel()

	p := &ProxyServer{}
	original := corpus.BuildPong(
		[]byte{1, 1, 1, 1, 1, 1, 1, 1},
		make([]byte, 8),
		"MCPE;Alias;800;1.21.80;0;10;123;Sub;Survival;1;19132;19133;",
	)
	p.recordPong(original)

	// Mutating the frame the caller passed in must not reach the cache either.
	original[1] = 0xEE

	first, ok := p.cachedPong()
	require.True(t, ok)
	require.Equal(t, byte(1), first[1], "recordPong must copy its argument")

	stampPingTime(first, proto.BuildUnconnectedPing([]byte{7, 7, 7, 7, 7, 7, 7, 7}, nil))

	second, ok := p.cachedPong()
	require.True(t, ok)
	require.Equal(t, byte(1), second[1], "cachedPong must hand out a copy")
	require.Equal(t, byte(1), p.lastKnownPong()[1])
}

// TestRecordPongIgnoresUnparseableFrames keeps garbage out of the cache: a
// frame phantom cannot read is useless as an offline template and must never
// be replayed to a client as though it came from the server.
func TestRecordPongIgnoresUnparseableFrames(t *testing.T) {
	t.Parallel()

	p := &ProxyServer{}
	p.recordPong([]byte{proto.UnconnectedPongID, 1, 2, 3})
	require.Nil(t, p.lastKnownPong())

	_, ok := p.cachedPong()
	require.False(t, ok)
}
