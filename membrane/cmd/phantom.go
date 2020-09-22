package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"time"

	"github.com/jhead/phantom/membrane/internal/proxy"
	"github.com/jhead/phantom/membrane/internal/services/api"
	"github.com/jhead/phantom/membrane/internal/services/db/jsondb"
	"github.com/jhead/phantom/membrane/internal/services/servers"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

var bindAddressString string
var serverAddressString string
var bindPortInt uint16

func main() {
	// Common
	debugArg := flag.Bool("debug", false, "Optional: Enables debug logging")
	apiFlag := flag.Bool("api", false, "Optional: Enables an HTTP server and JSON REST API for managing phantom")

	// Single-server mode
	serverArg := flag.String("server", "", "Required: Bedrock/MCPE server IP address and port (ex: 1.2.3.4:19132)")
	bindArg := flag.String("bind", "0.0.0.0", "Optional: IP address to listen on. Defaults to all interfaces.")
	bindPortArg := flag.Int("bind_port", 0, "Optional: Port to listen on. Defaults to 0, which selects a random port.\nNote that phantom always binds to port 19132 as well, so both ports need to be open.")
	timeoutArg := flag.Int("timeout", 60, "Optional: Seconds to wait before cleaning up a disconnected client")
	ipv6Arg := flag.Bool("6", false, "Optional: Enables IPv6 support on port 19133 (experimental)")
	removePortsArg := flag.Bool("remove_ports", false, "Optional: Forces ports to be excluded from pong packets (experimental)")

	flag.Usage = usage
	flag.Parse()

	logLevel := zerolog.InfoLevel
	if *debugArg {
		logLevel = zerolog.DebugLevel
	}

	// Configure logging output
	log.Logger = log.
		Output(zerolog.ConsoleWriter{Out: os.Stdout}).
		Level(logLevel)

	// SelectSingle-server mode (original) or API mode
	if !*apiFlag {
		singleServerMode(serverArg, bindArg, bindPortArg, timeoutArg, ipv6Arg, removePortsArg)
	} else {
		apiServerMode()
	}
}

func usage() {
	fmt.Fprintf(flag.CommandLine.Output(), "Usage: %s [options] -server <server-ip>\n\nOptions:\n", os.Args[0])
	flag.PrintDefaults()
}

// Watches for CTRL + C signals and shuts down the server
// A second CTRL + C will force it to exit immediately
func watchForInterrupt(finalizer func()) {
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
			finalizer()
		}
	}()
}

func singleServerMode(
	serverArg *string,
	bindArg *string,
	bindPortArg *int,
	timeoutArg *int,
	ipv6Arg *bool,
	removePortsArg *bool,
) {
	serverAddressString := *serverArg

	if serverAddressString == "" {
		// Maybe it only has the server IP?
		if len(os.Args) == 2 {
			serverAddressString = os.Args[1]
		} else {
			fmt.Println("Did you forget -server?")
			flag.Usage()
			return
		}
	}

	bindAddressString = *bindArg
	idleTimeout := time.Duration(*timeoutArg) * time.Second
	bindPortInt = uint16(*bindPortArg)

	fmt.Printf("Starting up with remote server IP: %s\n", serverAddressString)

	proxyServer, err := proxy.New(proxy.ProxyPrefs{
		bindAddressString,
		bindPortInt,
		serverAddressString,
		idleTimeout * time.Second,
		*ipv6Arg,
		*removePortsArg,
	})

	if err != nil {
		fmt.Printf("Failed to init server: %s\n", err)
		return
	}

	// Watch for CTRL + C
	watchForInterrupt(func() {
		proxyServer.Close()
	})

	if err := proxyServer.Start(); err != nil {
		fmt.Printf("Failed to start server: %s\n", err)
	}
}

func apiServerMode() {
	fmt.Println("Starting up in API server mode")

	database, err := jsondb.New("./phantom.json")
	if err != nil {
		log.Fatal().Err(err).Msgf("Failed to open database")
		return
	}

	settings, err := database.GetSettings()
	if err != nil {
		log.Fatal().Err(err).Msgf("Failed to load settings")
		return
	}

	apiService := api.New(settings, servers.New(database))

	watchForInterrupt(func() {
		log.Info().Msg("Shutting down API server")
		apiService.Close()
	})

	if err := apiService.Start(); err != nil {
		log.Fatal().Msgf("Failed to start API server: %v", err)
	}
}
