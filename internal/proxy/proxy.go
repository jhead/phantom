package proxy

import (
	"fmt"
	"math/rand"
	"net"
	"regexp"
	"time"

	"github.com/jhead/phantom/internal/clientmap"
	"github.com/jhead/phantom/internal/proto"
	"github.com/rs/zerolog/log"
	"github.com/tevino/abool"

	reuse "github.com/libp2p/go-reuseport"
)

const maxMTU = 1472

var idleCheckInterval = 5 * time.Second
var discoveryPingTimeout = 5 * time.Second

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
	}, nil
}

func (proxy *ProxyServer) Start() error {
	// Bind to 19132 on all addresses to receive broadcasted pings
	// Sets SO_REUSEADDR et al to support multiple instances of phantom
	log.Info().Msgf("Binding ping server to port 19132")
	if pingServer, err := reuse.ListenPacket("udp4", ":19132"); err == nil {
		proxy.pingServer = pingServer

		// Start proxying ping packets from the broadcast listener
		go proxy.readLoop(proxy.pingServer)
	} else {
		// Bind failed
		return err
	}

	// Minecraft automatically broadcasts on port 19133 to the local IPv6 network
	if proxy.prefs.EnableIPv6 {
		log.Info().Msgf("Binding IPv6 ping server to port 19133")
		if pingServerV6, err := reuse.ListenPacket("udp6", ":19133"); err == nil {
			proxy.pingServerV6 = pingServerV6

			// Start proxying ping packets from the broadcast listener
			go proxy.readLoop(proxy.pingServerV6)
		} else {
			// IPv6 Bind failed
			log.Warn().Msgf("Failed to bind IPv6 ping listener: %v", err)
		}
	}

	network := "udp4"
	if proxy.prefs.EnableIPv6 {
		network = "udp"
	}

	// Bind to specified UDP addr and port to receive data from Minecraft clients
	log.Info().Msgf("Binding proxy server to: %v", proxy.bindAddress)
	if server, err := reuse.ListenPacket(network, proxy.bindAddress.String()); err == nil {
		// a safe cast, I promise
		proxy.server = server.(*net.UDPConn)
	} else {
		return err
	}

	log.Info().Msgf("Proxy server listening!")
	log.Info().Msgf("Once your console pings phantom, you should see replies below.")

	// Start processing everything else using the proxy listener
	proxy.startWorkers(proxy.server)

	return nil
}

func (proxy *ProxyServer) Close() {
	log.Info().Msgf("Stopping proxy server")

	// Stop UDP listeners
	proxy.server.Close()
	proxy.pingServer.Close()

	if proxy.pingServerV6 != nil {
		proxy.pingServerV6.Close()
	}

	// Close all connections
	proxy.clientMap.Close()

	// Stop loops
	proxy.dead.Set()
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

	packetBuffer := make([]byte, maxMTU)

	for !proxy.dead.IsSet() {
		err := proxy.processDataFromClients(listener, packetBuffer)
		if err != nil {
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
	read, client, _ := listener.ReadFrom(packetBuffer)
	if read <= 0 {
		return nil
	}

	data := packetBuffer[:read]
	log.Trace().Msgf("client recv: %v", data)

	// LAN discovery must not share the per-client DialUDP session used for
	// gameplay. After a console disconnects, that socket is often a stale
	// RakNet association: the remote ignores further Unconnected Pings, while
	// console re-pings keep refreshing SetReadDeadline + idle lastActive, so
	// the session never expires and the server vanishes from LAN until restart
	// (GitHub #117).
	if data[0] == proto.UnconnectedPingID {
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

	// Wait 5 seconds for the server to respond to whatever we sent, or else timeout
	_ = serverConn.SetReadDeadline(time.Now().Add(time.Second * 5))

	// Write packet from client to server
	_, err = serverConn.Write(data)
	return err
}

// handleDiscoveryPing answers a console/LAN Unconnected Ping using a fresh
// probe to the remote server, independent of any gameplay session.
func (proxy *ProxyServer) handleDiscoveryPing(client net.Addr, ping []byte) error {
	log.Info().Msgf("Received LAN ping from client: %s", client.String())

	pong, err := proxy.probeRemoteUnconnectedPong(ping)
	if err != nil {
		proxy.markServerOffline()
		if _, werr := proxy.server.WriteTo(proxy.buildOfflinePong(ping), client); werr != nil {
			return werr
		}
		log.Info().Msgf("Sent server offline pong to client: %v", client.String())
		return nil
	}

	if proxy.serverOffline {
		log.Info().Msgf("Server is back online!")
		proxy.serverOffline = false
	}

	pong = proxy.rewriteUnconnectedPong(pong)
	if _, err := proxy.server.WriteTo(pong, client); err != nil {
		return err
	}
	log.Info().Msgf("Sent LAN pong to client: %v", client.String())
	return nil
}

// probeRemoteUnconnectedPong dials a one-shot UDP socket to the remote and
// waits for an Unconnected Pong. A fresh local port avoids stale RakNet state.
func (proxy *ProxyServer) probeRemoteUnconnectedPong(ping []byte) ([]byte, error) {
	conn, err := net.DialUDP("udp", nil, proxy.remoteServerAddress)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	_ = conn.SetDeadline(time.Now().Add(discoveryPingTimeout))
	if _, err := conn.Write(ping); err != nil {
		return nil, err
	}

	buf := make([]byte, maxMTU)
	for {
		n, err := conn.Read(buf)
		if err != nil {
			return nil, err
		}
		if n < 1 {
			continue
		}
		if buf[0] != proto.UnconnectedPongID {
			continue
		}
		out := make([]byte, n)
		copy(out, buf[:n])
		return out, nil
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

// buildOfflinePong returns the canned offline advertisement, echoing the
// client's ping time so consoles can match the response to their request.
func (proxy *ProxyServer) buildOfflinePong(ping []byte) []byte {
	reply := append([]byte(nil), proto.OfflinePong.Bytes()...)
	if len(ping) >= 9 && len(reply) >= 9 {
		copy(reply[1:9], ping[1:9])
	}
	return proxy.rewriteUnconnectedPong(reply)
}

// Proxies packets sent by the server to us for a specific Minecraft client back to
// that client's UDP connection.
func (proxy *ProxyServer) processDataFromServer(remoteConn *net.UDPConn, client net.Addr) {
	buffer := make([]byte, maxMTU)

	for !proxy.dead.IsSet() {
		// Read the next packet from the server
		read, _, err := remoteConn.ReadFrom(buffer)

		// Remove read timeout, server responded
		_ = remoteConn.SetReadDeadline(time.Time{})

		// Read error
		if err != nil {
			// Game-session read errors (including the 5s deadline after a
			// console disconnect) are not a reliable "server offline" signal.
			// LAN discovery uses a dedicated probe (#117).
			if !offlineErrorRegex.MatchString(err.Error()) {
				log.Warn().Msgf("%v", err)
			}
			break
		}

		// Empty read
		if read < 1 {
			continue
		}

		// Resize data to byte count from 'read'
		data := buffer[:read]
		log.Trace().Msgf("server recv: %v", data)

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

		// Overwrite port numbers sent back from server (if any)
		if packet.Pong.Port4 != "" && !proxy.prefs.RemovePorts {
			packet.Pong.Port4 = fmt.Sprintf("%d", proxy.boundPort)
			packet.Pong.Port6 = packet.Pong.Port4
		} else if proxy.prefs.RemovePorts {
			packet.Pong.Port4 = ""
			packet.Pong.Port6 = ""
		}

		packetBuffer := packet.Build()
		log.Debug().Msgf("Unconnected Pong: %v", packet)
		return packetBuffer.Bytes()
	} else {
		log.Warn().Msgf("Failed to rewrite pong: %v", err)
	}

	return data
}
