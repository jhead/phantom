package proxy

import (
	"fmt"
	"net"

	"github.com/jhead/phantom/internal/proto"
	"github.com/rs/zerolog/log"
	"github.com/tevino/abool"
)

// DiscoveryHub owns the exclusive :19132 (and optional :19133) LAN discovery
// sockets and fans each Unconnected Ping out to every registered proxy.
//
// Multiple OS processes cannot reliably share :19132: SO_REUSEADDR/PORT either
// load-balances or delivers unicast to a single socket, so only one phantom
// instance sees traffic (GitHub #175). One process + fan-out is the supported
// way to advertise multiple Bedrock servers on the same host.
type DiscoveryHub struct {
	ping4   net.PacketConn
	ping6   net.PacketConn
	proxies []*ProxyServer
	dead    *abool.AtomicBool
}

// StartDiscoveryHub binds discovery ports exclusively and starts read loops.
func StartDiscoveryHub(proxies []*ProxyServer, enableIPv6 bool) (*DiscoveryHub, error) {
	if len(proxies) == 0 {
		return nil, fmt.Errorf("no proxies to register for discovery")
	}

	hub := &DiscoveryHub{
		proxies: proxies,
		dead:    abool.New(),
	}

	log.Info().Msgf("Binding shared ping server to port 19132")
	ping4, err := net.ListenPacket("udp4", ":19132")
	if err != nil {
		return nil, fmt.Errorf("bind :19132: %w (only one phantom can own LAN discovery; pass multiple -server flags to one process instead of running multiple instances)", err)
	}
	hub.ping4 = ping4
	go hub.readLoop(ping4)

	if enableIPv6 {
		log.Info().Msgf("Binding shared IPv6 ping server to port 19133")
		ping6, err := net.ListenPacket("udp6", ":19133")
		if err != nil {
			log.Warn().Msgf("Failed to bind IPv6 ping listener: %v", err)
		} else {
			hub.ping6 = ping6
			go hub.readLoop(ping6)
		}
	}

	return hub, nil
}

// Close stops discovery listeners.
func (hub *DiscoveryHub) Close() {
	hub.dead.Set()
	if hub.ping4 != nil {
		_ = hub.ping4.Close()
	}
	if hub.ping6 != nil {
		_ = hub.ping6.Close()
	}
}

func (hub *DiscoveryHub) readLoop(listener net.PacketConn) {
	log.Info().Msgf("Discovery listener starting up: %s", listener.LocalAddr())
	buf := make([]byte, maxMTU)

	for !hub.dead.IsSet() {
		n, from, err := listener.ReadFrom(buf)
		if err != nil {
			if hub.dead.IsSet() {
				break
			}
			log.Warn().Msgf("Discovery read error: %v", err)
			continue
		}
		if n < 1 || buf[0] != proto.UnconnectedPingID {
			continue
		}

		// Copy once for fan-out; each proxy may hold the slice until Write returns.
		packet := append([]byte(nil), buf[:n]...)
		log.Info().Msgf("Received LAN ping from client: %s (fanning out to %d servers)", from.String(), len(hub.proxies))

		for _, p := range hub.proxies {
			proxy := p
			data := append([]byte(nil), packet...)
			go func() {
				if err := proxy.HandleUnconnectedPing(data, from); err != nil {
					log.Warn().Msgf("Discovery fan-out to %s failed: %v", proxy.prefs.RemoteServer, err)
				}
			}()
		}
	}

	log.Info().Msgf("Discovery listener shut down: %s", listener.LocalAddr())
}
