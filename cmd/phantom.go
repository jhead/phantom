package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"time"

	"github.com/jhead/phantom/internal/proxy"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

var bindAddressString string
var serverAddressString string
var bindPortInt uint16

func main() {
	// Required
	serverArg := flag.String("server", "", "Required: Bedrock/MCPE server IP address and port (ex: 1.2.3.4:19132)")

	// Optional
	bindArg := flag.String("bind", "0.0.0.0", "Optional: IP address to listen on. Defaults to all interfaces.")
	bindPortArg := flag.Int("bind_port", 0, "Optional: Port to listen on. Defaults to 0, which selects a random port.\nNote that phantom always binds to port 19132 as well, so both ports need to be open.")
	timeoutArg := flag.Int("timeout", 60, "Optional: Seconds to wait before cleaning up a disconnected client")
	debugArg := flag.Bool("debug", false, "Optional: Enables debug logging")
	ipv6Arg := flag.Bool("6", false, "Optional: Enables IPv6 support on port 19133 (experimental)")
	removePortsArg := flag.Bool("remove_ports", false, "Optional: Forces ports to be excluded from pong packets (experimental)")
	workersArg := flag.Uint("workers", 1, "Optional: Number of workers, useful for tweaking performance (experimental)")

	flag.Usage = usage
	flag.Parse()

	if *serverArg == "" {
		// Maybe it only has the server IP?
		if len(os.Args) == 2 {
			*serverArg = os.Args[1]
		} else {
			fmt.Println("Did you forget -server?")
			flag.Usage()
			return
		}
	}

	bindAddressString = *bindArg
	serverAddressString = *serverArg
	idleTimeout := time.Duration(*timeoutArg) * time.Second
	bindPortInt = uint16(*bindPortArg)

	logLevel := zerolog.InfoLevel
	if *debugArg {
		logLevel = zerolog.DebugLevel
	}

	fmt.Printf("Starting up with remote server IP: %s\n", serverAddressString)

	// Configure logging output
	log.Logger = log.
		Output(zerolog.ConsoleWriter{Out: os.Stdout}).
		Level(logLevel)

	proxyServer, err := proxy.New(proxy.ProxyPrefs{
		BindAddress:  bindAddressString,
		BindPort:     bindPortInt,
		RemoteServer: serverAddressString,
		IdleTimeout:  idleTimeout,
		EnableIPv6:   *ipv6Arg,
		RemovePorts:  *removePortsArg,
		NumWorkers:   *workersArg,
	})

	if err != nil {
		fmt.Printf("Failed to init server: %s\n", err)
		return
	}

	// Watch for CTRL + C
	watchForInterrupt(proxyServer)

	if err := proxyServer.Start(); err != nil {
		fmt.Printf("Failed to start server: %s\n", err)
	}
}

func usage() {
	fmt.Fprintf(flag.CommandLine.Output(), "Usage: %s [options] -server <server-ip>\n\nOptions:\n", os.Args[0])
	flag.PrintDefaults()
}

// Watches for CTRL + C signals and shuts down the server
// A second CTRL + C will force it to exit immediately
func watchForInterrupt(proxyServer *proxy.ProxyServer) {
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
			proxyServer.Close()
		}
	}()
}
