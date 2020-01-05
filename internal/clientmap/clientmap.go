package clientmap

import (
	"net"
	"sync"
	"time"

	"github.com/jhead/phantom/internal/logging"
	"github.com/tevino/abool"
)

// ClientMap provides a goroutine-safe map of UDP connections
// to a remote address keyed by the client address, with a built-in
// idle TTL that closes and removes entries that remain idle beyond it.
type ClientMap struct {
	IdleTimeout       time.Duration
	IdleCheckInterval time.Duration
	clients           map[string]clientEntry
	dead              *abool.AtomicBool
	mutex             *sync.RWMutex
}

type clientEntry struct {
	conn       *net.UDPConn
	lastActive time.Time
}

type ServerConnHandler func(*net.UDPConn)

var logger = logging.Get()

func New(idleTimeout time.Duration, idleCheckInterval time.Duration) *ClientMap {
	clientMap := ClientMap{
		idleTimeout,
		idleCheckInterval,
		make(map[string]clientEntry),
		abool.New(),
		&sync.RWMutex{},
	}

	// Start goroutine for cleaning up idle connections
	go clientMap.idleCleanupLoop()

	return &clientMap
}

// Close cleans up all clients
func (cm *ClientMap) Close() {
	if cm.dead.IsSet() {
		return
	}

	// Stop loop in goroutine
	cm.dead.Set()

	cm.mutex.RLock()
	for _, client := range cm.clients {
		client.conn.Close()
	}
	cm.mutex.RUnlock()
}

// Cleans up clients and remote connections that have not been used in a while.
// Blocks until the ClientMap has been closed.
func (cm *ClientMap) idleCleanupLoop() {
	logger.Println("Starting idle connection handler")

	// Loop forever using a channel that emits every IdleCheckInterval
	for currentTime := range time.Tick(cm.IdleCheckInterval) {
		// Stop the idle cleanup goroutine if the proxy stopped
		if cm.dead.IsSet() {
			break
		}

		cm.mutex.Lock()
		for key, client := range cm.clients {
			if client.lastActive.Add(cm.IdleTimeout).Before(currentTime) {
				logger.Printf("Cleaning up idle connection: %s", key)
				cm.clients[key].conn.Close()
				delete(cm.clients, key)
			}
		}
		cm.mutex.Unlock()
	}
}

// Get gets or creates a new UDP connection to the remote server and stores it
// in a map, matching clients to remote server connections. This way, we keep one
// UDP connection open to the server for each client. The handler parameter is
// invoked when a new connection needs to be created (for a new client) to defer
// that behavior to the caller.
func (cm *ClientMap) Get(
	clientAddr net.Addr,
	remote *net.UDPAddr,
	handler ServerConnHandler,
) (*net.UDPConn, error) {
	key := clientAddr.String()

	// Check if connection exists
	cm.mutex.RLock()

	if client, ok := cm.clients[key]; ok {
		cm.mutex.RUnlock()
		client.lastActive = time.Now()
		return client.conn, nil
	}

	cm.mutex.RUnlock()
	cm.mutex.Lock()
	defer cm.mutex.Unlock()

	// New connection needed
	logger.Printf("Opening connection to %s for new client %s!\n", remote, clientAddr)
	newServerConn, err := newServerConnection(remote)
	if err != nil {
		return nil, err
	}

	cm.clients[key] = clientEntry{
		newServerConn,
		time.Now(),
	}

	// Launch goroutine to pass packets from server to client
	go handler(newServerConn)

	return newServerConn, nil
}

// Creates a UDP connection to the remote address
func newServerConnection(remote *net.UDPAddr) (*net.UDPConn, error) {
	logger.Printf("Opening connection to %s\n", remote)

	conn, err := net.DialUDP("udp", nil, remote)
	if err != nil {
		return nil, err
	}

	return conn, nil
}
