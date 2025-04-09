# tun
A tunneling application written Go Lang

# Directory Structure

tun/
├── cmd/
│   ├── server/
│   │   └── main.go       # Entry point for relay server
│   └── client/
│       └── main.go       # Entry point for client
├── internal/
│   ├── server/
│   │   └── server.go     # Relay server implementation
│   └── client/
│       └── client.go     # Client implementation
├── pkg/
│   └── protocol/
│       └── protocol.go   # Shared protocol definitions
└── go.mod               # Go module definition
