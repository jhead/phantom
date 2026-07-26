package proxy

import (
	"errors"
	"fmt"
	"math/rand"
	"net"
	"regexp"
	"strings"
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

var idleCheckInterval = 5 * time.Second

type ProxyServer struct {
	bindAddress         *net.UDPAddr
	boundPort           uint16
	remoteServerAddress *net.UDPAddr
	pingServer          net.PacketConn
	pingServerV6        net.PacketConn
	server              *net.UDPConn
	clientMap           *clientmap.ClientMap
	prefs               ProxyPrefs
	dead                *abool.AtomicBool
	serverOffline       bool
	offlineTimeouts     int
}

type ProxyPrefs struct {
	BindAddress  string
	BindPort     uint16
	RemoteServer string
	IdleTimeout  time.Duration
	EnableIPv6   bool
	RemovePorts  bool
	NumWorkers   uint
}

var randSource = rand.NewSource(time.Now().UnixNano())
var serverID = randSource.Int63()
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

	// Randomize port if not provided
	if bindPort == 0 {
		bindPort = (uint16(randSource.Int63()) % 14000) + 50000
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
		bindAddress,
		bindPort,
		remoteServerAddress,
		nil,
		nil,
		nil,
		clientmap.New(prefs.IdleTimeout, idleCheckInterval),
		prefs,
		abool.New(),
		false,
		0,
	}, nil
}

func (proxy *ProxyServer) Start() error {
	// Bind to 19132 on all addresses to receive broadcasted pings.
	// Prefer reuseport when available; fall back on iSH and similar envs
	// that reject SO_REUSEPORT with EINVAL (#94 / #108).
	log.Info().Msgf("Binding ping server to port 19132")
	pingServer, err := listenPacket("udp4", ":19132")
	if err != nil {
		return err
	}
	proxy.pingServer = pingServer
	go proxy.readLoop(proxy.pingServer)

	// Minecraft automatically broadcasts on port 19133 to the local IPv6 network
	if proxy.prefs.EnableIPv6 {
		log.Info().Msgf("Binding IPv6 ping server to port 19133")
		if pingServerV6, err := listenPacket("udp6", ":19133"); err == nil {
			proxy.pingServerV6 = pingServerV6
			go proxy.readLoop(proxy.pingServerV6)
		} else {
			log.Warn().Msgf("Failed to bind IPv6 ping listener: %v", err)
		}
	}

	network := "udp4"
	if proxy.prefs.EnableIPv6 {
		network = "udp"
	}

	// Bind to specified UDP addr and port to receive data from Minecraft clients
	log.Info().Msgf("Binding proxy server to: %v", proxy.bindAddress)
	server, err := listenPacket(network, proxy.bindAddress.String())
	if err != nil {
		return err
	}
	// a safe cast, I promise
	proxy.server = server.(*net.UDPConn)

	log.Info().Msgf("Proxy server listening!")
	log.Info().Msgf("Once your console pings phantom, you should see replies below.")

	// Start processing everything else using the proxy listener
	proxy.startWorkers(proxy.server)

	return nil
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

	// Stop UDP listeners
	proxy.server.Close()
	proxy.pingServer.Close()

	if proxy.pingServerV6 != nil {
		proxy.pingServerV6.Close()
	}

	// Close all connections
	proxy.clientMap.Close()
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

	// Wait 5 seconds for the server to respond to whatever we sent, or else timeout
	_ = serverConn.SetReadDeadline(time.Now().Add(time.Second * 5))

	if packetID := data[0]; packetID == proto.UnconnectedPingID {
		log.Info().Msgf("Received LAN ping from client: %s", client.String())

		if proxy.serverOffline {
			replyBuffer := proto.OfflinePong
			replyBytes := proxy.rewriteUnconnectedPong(replyBuffer.Bytes())

			proxy.server.WriteTo(replyBytes, client)
			log.Info().Msgf("Sent server offline pong to client: %v", client.String())
		}

		// Pass ping through to server even if it's offline
	}

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
		proxy.offlineTimeouts = 0
		proxy.markServerOffline()
		return
	}
	if isTimeoutError(err) {
		proxy.offlineTimeouts++
		if proxy.offlineTimeouts >= offlineTimeoutThreshold {
			proxy.markServerOffline()
		}
		return
	}
}

func (proxy *ProxyServer) markServerOffline() {
	if proxy.serverOffline {
		return
	}
	log.Warn().Msgf("Server seems to be offline :(")
	log.Warn().Msgf("We'll keep trying to connect...")
	proxy.serverOffline = true
}

func (proxy *ProxyServer) noteUpstreamReachable() {
	proxy.offlineTimeouts = 0
	if proxy.serverOffline {
		log.Info().Msgf("Server is back online!")
		proxy.serverOffline = false
	}
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

			proxy.noteUpstreamReadError(err)
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

		// Rewrite Unconnected Pong packets
		if packetID := data[0]; packetID == proto.UnconnectedPongID {
			data = proxy.rewriteUnconnectedPong(data)
			log.Info().Msgf("Sent LAN pong to client: %v", client.String())
		}

		proxy.server.WriteTo(data, client)
	}

	proxy.clientMap.Delete(client)
}

func (proxy *ProxyServer) rewriteUnconnectedPong(data []byte) []byte {
	log.Debug().Msgf("Received Unconnected Pong from server: %v", data)

	if packet, err := proto.ReadUnconnectedPing(data); err == nil {
		// Overwrite the server ID with one unique to this phantom instance.
		// If we don't do this, the client will get confused if you restart phantom.
		packet.Pong.ServerID = fmt.Sprintf("%d", serverID)

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
