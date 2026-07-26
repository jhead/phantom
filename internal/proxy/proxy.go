package proxy

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math/rand"
	"net"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/jhead/phantom/internal/clientmap"
	"github.com/jhead/phantom/internal/proto"
	"github.com/rs/zerolog/log"
	"github.com/tevino/abool"

	reuse "github.com/libp2p/go-reuseport"
)

// udpRecvBufferSize is the per-read buffer for proxied UDP datagrams.
//
// RakNet advertises MTU up to 1492 including IP(20)+UDP(8) headers, so a
// well-formed max UDP payload is 1464. Some Bedrock stacks still emit slightly
// larger datagrams (observed ≥1474), and a too-small ReadFrom buffer silently
// truncates on several platforms — including macOS, where the error is nil.
// Resource-pack transfer is mostly full-MTU traffic, so truncation surfaces as
// clients stuck on "Loading resources...". Use the full UDP datagram limit.
const udpRecvBufferSize = 65535

// offlineTimeoutThreshold is how many consecutive upstream read timeouts are
// required before advertising OfflinePong on LAN discovery. A single flaky
// timeout must not flip the whole proxy "offline" (#104).
const offlineTimeoutThreshold = 3

// upstreamSessionTimeout bounds how long a gameplay session waits for the
// remote to say anything before the socket is reaped.
//
// It must stay comfortably above RakNet's keepalive cadence. Bedrock sends a
// connected ping every 5s and gives up on a peer after ~10s of silence, so a 5s
// deadline sat exactly on the keepalive boundary: a quiet moment mid-session
// could reap a live client's socket, and the client's next packet would arrive
// at the remote from a brand new source port that its connection table does not
// know. Idle clients are still cleaned up by ClientMap's IdleTimeout.
const upstreamSessionTimeout = 15 * time.Second

var idleCheckInterval = 5 * time.Second

// Discovery probe deadlines (nanoseconds). Stored atomically so tests can shorten
// them without racing the background health-check goroutine.
var (
	discoveryPingTimeoutNanos          atomic.Int64
	discoveryRecoveryProbeTimeoutNanos atomic.Int64
	discoveryHealthIntervalNanos       atomic.Int64
	discoveryPongCacheTTLNanos         atomic.Int64
)

func init() {
	discoveryPingTimeoutNanos.Store(int64(1500 * time.Millisecond))
	discoveryRecoveryProbeTimeoutNanos.Store(int64(500 * time.Millisecond))
	discoveryHealthIntervalNanos.Store(int64(2 * time.Second))
	// Matches the health-check interval, so the background probe alone keeps
	// discovery answerable and player counts stay at most one interval stale.
	discoveryPongCacheTTLNanos.Store(int64(2 * time.Second))
}

func discoveryPingTimeout() time.Duration {
	return time.Duration(discoveryPingTimeoutNanos.Load())
}

func discoveryRecoveryProbeTimeout() time.Duration {
	return time.Duration(discoveryRecoveryProbeTimeoutNanos.Load())
}

func discoveryHealthInterval() time.Duration {
	return time.Duration(discoveryHealthIntervalNanos.Load())
}

func discoveryPongCacheTTL() time.Duration {
	return time.Duration(discoveryPongCacheTTLNanos.Load())
}

type ProxyServer struct {
	bindAddress         *net.UDPAddr
	boundPort           uint16
	remoteServerAddress *net.UDPAddr
	pingServer          net.PacketConn
	pingServerV6        net.PacketConn
	server              atomic.Pointer[net.UDPConn]
	clientMap           *clientmap.ClientMap
	prefs               ProxyPrefs
	dead                *abool.AtomicBool
	offlineMu           sync.Mutex
	serverOffline       bool
	offlineTimeouts     int
	serverID            int64

	pongMu       sync.Mutex
	lastPong     []byte
	lastPongTime time.Time
}

type ProxyPrefs struct {
	BindAddress  string
	BindPort     uint16
	RemoteServer string
	IdleTimeout  time.Duration
	EnableIPv6   bool
	RemovePorts  bool
	NumWorkers   uint
	// DisableDiscoveryListener skips binding :19132/:19133. Used when a
	// DiscoveryHub owns discovery and fans pings into HandleUnconnectedPing.
	DisableDiscoveryListener bool
}

var offlineErrorRegex = regexp.MustCompile("(timeout)|(connection refused)")

// isOfflineError reports whether err indicates the remote Bedrock server is
// unreachable. Connected UDP sockets surface ICMP port-unreachable as
// ECONNREFUSED on a later Read/Write — normal UDP behavior, not a proxy bug.
func isOfflineError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, syscall.ECONNREFUSED) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	// Fallback for platforms / wrappers that only expose the message text.
	return offlineErrorRegex.MatchString(err.Error())
}

func New(prefs ProxyPrefs) (*ProxyServer, error) {
	bindPort := prefs.BindPort

	// Randomize port if not provided. rand's top-level source is goroutine-safe
	// and auto-seeded; a bare rand.NewSource is neither, and New is called once
	// per -server value.
	if bindPort == 0 {
		bindPort = uint16(rand.Intn(14000)) + 50000
	}

	// Format full bind address with port
	prefs.BindAddress = fmt.Sprintf("%s:%d", prefs.BindAddress, bindPort)

	bindAddress, err := net.ResolveUDPAddr("udp", prefs.BindAddress)
	if err != nil {
		return nil, fmt.Errorf("Invalid bind address: %s", err)
	}

	remoteServerAddress, err := net.ResolveUDPAddr("udp", prefs.RemoteServer)
	if err != nil {
		return nil, fmt.Errorf("Invalid server address: %s", err)
	}

	return &ProxyServer{
		bindAddress:         bindAddress,
		boundPort:           bindPort,
		remoteServerAddress: remoteServerAddress,
		clientMap:           clientmap.New(prefs.IdleTimeout, idleCheckInterval),
		prefs:               prefs,
		dead:                abool.New(),
		serverID:            rand.Int63(),
	}, nil
}

func (proxy *ProxyServer) Start() error {
	if err := proxy.listen(); err != nil {
		return err
	}

	// Start processing everything else using the proxy listener
	proxy.startWorkers(proxy.dataConn())

	return nil
}

// StartAsync is like Start but runs all workers in goroutines and returns once
// listening. Used when one process hosts multiple proxies behind a DiscoveryHub.
func (proxy *ProxyServer) StartAsync() error {
	if err := proxy.listen(); err != nil {
		return err
	}

	log.Info().Msgf("Starting %d workers", proxy.prefs.NumWorkers)
	for i := uint(0); i < proxy.prefs.NumWorkers; i++ {
		go proxy.readLoop(proxy.dataConn())
	}
	return nil
}

func (proxy *ProxyServer) listen() error {
	if !proxy.prefs.DisableDiscoveryListener {
		// Exclusive bind: SO_REUSEPORT/ADDR cannot correctly share discovery
		// across processes (see DiscoveryHub / #175).
		log.Info().Msgf("Binding ping server to port 19132")
		pingServer, err := net.ListenPacket("udp4", ":19132")
		if err != nil {
			return fmt.Errorf("bind :19132: %w (only one phantom can own LAN discovery; pass multiple -server flags to one process instead of running multiple instances)", err)
		}
		proxy.pingServer = pingServer
		go proxy.readLoop(proxy.pingServer)

		// Minecraft automatically broadcasts on port 19133 to the local IPv6 network
		if proxy.prefs.EnableIPv6 {
			log.Info().Msgf("Binding IPv6 ping server to port 19133")
			if pingServerV6, err := net.ListenPacket("udp6", ":19133"); err == nil {
				proxy.pingServerV6 = pingServerV6
				go proxy.readLoop(proxy.pingServerV6)
			} else {
				log.Warn().Msgf("Failed to bind IPv6 ping listener: %v", err)
			}
		}
	}

	log.Info().Msgf("Binding proxy server to: %v", proxy.bindAddress)
	proxyServer, err := net.ListenUDP("udp", proxy.bindAddress)
	if err != nil {
		return err
	}
	proxy.server.Store(proxyServer)

	// Pongs go out over the data socket so consoles learn phantom's data port
	// from the reply's source address. A wildcard bind gives Go a dual-stack
	// socket, so that works for IPv6 discovery too; an explicit IPv4 bind does
	// not, and every v6 pong then fails with an address-family error that says
	// nothing about the cause.
	if proxy.prefs.EnableIPv6 {
		if local, ok := proxyServer.LocalAddr().(*net.UDPAddr); ok && local.IP.To4() != nil {
			log.Warn().Msgf(
				"IPv6 discovery is enabled but the data socket is bound to IPv4 %s; "+
					"IPv6 clients will not be able to reach it. Use the default -bind 0.0.0.0 for dual-stack.",
				local.IP)
		}
	}

	// Learn offline/online before the first console ping, and recover without
	// making OfflinePong wait on a synchronous probe.
	proxy.startUpstreamHealthCheck()

	return nil
}

func (proxy *ProxyServer) dataConn() *net.UDPConn {
	return proxy.server.Load()
}

// listenPacket prefers SO_REUSEPORT via libp2p/reuseport, but some platforms
// (notably iSH on iOS) return EINVAL for that option. Fall back to the stdlib
// listener only for that unsupported-option case so real bind conflicts still
// surface as errors.
func listenPacket(network, address string) (net.PacketConn, error) {
	conn, err := reuse.ListenPacket(network, address)
	if err == nil {
		return conn, nil
	}
	if !isReuseportUnsupported(err) {
		return nil, err
	}
	log.Warn().Msgf("reuseport listen %s %s unsupported (%v); falling back to net.ListenPacket", network, address, err)
	return net.ListenPacket(network, address)
}

func isReuseportUnsupported(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, syscall.EINVAL) {
		return true
	}
	var opErr *net.OpError
	if errors.As(err, &opErr) && opErr.Err != nil {
		if errors.Is(opErr.Err, syscall.EINVAL) {
			return true
		}
		err = opErr.Err
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "invalid argument")
}

func (proxy *ProxyServer) Close() {
	log.Info().Msgf("Stopping proxy server")

	// Stop loops before closing sockets so readLoop does not busy-spin on
	// "use of closed network connection" while dead is still unset.
	proxy.dead.Set()

	// Stop UDP listeners (ping listeners may be nil when a DiscoveryHub owns them)
	if server := proxy.dataConn(); server != nil {
		_ = server.Close()
	}
	if proxy.pingServer != nil {
		_ = proxy.pingServer.Close()
	}
	if proxy.pingServerV6 != nil {
		_ = proxy.pingServerV6.Close()
	}

	// Close all connections
	if proxy.clientMap != nil {
		proxy.clientMap.Close()
	}
}

// RemoteServer returns the upstream address this proxy forwards to.
func (proxy *ProxyServer) RemoteServer() string {
	return proxy.prefs.RemoteServer
}

// HandleUnconnectedPing processes a discovery ping fanned out from a
// DiscoveryHub. Uses the dedicated discovery probe path (#117).
func (proxy *ProxyServer) HandleUnconnectedPing(data []byte, from net.Addr) error {
	if proxy.dead.IsSet() || proxy.dataConn() == nil {
		return fmt.Errorf("proxy not running")
	}
	if from == nil {
		return fmt.Errorf("nil client address")
	}
	if !proto.IsUnconnectedPing(data) {
		return fmt.Errorf("not an unconnected ping")
	}
	return proxy.handleDiscoveryPing(from, data)
}

func (proxy *ProxyServer) startWorkers(listener net.PacketConn) {
	log.Info().Msgf("Starting %d workers", proxy.prefs.NumWorkers)

	for i := uint(0); i < proxy.prefs.NumWorkers; i++ {
		if i < proxy.prefs.NumWorkers-1 {
			go proxy.readLoop(listener)
		} else {
			proxy.readLoop(listener)
		}
	}
}

// Continually reads data from the provided listener and passes it to
// processDataFromClients until the ProxyServer has been closed.
func (proxy *ProxyServer) readLoop(listener net.PacketConn) {
	log.Info().Msgf("Listener starting up: %s", listener.LocalAddr())

	packetBuffer := make([]byte, udpRecvBufferSize)

	for !proxy.dead.IsSet() {
		err := proxy.processDataFromClients(listener, packetBuffer)
		if err != nil {
			if proxy.dead.IsSet() {
				break
			}
			log.Warn().Msgf("Error while processing client data: %s", err)
		}
	}

	log.Info().Msgf("Listener shut down: %s", listener.LocalAddr())
}

// Inspects an incoming UDP packet, looking up the client in our connection
// map, lazily creating a new connection to the remote server when necessary,
// then forwarding the data to that remote connection.
//
// When a new client connects, an additional goroutine is created to read
// data from the server and send it back to the client.
func (proxy *ProxyServer) processDataFromClients(listener net.PacketConn, packetBuffer []byte) error {
	// Read the next packet from the client
	read, client, err := listener.ReadFrom(packetBuffer)
	if err != nil {
		return err
	}
	if read <= 0 {
		return nil
	}
	// A full buffer often means the kernel truncated a larger datagram
	// (and on some platforms ReadFrom still returns err == nil).
	if read == len(packetBuffer) {
		return fmt.Errorf("UDP datagram filled receive buffer (%d bytes); possible truncation", read)
	}

	data := packetBuffer[:read]
	log.Trace().Msgf("client recv: %v", data)

	// Drop echoes of our own DialUDP traffic. When -server points at this same
	// host's :19132 (typical LAN setup), SO_REUSEADDR can deliver our outbound
	// packets back to the ping listener. Proxying those again would open a new
	// UDP socket per echo until dial fails with "too many open files".
	if proxy.clientMap.IsOwnAddress(client) {
		return nil
	}

	// LAN discovery must not share the per-client DialUDP session used for
	// gameplay. After a console disconnects, that socket is often a stale
	// RakNet association: the remote ignores further Unconnected Pings, while
	// console re-pings keep refreshing SetReadDeadline + idle lastActive, so
	// the session never expires and the server vanishes from LAN until restart
	// (GitHub #117).
	//
	// The magic is checked, not just the packet ID: anything that only looks
	// like a ping is left alone and relayed upstream like any other datagram,
	// so garbage cannot divert data-plane traffic into a discovery probe.
	if proto.IsUnconnectedPing(data) {
		return proxy.handleDiscoveryPing(client, data)
	}

	// Handler triggered when a new client connects and we create a new connetion to the remote server
	onNewConnection := func(newServerConn *net.UDPConn) {
		log.Info().Msgf("New connection from client %s -> %s", client.String(), listener.LocalAddr())
		proxy.processDataFromServer(newServerConn, client)
	}

	serverConn, err := proxy.clientMap.Get(
		client,
		proxy.remoteServerAddress,
		onNewConnection,
	)

	if err != nil {
		return err
	}

	// Bound how long we wait for the server to say anything at all.
	_ = serverConn.SetReadDeadline(time.Now().Add(upstreamSessionTimeout))

	// Write packet from client to server
	_, err = serverConn.Write(data)
	if err == nil {
		return nil
	}

	// Write often consumes the pending ICMP error before processDataFromServer's
	// ReadFrom sees it. Meanwhile LAN pings keep refreshing SetReadDeadline and
	// ClientMap lastActive, so the session never times out and never marks the
	// server offline — producing a spam of "connection refused" warnings (#79).
	if isOfflineError(err) {
		proxy.markServerOffline()
		proxy.clientMap.Delete(client)
		return nil
	}

	return err
}

func isTimeoutError(err error) bool {
	if err == nil {
		return false
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	return strings.Contains(err.Error(), "timeout")
}

func isConnRefusedError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, syscall.ECONNREFUSED) {
		return true
	}
	return strings.Contains(strings.ToLower(err.Error()), "connection refused")
}

// noteUpstreamReadError updates offline state from a failed server read.
// Connection refused marks offline immediately. Timeouts only do so after
// offlineTimeoutThreshold consecutive failures so one flaky UDP deadline
// does not broadcast OfflinePong to every LAN client (#104).
func (proxy *ProxyServer) noteUpstreamReadError(err error) {
	log.Warn().Msgf("%v", err)

	if isConnRefusedError(err) {
		proxy.offlineMu.Lock()
		proxy.offlineTimeouts = 0
		proxy.markServerOfflineLocked()
		proxy.offlineMu.Unlock()
		return
	}
	if isTimeoutError(err) {
		proxy.offlineMu.Lock()
		proxy.offlineTimeouts++
		if proxy.offlineTimeouts >= offlineTimeoutThreshold {
			proxy.markServerOfflineLocked()
		}
		proxy.offlineMu.Unlock()
		return
	}
}

// handleDiscoveryPing answers a console/LAN Unconnected Ping from a recent
// upstream pong, probing the remote on a fresh socket when there isn't one.
// Either way this is independent of any gameplay session.
func (proxy *ProxyServer) handleDiscoveryPing(client net.Addr, ping []byte) error {
	log.Info().Msgf("Received LAN ping from client: %s", client.String())

	server := proxy.dataConn()
	if server == nil {
		return fmt.Errorf("proxy not running")
	}

	// Already offline: reply immediately. Waiting on a recovery probe made
	// OfflinePong arrive after console discovery timeouts — blackholed remotes
	// take the full deadline, so the LAN entry never appeared even though logs
	// later showed a pong was sent.
	if proxy.isServerOffline() {
		if _, err := server.WriteTo(proxy.buildOfflinePong(ping), client); err != nil {
			return err
		}
		log.Info().Msgf("Sent server offline pong to client: %v", client.String())
		return nil
	}

	// Consoles re-ping about once a second, and every console on the LAN pings
	// independently. Without this, each of those became its own upstream probe
	// on its own fresh socket, so phantom's ping rate at the remote scaled with
	// the number of consoles. A recent pong answers all of them.
	pong, cached := proxy.cachedPong()
	if !cached {
		var err error
		pong, err = proxy.probeRemoteUnconnectedPong(ping)
		if err != nil {
			proxy.markServerOffline()
			if _, werr := server.WriteTo(proxy.buildOfflinePong(ping), client); werr != nil {
				return werr
			}
			log.Info().Msgf("Sent server offline pong to client: %v", client.String())
			return nil
		}
	}

	proxy.noteUpstreamReachable()

	// Clients time their latency from the echoed ping time, so a pong served
	// from cache has to carry this client's value rather than the probe's.
	pong = stampPingTime(pong, ping)
	pong = proxy.rewriteUnconnectedPong(pong)
	if _, err := server.WriteTo(pong, client); err != nil {
		return err
	}
	log.Info().Msgf("Sent LAN pong to client: %v", client.String())
	return nil
}

// startUpstreamHealthCheck probes the remote on a timer so OfflinePong can be
// advertised before the first LAN ping, and so recovery does not require a
// client to wait out a synchronous probe.
func (proxy *ProxyServer) startUpstreamHealthCheck() {
	go func() {
		proxy.probeUpstreamHealth()
		ticker := time.NewTicker(discoveryHealthInterval())
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if proxy.dead.IsSet() {
					return
				}
				proxy.probeUpstreamHealth()
			}
		}
	}()
}

func (proxy *ProxyServer) probeUpstreamHealth() {
	if proxy.dead.IsSet() || proxy.dataConn() == nil {
		return
	}
	_, err := proxy.probeRemoteUnconnectedPong(proxy.healthCheckPing())
	if err != nil {
		proxy.markServerOffline()
		return
	}
	proxy.noteUpstreamReachable()
}

// healthCheckPing is a well-formed Unconnected Ping for background probes.
//
// The field order matters. RakNet puts the magic *before* the client GUID in a
// ping (the reverse of a pong), and implementations that validate the magic —
// RakLib/PocketMine and Nukkit among them — drop anything with it in the wrong
// place. A malformed probe here is invisible: the health check simply never
// gets a pong, the proxy latches "offline", and every LAN discovery ping is
// answered with OfflinePong even though the server is up and reachable.
func (proxy *ProxyServer) healthCheckPing() []byte {
	var pingTime [8]byte
	binary.BigEndian.PutUint64(pingTime[:], uint64(time.Now().UnixMilli()))

	var guid [8]byte
	binary.BigEndian.PutUint64(guid[:], uint64(proxy.serverID))

	return proto.BuildUnconnectedPing(pingTime[:], guid[:])
}

// probeRemoteUnconnectedPong dials a one-shot UDP socket to the remote and
// waits for an Unconnected Pong. A fresh local port avoids stale RakNet state.
func (proxy *ProxyServer) probeRemoteUnconnectedPong(ping []byte) ([]byte, error) {
	conn, err := net.DialUDP("udp", nil, proxy.remoteServerAddress)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	timeout := discoveryPingTimeout()
	if proxy.isServerOffline() {
		timeout = discoveryRecoveryProbeTimeout()
	}
	_ = conn.SetDeadline(time.Now().Add(timeout))
	if _, err := conn.Write(ping); err != nil {
		return nil, err
	}

	buf := make([]byte, udpRecvBufferSize)
	for {
		n, err := conn.Read(buf)
		if err != nil {
			return nil, err
		}
		if n < 1 {
			continue
		}
		if n == len(buf) {
			continue
		}
		if buf[0] != proto.UnconnectedPongID {
			continue
		}
		out := make([]byte, n)
		copy(out, buf[:n])
		proxy.recordPong(out)
		return out, nil
	}
}

// recordPong remembers the newest upstream pong. It backs both the discovery
// cache and the offline advertisement.
func (proxy *ProxyServer) recordPong(pong []byte) {
	// Only keep pongs phantom can actually parse; a frame it cannot read is no
	// use as an offline template and must not be replayed to other clients.
	if _, err := proto.ReadUnconnectedPing(pong); err != nil {
		return
	}
	proxy.pongMu.Lock()
	defer proxy.pongMu.Unlock()
	// Store a copy: callers stamp their client's ping time into the frame they
	// were handed, and the cached entry must not move under them.
	proxy.lastPong = append([]byte(nil), pong...)
	proxy.lastPongTime = time.Now()
}

// cachedPong returns a copy of the last upstream pong if it is fresh enough to
// answer a discovery ping with.
func (proxy *ProxyServer) cachedPong() ([]byte, bool) {
	ttl := discoveryPongCacheTTL()
	if ttl <= 0 {
		return nil, false
	}

	proxy.pongMu.Lock()
	defer proxy.pongMu.Unlock()

	if proxy.lastPong == nil || time.Since(proxy.lastPongTime) > ttl {
		return nil, false
	}
	return append([]byte(nil), proxy.lastPong...), true
}

// lastKnownPong returns a copy of the most recent upstream pong regardless of
// age, or nil if the remote has never answered.
func (proxy *ProxyServer) lastKnownPong() []byte {
	proxy.pongMu.Lock()
	defer proxy.pongMu.Unlock()

	if proxy.lastPong == nil {
		return nil
	}
	return append([]byte(nil), proxy.lastPong...)
}

// stampPingTime writes the client's ping time into a pong so it can be matched
// to the request it answers.
func stampPingTime(pong, ping []byte) []byte {
	if len(pong) >= 9 && len(ping) >= 9 {
		copy(pong[1:9], ping[1:9])
	}
	return pong
}

func (proxy *ProxyServer) markServerOffline() {
	proxy.offlineMu.Lock()
	defer proxy.offlineMu.Unlock()
	proxy.markServerOfflineLocked()
}

func (proxy *ProxyServer) markServerOfflineLocked() {
	if proxy.serverOffline {
		return
	}
	log.Warn().Msgf("Server seems to be offline :(")
	log.Warn().Msgf("We'll keep trying to connect...")
	proxy.serverOffline = true
}

func (proxy *ProxyServer) noteUpstreamReachable() {
	proxy.offlineMu.Lock()
	defer proxy.offlineMu.Unlock()
	proxy.offlineTimeouts = 0
	if proxy.serverOffline {
		log.Info().Msgf("Server is back online!")
		proxy.serverOffline = false
	}
}

func (proxy *ProxyServer) isServerOffline() bool {
	proxy.offlineMu.Lock()
	defer proxy.offlineMu.Unlock()
	return proxy.serverOffline
}

// buildOfflinePong returns the offline advertisement, echoing the client's ping
// time so consoles can match the response to their request.
func (proxy *ProxyServer) buildOfflinePong(ping []byte) []byte {
	return proxy.rewriteUnconnectedPong(stampPingTime(proxy.offlinePongTemplate(), ping))
}

// offlinePongTemplate is the pong body used while the remote is unreachable.
//
// It is derived from the last pong the remote actually sent, so the LAN entry
// keeps that server's real edition, protocol number and version name and only
// the MOTD changes. Advertising a fixed protocol instead would make the offline
// entry claim a version unrelated to the server behind it, and one that ages
// out of every client's compatible range as Mojang ships new releases.
func (proxy *ProxyServer) offlinePongTemplate() []byte {
	last := proxy.lastKnownPong()
	if last == nil {
		return append([]byte(nil), proto.OfflinePong.Bytes()...)
	}

	packet, err := proto.ReadUnconnectedPing(last)
	if err != nil {
		return append([]byte(nil), proto.OfflinePong.Bytes()...)
	}

	packet.Pong.MOTD = proto.OfflineMOTD
	packet.Pong.Players = "0"
	// Non-zero capacity: some consoles omit full/zero-slot entries from Friends.
	if packet.Pong.MaxPlayers == "" || packet.Pong.MaxPlayers == "0" {
		packet.Pong.MaxPlayers = "1"
	}

	buf := packet.Build()
	return buf.Bytes()
}

// Proxies packets sent by the server to us for a specific Minecraft client back to
// that client's UDP connection.
func (proxy *ProxyServer) processDataFromServer(remoteConn *net.UDPConn, client net.Addr) {
	buffer := make([]byte, udpRecvBufferSize)

	for !proxy.dead.IsSet() {
		// Read the next packet from the server
		read, _, err := remoteConn.ReadFrom(buffer)

		// Remove read timeout, server responded
		_ = remoteConn.SetReadDeadline(time.Time{})

		// Read error
		if err != nil {
			// Conn closed by idle cleanup / offline write-path Delete — expected.
			if errors.Is(err, net.ErrClosed) {
				break
			}

			// Game-session read errors (including the deadline that fires after
			// a console disconnect) are not a reliable "server offline" signal.
			// LAN discovery uses a dedicated probe (#117). Connection refused
			// on an active session still marks offline (#79/#104).
			if isConnRefusedError(err) {
				proxy.noteUpstreamReadError(err)
			} else if !offlineErrorRegex.MatchString(err.Error()) {
				log.Warn().Msgf("%v", err)
			}
			break
		}

		// Empty read
		if read < 1 {
			continue
		}

		if read == len(buffer) {
			log.Warn().Msgf("UDP datagram from server filled receive buffer (%d bytes); dropping possibly truncated packet", read)
			continue
		}

		proxy.noteUpstreamReachable()

		// Resize data to byte count from 'read'
		data := buffer[:read]
		log.Trace().Msgf("server recv: %v", data)

		if server := proxy.dataConn(); server != nil {
			server.WriteTo(data, client)
		}
	}

	proxy.clientMap.Delete(client)
}

func (proxy *ProxyServer) rewriteUnconnectedPong(data []byte) []byte {
	log.Debug().Msgf("Received Unconnected Pong from server: %v", data)

	if packet, err := proto.ReadUnconnectedPing(data); err == nil {
		// Overwrite the server ID with one unique to this proxy.
		// If we don't do this, the client will get confused if you restart phantom,
		// and multiple -server backends in one process would look identical.
		id := make([]byte, 8)
		binary.BigEndian.PutUint64(id, uint64(proxy.serverID))
		packet.ID = id
		packet.Pong.ServerID = fmt.Sprintf("%d", proxy.serverID)

		// Always advertise phantom's bind port. Upstream MOTDs (notably Geyser)
		// often omit Port4/Port6; leaving them empty makes consoles fall back to
		// 19132 or skip the LAN/Friends entry entirely.
		if proxy.prefs.RemovePorts {
			packet.Pong.Port4 = ""
			packet.Pong.Port6 = ""
		} else {
			packet.Pong.Port4 = fmt.Sprintf("%d", proxy.boundPort)
			packet.Pong.Port6 = packet.Pong.Port4
		}

		packetBuffer := packet.Build()
		log.Debug().Msgf("Unconnected Pong: %v", packet)
		return packetBuffer.Bytes()
	} else {
		log.Warn().Msgf("Failed to rewrite pong: %v", err)
	}

	return data
}
