package proxy

import (
	"encoding/binary"
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
	serverID            int64
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

var randSource = rand.NewSource(time.Now().UnixNano())
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
		randSource.Int63(),
	}, nil
}

func (proxy *ProxyServer) Start() error {
	if err := proxy.listen(); err != nil {
		return err
	}

	// Start processing everything else using the proxy listener
	proxy.startWorkers(proxy.server)

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
		go proxy.readLoop(proxy.server)
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
	return nil
}

// RemoteServer returns the upstream address this proxy forwards to.
func (proxy *ProxyServer) RemoteServer() string {
	return proxy.prefs.RemoteServer
}

func (proxy *ProxyServer) Close() {
	log.Info().Msgf("Stopping proxy server")

	// Stop UDP listeners
	if proxy.server != nil {
		proxy.server.Close()
	}
	if proxy.pingServer != nil {
		proxy.pingServer.Close()
	}

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

// HandleUnconnectedPing processes a discovery ping fanned out from a
// DiscoveryHub-owned :19132 listener. Replies are sent from this proxy's data port.
func (proxy *ProxyServer) HandleUnconnectedPing(data []byte, from net.Addr) error {
	if proxy.dead.IsSet() || proxy.server == nil {
		return fmt.Errorf("proxy not running")
	}
	if from == nil {
		return fmt.Errorf("nil client address")
	}
	if len(data) < 1 || data[0] != proto.UnconnectedPingID {
		return fmt.Errorf("not an unconnected ping")
	}
	return proxy.handleClientPacket(from, data)
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

	return proxy.handleClientPacket(client, packetBuffer[:read])
}

// handleClientPacket is the shared discovery/data-plane path used by both the
// local UDP read loops and DiscoveryHub fan-out via HandleUnconnectedPing.
func (proxy *ProxyServer) handleClientPacket(client net.Addr, data []byte) error {
	log.Trace().Msgf("client recv: %v", data)

	// Handler triggered when a new client connects and we create a new connetion to the remote server
	onNewConnection := func(newServerConn *net.UDPConn) {
		log.Info().Msgf("New connection from client %s -> %s", client.String(), proxy.bindAddress)
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

	if len(data) > 0 && data[0] == proto.UnconnectedPingID {
		log.Info().Msgf("Received LAN ping from client: %s", client.String())

		if proxy.serverOffline {
			replyBuffer := proto.OfflinePong
			replyBytes := proxy.rewriteUnconnectedPong(replyBuffer.Bytes())

			if proxy.server != nil {
				_, _ = proxy.server.WriteTo(replyBytes, client)
			}
			log.Info().Msgf("Sent server offline pong to client: %v", client.String())
		}

		// Pass ping through to server even if it's offline
	}

	// Write packet from client to server
	_, err = serverConn.Write(data)
	return err
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
			log.Warn().Msgf("%v", err)

			offlineError := offlineErrorRegex.MatchString(err.Error())

			if offlineError && !proxy.serverOffline {
				log.Warn().Msgf("Server seems to be offline :(")
				log.Warn().Msgf("We'll keep trying to connect...")
				proxy.serverOffline = true
			}

			break
		}

		// Empty read
		if read < 1 {
			continue
		}

		if proxy.serverOffline {
			log.Info().Msgf("Server is back online!")
			proxy.serverOffline = false
		}

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
		// Overwrite the server ID with one unique to this proxy.
		// If we don't do this, the client will get confused if you restart phantom,
		// and multiple -server backends in one process would look identical.
		id := make([]byte, 8)
		binary.BigEndian.PutUint64(id, uint64(proxy.serverID))
		packet.ID = id
		packet.Pong.ServerID = fmt.Sprintf("%d", proxy.serverID)

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
