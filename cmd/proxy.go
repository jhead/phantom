package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/jhead/phantom/internal/proxy"
)

var bindAddressString string
var serverAddressString string
var versionString string
var nameString string
var usersInt int

func main() {
	bindArg := flag.String("bind", "0.0.0.0:19132", "IP address and port to listen on")
	serverArg := flag.String("server", "", "Bedrock/MCPE server IP address and port (ex: 1.2.3.4:19132)")

	flag.Usage = usage
	flag.Parse()

	if *serverArg == "" {
		flag.Usage()
		return
	}

	bindAddressString = *bindArg
	serverAddressString = *serverArg

	fmt.Printf("Starting up with remote server IP: %s\n", serverAddressString)

	proxyServer, err := proxy.New(bindAddressString, serverAddressString)
	if err != nil {
		fmt.Printf("Failed to init server: %s\n", err)
		return
	}

	if err := proxyServer.Start(); err != nil {
		fmt.Printf("Failed to start server: %s\n", err)
	}
}

func usage() {
	fmt.Fprintf(flag.CommandLine.Output(), "Usage: %s [options] -server <server-ip>\n\nOptions:\n", os.Args[0])
	flag.PrintDefaults()
}
