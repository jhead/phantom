package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/jhead/phantom/internal/proxy"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// serverList accumulates repeated and/or comma-separated -server values.
type serverList []string

func (s *serverList) String() string {
	return strings.Join(*s, ",")
}

func (s *serverList) Set(value string) error {
	for _, part := range strings.Split(value, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		*s = append(*s, part)
	}
	return nil
}

func main() {
	var servers serverList

	// Required (repeatable / comma-separated)
	flag.Var(&servers, "server", "Required: Bedrock/MCPE server IP:port. Repeat or comma-separate for multiple servers on one process (ex: 1.2.3.4:19132)")

	// Optional
	bindArg := flag.String("bind", "0.0.0.0", "Optional: IP address to listen on. Defaults to all interfaces.")
	bindPortArg := flag.Int("bind_port", 0, "Optional: Port to listen on. Defaults to 0, which selects a random port.\nNote that phantom always binds to port 19132 as well, so both ports need to be open.\nOnly valid with a single -server.")
	timeoutArg := flag.Int("timeout", 60, "Optional: Seconds to wait before cleaning up a disconnected client")
	debugArg := flag.Bool("debug", false, "Optional: Enables debug logging")
	removePortsArg := flag.Bool("remove_ports", false, "Optional: Forces ports to be excluded from pong packets (experimental)")
	workersArg := flag.Uint("workers", 1, "Optional: Number of workers, useful for tweaking performance (experimental)")
	disableDiscoveryArg := flag.Bool("disable_discovery", false, "Optional: Do not bind LAN discovery ports 19132/19133; data-plane pings on -bind_port still work")

	// Prefer -ipv6: PowerShell treats bare -6 as a number, so the flag never reaches
	// the process (see #124). Keep -6 as a legacy alias for cmd.exe / POSIX shells.
	var enableIPv6 bool
	flag.BoolVar(&enableIPv6, "ipv6", false, "Optional: Enables IPv6 support on port 19133 (experimental)")
	flag.BoolVar(&enableIPv6, "6", false, "Optional: Same as -ipv6 (legacy; broken in PowerShell — use -ipv6)")

	flag.Usage = usage
	flag.Parse()

	if len(servers) == 0 {
		// Maybe it only has the server IP?
		if len(os.Args) == 2 {
			_ = servers.Set(os.Args[1])
		} else {
			fmt.Println("Did you forget -server?")
			flag.Usage()
			return
		}
	}

	if len(servers) == 0 {
		fmt.Println("Did you forget -server?")
		flag.Usage()
		return
	}

	if *bindPortArg != 0 && len(servers) > 1 {
		fmt.Println("-bind_port cannot be used with multiple -server values; omit it so each server gets a random data port")
		return
	}

	idleTimeout := time.Duration(*timeoutArg) * time.Second
	bindPortInt := uint16(*bindPortArg)

	logLevel := zerolog.InfoLevel
	if *debugArg {
		logLevel = zerolog.DebugLevel
	}

	fmt.Printf("Starting up with remote server(s): %s\n", strings.Join(servers, ", "))

	// Configure logging output
	log.Logger = log.
		Output(zerolog.ConsoleWriter{Out: os.Stdout}).
		Level(logLevel)

	proxies := make([]*proxy.ProxyServer, 0, len(servers))
	for _, remote := range servers {
		prefs := proxy.ProxyPrefs{
			BindAddress:              *bindArg,
			BindPort:                 bindPortInt,
			RemoteServer:             remote,
			IdleTimeout:              idleTimeout,
			EnableIPv6:               enableIPv6,
			RemovePorts:              *removePortsArg,
			NumWorkers:               *workersArg,
			DisableDiscoveryListener: *disableDiscoveryArg || len(servers) > 1,
		}

		proxyServer, err := proxy.New(prefs)
		if err != nil {
			fmt.Printf("Failed to init server %s: %s\n", remote, err)
			closeAll(proxies, nil)
			return
		}
		proxies = append(proxies, proxyServer)
	}

	// Single server: original blocking Start() (owns :19132 itself).
	if len(proxies) == 1 {
		watchForInterrupt(proxies, nil)
		if err := proxies[0].Start(); err != nil {
			fmt.Printf("Failed to start server: %s\n", err)
		}
		return
	}

	// Multiple servers: bind data planes first, then one shared discovery hub.
	for _, p := range proxies {
		if err := p.StartAsync(); err != nil {
			fmt.Printf("Failed to start server %s: %s\n", p.RemoteServer(), err)
			closeAll(proxies, nil)
			return
		}
	}

	hub, err := proxy.StartDiscoveryHub(proxies, enableIPv6)
	if err != nil {
		fmt.Printf("Failed to start discovery: %s\n", err)
		closeAll(proxies, nil)
		return
	}

	watchForInterrupt(proxies, hub)
	select {}
}

func closeAll(proxies []*proxy.ProxyServer, hub *proxy.DiscoveryHub) {
	if hub != nil {
		hub.Close()
	}
	for _, p := range proxies {
		p.Close()
	}
}

func usage() {
	fmt.Fprintf(flag.CommandLine.Output(), "Usage: %s [options] -server <server-ip> [-server <server-ip> ...]\n\nOptions:\n", os.Args[0])
	flag.PrintDefaults()
}

// Watches for CTRL + C signals and shuts down the server
// A second CTRL + C will force it to exit immediately
func watchForInterrupt(proxies []*proxy.ProxyServer, hub *proxy.DiscoveryHub) {
	signalChan := make(chan os.Signal, 1)

	signal.Notify(signalChan, os.Interrupt)

	go func() {
		once := false

		for range signalChan {
			if once {
				fmt.Println("\nForce quitting")
				os.Exit(2)
			}

			fmt.Println("\nPress CTRL + C again to force quit")

			once = true
			closeAll(proxies, hub)
		}
	}()
}
