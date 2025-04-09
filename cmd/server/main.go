package main

import (
    "flag"
    "log"
    "os"
    "os/signal"
    "syscall"

    "github.com/euphoricair7/tun/internal/server"
)

func main() {
    // Command-line flags
    registrationPort := flag.Int("port", 5678, "Port for client registrations")
    minPort := flag.Int("min-port", 10000, "Minimum port in the range of assignable ports")
    maxPort := flag.Int("max-port", 10050, "Maximum port in the range of assignable ports")
    flag.Parse()

    // Create and start the relay server
    s, err := server.NewRelayServer(*registrationPort, *minPort, *maxPort)
    if err != nil {
        log.Fatalf("Failed to create relay server: %v", err)
    }

    // Start server in a goroutine
    go func() {
        log.Printf("Starting relay server on port %d...", *registrationPort)
        if err := s.Start(); err != nil {
            log.Fatalf("Server failed: %v", err)
        }
    }()

    // Handle graceful shutdown
    sig := make(chan os.Signal, 1)
    signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
    <-sig

    log.Println("Shutting down relay server...")
    s.Shutdown()
    log.Println("Server shutdown complete")
}