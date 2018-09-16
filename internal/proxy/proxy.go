package proxy

import (
	"fmt"
	"log"
	"net"
	"os"
	"time"

	"github.com/jhead/phantom/internal/proto"
)

const maxMTU = 1472

var logger = log.New(os.Stdout, "INFO: ", log.Ldate|log.Ltime|log.Lshortfile)
var idleTimeout = time.Duration(120) * time.Second
var idleCheckInterval = time.Duration(5) * time.Second

type clientMap map[string]*net.UDPConn
type idleMap map[string]time.Time

type ProxyServer struct {
	bindAddress         *net.UDPAddr
	remoteServerAddress *net.UDPAddr
	server              *net.UDPConn
	clients             clientMap
	idleMap             idleMap
	pingChannel         chan PingPacket
	dead                bool
}

type PingPacket struct {
	client net.Addr
	data   []byte
}

func New(bind, remoteServer string) (*ProxyServer, error) {
	bindAddress, err := net.ResolveUDPAddr("udp", bind)
	if err != nil {
		return nil, fmt.Errorf("Invalid bind address: %s", err)
	}

	remoteServerAddress, err := net.ResolveUDPAddr("udp", remoteServer)
	if err != nil {
		return nil, fmt.Errorf("Invalid server address: %s", err)
	}

	return &ProxyServer{
		bindAddress,
		remoteServerAddress,
		nil,
		make(clientMap),
		make(idleMap),
		make(chan PingPacket),
		false,
	}, nil
}

func (proxy *ProxyServer) Start() error {
	logger.Printf("Binding proxy server to: %v\n", proxy.bindAddress)

	// Bind local UDP port to receive data from Minecraft clients
	if server, err := net.ListenUDP("udp", proxy.bindAddress); err == nil {
		// Save UDP server handle
		proxy.server = server
	} else {
		// Bind failed
		return err
	}

	logger.Printf("Proxy server listening!")

	// Close connection upon exit
	defer proxy.server.Close()

	// Start goroutine for handling ping broadcasts
	go proxy.handleBroadcastPackets()

	// Start goroutine for cleaning up idle connections
	go proxy.idleConnectionCleanup()

	// Start processing packets from Minecraft clients
	proxy.clientLoop()

	return nil
}

func (proxy *ProxyServer) Stop() {
	logger.Println("Stopping proxy server")

	// Stop UDP listener
	proxy.server.Close()

	// Close all connections
	for _, conn := range proxy.clients {
		conn.Close()
	}

	// Stop loops
	proxy.dead = true
}

func (proxy *ProxyServer) clientLoop() {
	packetBuffer := make([]byte, maxMTU)

	for !proxy.dead {
		err := proxy.processDataFromClients(packetBuffer)
		if err != nil {
			logger.Printf("Error while processing client data: %s\n", err)
		}
	}

	logger.Println("Proxy server shut down")
}

func (proxy *ProxyServer) processDataFromClients(packetBuffer []byte) error {
	read, client, _ := proxy.server.ReadFrom(packetBuffer)
	if read <= 0 {
		return nil
	}

	data := packetBuffer[:read]
	packetID := data[0]

	// Broadcasted ping packet; offload to our ping handler
	if packetID == proto.UnconnectedRequestID {
		proxy.pingChannel <- PingPacket{client, data}
		return nil
	}

	// All other packets should be proxied
	conn, err := proxy.getServerConnection(client)
	if err != nil {
		return err
	}

	// Write packet from client to server
	_, err = conn.Write(data)
	return err
}

// getServerConnection gets or creates a new UDP connection to the remote server
// and stores it in a map, matching clients to remote server connections.
// This way, we keep one UDP connection open to the server for each Minecraft
// client that's connected to the proxy.
func (proxy *ProxyServer) getServerConnection(client net.Addr) (*net.UDPConn, error) {
	key := client.String()

	// Store time for cleanup later
	proxy.idleMap[key] = time.Now()

	// Connection exists
	if conn, ok := proxy.clients[key]; ok {
		return conn, nil
	}

	// New connection needed
	logger.Printf("Opening connection to %s for new client %s!\n", proxy.remoteServerAddress, client)

	conn, err := net.DialUDP("udp", nil, proxy.remoteServerAddress)
	if err != nil {
		return nil, err
	}

	proxy.clients[key] = conn

	// Launch goroutine to pass packets from server to client
	go proxy.processDataFromServer(conn, client)

	return conn, nil
}

// processDataFromServer proxies packets sent by the server to us for a specific
// Minecraft client back to that client's UDP connection.
func (proxy *ProxyServer) processDataFromServer(remoteConn *net.UDPConn, client net.Addr) {
	buffer := make([]byte, maxMTU)

	for !proxy.dead {
		read, _, err := remoteConn.ReadFrom(buffer)

		if err != nil {
			fmt.Println(err)
			break
		}

		data := buffer[:read]
		proxy.server.WriteTo(data, client)
	}
}

func (proxy *ProxyServer) handleBroadcastPackets() {
	logger.Println("Starting ping handler")

	for !proxy.dead {
		packet := <-proxy.pingChannel
		id := packet.data[1:9]
		magic := packet.data[9:25]

		logger.Printf("Ping from %v!\n", packet.client)

		serverName := fmt.Sprintf("MCPE;Remote Server %s;2 7;0.11.0;0;20", proxy.remoteServerAddress)
		replyBuffer := proto.UnconnectedReply{id, magic, serverName}.Build()

		proxy.server.WriteTo(replyBuffer.Bytes(), packet.client)
	}
}

func (proxy *ProxyServer) idleConnectionCleanup() {
	logger.Println("Starting idle connection handler")

	for !proxy.dead {
		currentTime := time.Now()

		for key, lastActive := range proxy.idleMap {
			if lastActive.Add(idleTimeout).Before(currentTime) {
				logger.Printf("Cleaning up idle connection: %s", key)
				proxy.clients[key].Close()
				delete(proxy.clients, key)
				delete(proxy.idleMap, key)
			}
		}

		time.Sleep(idleCheckInterval)
	}
}
