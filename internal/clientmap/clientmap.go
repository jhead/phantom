package clientmap

import (
	"net"
	"sync"
	"time"

	"github.com/jhead/phantom/internal/logging"
)

type ClientMap struct {
	IdleTimeout       time.Duration
	IdleCheckInterval time.Duration
	clients           map[string]clientEntry
	dead              bool
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
		false,
		&sync.RWMutex{},
	}

	// Start goroutine for cleaning up idle connections
	go clientMap.idleCleanupLoop()

	return &clientMap
}

func (cm *ClientMap) Close() {
	cm.dead = true

	cm.mutex.RLock()
	for _, client := range cm.clients {
		client.conn.Close()
	}
	cm.mutex.RUnlock()
}

func (cm *ClientMap) idleCleanupLoop() {
	logger.Println("Starting idle connection handler")

	// Loop forever using a channel that emits every IdleCheckInterval
	for currentTime := range time.Tick(cm.IdleCheckInterval) {
		// Stop the idle cleanup goroutine if the proxy stopped
		if cm.dead {
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

// Get gets or creates a new UDP connection to the remote server
// and stores it in a map, matching clients to remote server connections.
// This way, we keep one UDP connection open to the server for each Minecraft
// client that's connected to the proxy.
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

func newServerConnection(remote *net.UDPAddr) (*net.UDPConn, error) {
	logger.Printf("Opening connection to %s\n", remote)

	conn, err := net.DialUDP("udp", nil, remote)
	if err != nil {
		return nil, err
	}

	return conn, nil
}
