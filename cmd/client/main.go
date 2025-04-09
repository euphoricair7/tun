package main

import (
    "flag"
    "log"
    "os"
    "os/signal"
    "syscall"

    "github.com/euphoricair7/tun/internal/client"
)

func main() {
    // Command-line flags
    relayHost := flag.String("relay", "localhost", "Relay server hostname or IP")
    relayPort := flag.Int("relay-port", 5678, "Relay server registration port")
    localHost := flag.String("local-host", "localhost", "Local service hostname")
    localPort := flag.Int("local-port", 3000, "Local service port")
    flag.Parse()

    // Create tunnel client
    tunnelClient, err := client.NewTunnelClient(*relayHost, *relayPort, *localHost, *localPort)
    if err != nil {
        log.Fatalf("Failed to create tunnel client: %v", err)
    }

    // Connect to relay in a goroutine
    go func() {
        log.Printf("Connecting to relay server %s:%d...", *relayHost, *relayPort)
        if err := tunnelClient.Start(); err != nil {
            log.Fatalf("Tunnel client failed: %v", err)
        }
    }()

    // Handle graceful shutdown
    sig := make(chan os.Signal, 1)
    signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
    <-sig

    log.Println("Shutting down tunnel client...")
    tunnelClient.Shutdown()
    log.Println("Client shutdown complete")
}