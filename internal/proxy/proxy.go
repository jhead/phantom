package proxy

import (
	"fmt"
	"math/rand"
	"net"
	"time"

	"github.com/jhead/phantom/internal/clientmap"
	"github.com/jhead/phantom/internal/logging"
	"github.com/jhead/phantom/internal/proto"
	"github.com/tevino/abool"

	reuse "github.com/libp2p/go-reuseport"
)

const maxMTU = 1472

var logger = logging.Get()
var idleCheckInterval = 5 * time.Second

type ProxyServer struct {
	bindAddress         *net.UDPAddr
	remoteServerAddress *net.UDPAddr
	pingServer          net.PacketConn
	server              *net.UDPConn
	clientMap           *clientmap.ClientMap
	prefs               ProxyPrefs
	dead                *abool.AtomicBool
}

type ProxyPrefs struct {
	BindAddress  string
	BindPort     uint16
	RemoteServer string
	IdleTimeout  time.Duration
}

func New(prefs ProxyPrefs) (*ProxyServer, error) {
	bindPort := prefs.BindPort

	// Randomize port if not provided
	if bindPort == 0 {
		randSource := rand.NewSource(time.Now().UnixNano())
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
		remoteServerAddress,
		nil,
		nil,
		clientmap.New(prefs.IdleTimeout, idleCheckInterval),
		prefs,
		abool.New(),
	}, nil
}

func (proxy *ProxyServer) Start() error {
	// Bind to 19132 on all addresses to receive broadcasted pings
	// Sets SO_REUSEADDR et al to support multiple instances of phantom
	logger.Printf("Binding ping server to: %v\n", "0.0.0.0:19132")
	if pingServer, err := reuse.ListenPacket("udp", "0.0.0.0:19132"); err == nil {
		proxy.pingServer = pingServer
	} else {
		// Bind failed
		return err
	}

	// Bind to specified UDP addr and port to receive data from Minecraft clients
	logger.Printf("Binding proxy server to: %v\n", proxy.bindAddress)
	if server, err := net.ListenUDP("udp", proxy.bindAddress); err == nil {
		proxy.server = server
	} else {
		return err
	}

	logger.Printf("Proxy server listening!")

	// Start proxying ping packets from the broadcast listener
	go proxy.readLoop(proxy.pingServer)

	// Start processing everything else using the proxy listener
	proxy.readLoop(proxy.server)

	return nil
}

func (proxy *ProxyServer) Close() {
	logger.Println("Stopping proxy server")

	// Stop UDP listeners
	proxy.server.Close()
	proxy.pingServer.Close()

	// Close all connections
	proxy.clientMap.Close()

	// Stop loops
	proxy.dead.Set()
}

// Continually reads data from the provided listener and passes it to
// processDataFromClients until the ProxyServer has been closed.
func (proxy *ProxyServer) readLoop(listener net.PacketConn) {
	packetBuffer := make([]byte, maxMTU)

	for !proxy.dead.IsSet() {
		err := proxy.processDataFromClients(listener, packetBuffer)
		if err != nil {
			logger.Printf("Error while processing client data: %s\n", err)
		}
	}

	logger.Printf("Listener shut down: %s\n", listener.LocalAddr())
}

// Inspects an incoming UDP packet, looking up the client in our connection
// map, lazily creating a new connection to the remote server when necessary,
// then forwarding the data to that remote connection.
//
// When a new client connects, an additional goroutine is created to read
// data from the server and send it back to the client.
func (proxy *ProxyServer) processDataFromClients(listener net.PacketConn, packetBuffer []byte) error {
	read, client, _ := listener.ReadFrom(packetBuffer)
	if read <= 0 {
		return nil
	}

	data := packetBuffer[:read]

	// Handler triggered when a new client connects and we create a new connetion to the remote server
	onNewConnection := func(newServerConn *net.UDPConn) {
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

	// Write packet from client to server
	_, err = serverConn.Write(data)
	return err
}

// Proxies packets sent by the server to us for a specific Minecraft client back to
// that client's UDP connection.
func (proxy *ProxyServer) processDataFromServer(remoteConn *net.UDPConn, client net.Addr) {
	buffer := make([]byte, maxMTU)

	for !proxy.dead.IsSet() {
		read, _, err := remoteConn.ReadFrom(buffer)

		// Read error
		if err != nil {
			fmt.Println(err)
			break
		}

		// Empty read
		if read < 1 {
			continue
		}

		// Resize data to byte count from 'read'
		data := buffer[:read]

		// Rewrite Unconnected Reply packets
		if packetID := data[0]; packetID == proto.UnconnectedReplyID {
			if packet, err := proto.ReadUnconnectedReply(data); err == nil {
				// Rewrite server MOTD to remove ports
				packetBuffer := packet.Build()
				data = packetBuffer.Bytes()
			} else {
				fmt.Printf("Failed to rewrite pong: %v\n", err)
			}
		}

		proxy.server.WriteTo(data, client)
	}
}
