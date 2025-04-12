# tun - A Tunneling Application in Go

## Overview
**tun** is a lightweight tunneling application implemented in Go. It facilitates secure and efficient communication between a client and a relay server.

## Directory Structure
tun/

├── cmd/

│   ├── server/
│   │   └── main.go       # Entry point for the relay server

│   └── client/
│       └── main.go       # Entry point for the client

├── internal/
│   ├── server/
│   │   └── server.go     # Relay server implementation

│   └── client/
│       └── client.go     # Client implementation

├── pkg/
│   └── protocol/
│       └── protocol.go   # Shared protocol definitions

└── go.mod                # Go module definition


