package proxy

import (
	"fmt"
	"log"
	"math/rand"
	"net"
	"os"
	"strings"
	"time"

	"github.com/jhead/phantom/internal/proto"

	reuse "github.com/libp2p/go-reuseport"
)

const maxMTU = 1472

var logger = log.New(os.Stdout, "INFO: ", log.Ldate|log.Ltime|log.Lshortfile)
var idleCheckInterval = time.Duration(5) * time.Second

type clientMap map[string]*net.UDPConn
type idleMap map[string]time.Time

type ProxyServer struct {
	bindAddress         *net.UDPAddr
	remoteServerAddress *net.UDPAddr
	pingServer          net.PacketConn
	server              *net.UDPConn
	clients             clientMap
	idleMap             idleMap
	lookupChan          chan connLookup
	dead                bool
	prefs               ProxyPrefs
}

type ProxyPrefs struct {
	BindAddress  string
	RemoteServer string
	IdleTimeout  time.Duration
}

type connLookup struct {
	client       net.Addr
	responseChan chan connResponse
}

type connResponse struct {
	conn *net.UDPConn
	err  error
}

func New(prefs ProxyPrefs) (*ProxyServer, error) {
	if strings.ContainsAny(prefs.BindAddress, ":") {
		prefs.BindAddress = prefs.BindAddress
	} else {
		randSource := rand.NewSource(time.Now().UnixNano())
		randomPort := (uint16(randSource.Int63()) % 14000) + 50000
		prefs.BindAddress = fmt.Sprintf("%s:%d", prefs.BindAddress, randomPort)
}

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
		make(clientMap),
		make(idleMap),
		make(chan connLookup),
		false,
		prefs,
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

	// Close connection upon exit
	defer proxy.server.Close()
	defer proxy.pingServer.Close()

	// Start goroutine for cleaning up idle connections
	go proxy.idleConnectionCleanup()

	// Start goroutine for concurrent client connection map access
	go proxy.serverConnectionLookupLoop()

	// Start proxying ping packets from the broadcast listener
	go proxy.readLoop(proxy.pingServer)

	// Start processing everything else using the proxy listener
	proxy.readLoop(proxy.server)

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

func (proxy *ProxyServer) readLoop(listener net.PacketConn) {
	packetBuffer := make([]byte, maxMTU)

	for !proxy.dead {
		err := proxy.processDataFromClients(listener, packetBuffer)
		if err != nil {
			logger.Printf("Error while processing client data: %s\n", err)
		}
	}

	logger.Println("Proxy server shut down")
}

func (proxy *ProxyServer) processDataFromClients(listener net.PacketConn, packetBuffer []byte) error {
	read, client, _ := listener.ReadFrom(packetBuffer)
	if read <= 0 {
		return nil
	}

	data := packetBuffer[:read]

	serverConn, err := proxy.getServerConnection(client)
	if err != nil {
		return err
	}

	// Write packet from client to server
	_, err = serverConn.Write(data)
	return err
}

func (proxy *ProxyServer) serverConnectionLookupLoop() {
	for !proxy.dead {
		lookup := <-proxy.lookupChan
		conn, err := getServerConnection(proxy, lookup.client)
		lookup.responseChan <- connResponse{conn, err}
	}
}

func (proxy *ProxyServer) getServerConnection(client net.Addr) (*net.UDPConn, error) {
	lookup := connLookup{
		client,
		make(chan connResponse),
	}

	proxy.lookupChan <- lookup

	response := <-lookup.responseChan

	return response.conn, response.err
}

// getServerConnection gets or creates a new UDP connection to the remote server
// and stores it in a map, matching clients to remote server connections.
// This way, we keep one UDP connection open to the server for each Minecraft
// client that's connected to the proxy.
func getServerConnection(proxy *ProxyServer, client net.Addr) (*net.UDPConn, error) {
	key := client.String()

	// Store time for cleanup later
	proxy.idleMap[key] = time.Now()

	// Connection exists
	if conn, ok := proxy.clients[key]; ok {
		return conn, nil
	}

	// New connection needed
	logger.Printf("Opening connection to %s for new client %s!\n", proxy.remoteServerAddress, client)
	conn, err := proxy.newServerConnection()
	if err != nil {
		return nil, err
	}

	proxy.clients[key] = conn

	// Launch goroutine to pass packets from server to client
	go proxy.processDataFromServer(conn, client)

	return conn, nil
}

func (proxy *ProxyServer) newServerConnection() (*net.UDPConn, error) {
	logger.Printf("Opening connection to %s\n", proxy.remoteServerAddress)
	conn, err := net.DialUDP("udp", nil, proxy.remoteServerAddress)
	if err != nil {
		return nil, err
	}

	return conn, nil
}

// processDataFromServer proxies packets sent by the server to us for a specific
// Minecraft client back to that client's UDP connection.
func (proxy *ProxyServer) processDataFromServer(remoteConn *net.UDPConn, client net.Addr) {
	buffer := make([]byte, maxMTU)

	for !proxy.dead {
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
				truncServerName := strings.Split(packet.ServerName, ";")[:9]
				packet.ServerName = fmt.Sprintf("%v;", strings.Join(truncServerName, ";"))
				packetBuffer := packet.Build()
				data = packetBuffer.Bytes()
			} else {
				fmt.Printf("Failed to rewrite pong: %v\n", err)
			}
		}

		proxy.server.WriteTo(data, client)
	}
}

func (proxy *ProxyServer) idleConnectionCleanup() {
	logger.Println("Starting idle connection handler")

	for !proxy.dead {
		currentTime := time.Now()

		for key, lastActive := range proxy.idleMap {
			if lastActive.Add(proxy.prefs.IdleTimeout).Before(currentTime) {
				logger.Printf("Cleaning up idle connection: %s", key)
				proxy.clients[key].Close()
				delete(proxy.clients, key)
				delete(proxy.idleMap, key)
			}
		}

		time.Sleep(idleCheckInterval)
	}
}
