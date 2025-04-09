package server

import (
    "encoding/json"
    "fmt"
    "io"
    "log"
    "net"
    "sync"
    "time"

    "github.com/euphoricair7/tun/pkg/protocol"
)

// RelayServer handles client registrations and forwards traffic
type RelayServer struct {
    registrationPort int
    availablePorts   []int
    portsMutex       sync.Mutex
    clients          map[int]*clientConnection
    clientsMutex     sync.RWMutex
    listener         net.Listener
    shutdown         chan struct{}
}

type clientConnection struct {
    conn          net.Conn
    targetHost    string
    targetPort    int
    userConns     map[string]net.Conn // key is user connection ID
    userConnMutex sync.RWMutex
}

// NewRelayServer creates a new relay server instance
func NewRelayServer(registrationPort, minPort, maxPort int) (*RelayServer, error) {
    // Generate available ports list
    availablePorts := make([]int, 0, maxPort-minPort+1)
    for port := minPort; port <= maxPort; port++ {
        availablePorts = append(availablePorts, port)
    }

    return &RelayServer{
        registrationPort: registrationPort,
        availablePorts:   availablePorts,
        clients:          make(map[int]*clientConnection),
        shutdown:         make(chan struct{}),
    }, nil
}

// Start begins accepting client registration requests
func (s *RelayServer) Start() error {
    var err error
    s.listener, err = net.Listen("tcp", fmt.Sprintf(":%d", s.registrationPort))
    if err != nil {
        return fmt.Errorf("failed to start registration listener: %w", err)
    }
    defer s.listener.Close()

    log.Printf("Registration server listening on port %d", s.registrationPort)

    // Accept connections in a loop
    for {
        conn, err := s.listener.Accept()
        if err != nil {
            select {
            case <-s.shutdown:
                return nil // Server is shutting down
            default:
                log.Printf("Error accepting connection: %v", err)
                continue
            }
        }

        // Handle each client in a separate goroutine
        go s.handleClientRegistration(conn)
    }
}

// Shutdown gracefully stops the server
func (s *RelayServer) Shutdown() {
    close(s.shutdown)
    if s.listener != nil {
        s.listener.Close()
    }

    // Close all client connections
    s.clientsMutex.Lock()
    defer s.clientsMutex.Unlock()

    for port, client := range s.clients {
        log.Printf("Closing client connection on port %d", port)
        client.conn.Close()

        // Close all user connections for this client
        client.userConnMutex.Lock()
        for id, conn := range client.userConns {
            log.Printf("Closing user connection %s", id)
            conn.Close()
        }
        client.userConnMutex.Unlock()
    }
}

// handleClientRegistration processes a new client connection
func (s *RelayServer) handleClientRegistration(conn net.Conn) {
    defer func() {
        if err := recover(); err != nil {
            log.Printf("Panic in handleClientRegistration: %v", err)
        }
    }()

    clientAddr := conn.RemoteAddr().String()
    log.Printf("New client connection from %s", clientAddr)

    // Read client registration request
    decoder := json.NewDecoder(conn)
    var req protocol.RegistrationRequest
    if err := decoder.Decode(&req); err != nil {
        log.Printf("Error decoding registration request from %s: %v", clientAddr, err)
        sendErrorResponse(conn, "Invalid request format")
        conn.Close()
        return
    }

    if req.LocalPort <= 0 {
        sendErrorResponse(conn, "Invalid local port specified")
        conn.Close()
        return
    }

    // Allocate a port
    port, err := s.allocatePort()
    if err != nil {
        log.Printf("Failed to allocate port for %s: %v", clientAddr, err)
        sendErrorResponse(conn, err.Error())
        conn.Close()
        return
    }

    // Create port listener
    listener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
    if err != nil {
        log.Printf("Failed to create listener for port %d: %v", port, err)
        s.releasePort(port)
        sendErrorResponse(conn, fmt.Sprintf("Failed to bind to port %d", port))
        conn.Close()
        return
    }

    // Initialize client connection
    client := &clientConnection{
        conn:       conn,
        targetHost: req.LocalHost,
        targetPort: req.LocalPort,
        userConns:  make(map[string]net.Conn),
    }

    // Save the client connection
    s.clientsMutex.Lock()
    s.clients[port] = client
    s.clientsMutex.Unlock()

    // Send success response with assigned port
    resp := protocol.RegistrationResponse{
        Success:    true,
        PublicPort: port,
    }
    encoder := json.NewEncoder(conn)
    if err := encoder.Encode(resp); err != nil {
        log.Printf("Error sending response to client %s: %v", clientAddr, err)
        s.cleanupClient(port, listener)
        return
    }

    log.Printf("Assigned port %d to client %s for service %s:%d", 
        port, clientAddr, req.LocalHost, req.LocalPort)

    // Start a goroutine to handle client protocol messages
    go s.handleClientCommunication(port, client)

    // Start accepting user connections on the assigned port
    go s.acceptUserConnections(port, listener, client)
}

// handleClientCommunication processes messages from the client
func (s *RelayServer) handleClientCommunication(port int, client *clientConnection) {
    decoder := json.NewDecoder(client.conn)

    for {
        var msg protocol.ClientMessage
        if err := decoder.Decode(&msg); err != nil {
            if err != io.EOF {
                log.Printf("Error decoding message from client on port %d: %v", port, err)
            }
            s.cleanupClient(port, nil)
            return
        }

        // Process message based on type
        switch msg.Type {
        case protocol.MessageTypeData:
            // Handle data from client to a specific user
            if msg.UserID == "" || msg.Data == nil {
                continue
            }

            client.userConnMutex.RLock()
            userConn, exists := client.userConns[msg.UserID]
            client.userConnMutex.RUnlock()

            if exists {
                if _, err := userConn.Write(msg.Data); err != nil {
                    log.Printf("Error writing to user %s: %v", msg.UserID, err)
                    client.userConnMutex.Lock()
                    userConn.Close()
                    delete(client.userConns, msg.UserID)
                    client.userConnMutex.Unlock()
                }
            }

        case protocol.MessageTypePing:
            // Handle ping to keep connection alive
            encoder := json.NewEncoder(client.conn)
            pong := protocol.ClientMessage{Type: protocol.MessageTypePong}
            if err := encoder.Encode(pong); err != nil {
                log.Printf("Error sending pong to client on port %d: %v", port, err)
            }

        case protocol.MessageTypeDisconnect:
            // Client wants to disconnect
            log.Printf("Client on port %d requested disconnect", port)
            s.cleanupClient(port, nil)
            return
        }
    }
}

// acceptUserConnections handles incoming connections on the client's assigned public port
func (s *RelayServer) acceptUserConnections(port int, listener net.Listener, client *clientConnection) {
    defer listener.Close()

    for {
        userConn, err := listener.Accept()
        if err != nil {
            select {
            case <-s.shutdown:
                return // Server is shutting down
            default:
                log.Printf("Error accepting user connection on port %d: %v", port, err)
                continue
            }
        }

        userAddr := userConn.RemoteAddr().String()
        log.Printf("New user connection from %s to port %d", userAddr, port)

        // Generate a unique ID for this user connection
        userID := fmt.Sprintf("%s-%d", userAddr, time.Now().UnixNano())

        // Save user connection
        client.userConnMutex.Lock()
        client.userConns[userID] = userConn
        client.userConnMutex.Unlock()

        // Notify client about new connection
        connectMsg := protocol.ClientMessage{
            Type:   protocol.MessageTypeConnect,
            UserID: userID,
        }
        encoder := json.NewEncoder(client.conn)
        if err := encoder.Encode(connectMsg); err != nil {
            log.Printf("Error notifying client of new connection: %v", err)
            userConn.Close()
            continue
        }

        // Start a goroutine to handle user data
        go s.handleUserData(port, client, userID, userConn)
    }
}

// handleUserData forwards data from the user connection to the client
func (s *RelayServer) handleUserData(port int, client *clientConnection, userID string, userConn net.Conn) {
    defer func() {
        userConn.Close()
        client.userConnMutex.Lock()
        delete(client.userConns, userID)
        client.userConnMutex.Unlock()

        // Notify client about disconnection
        disconnectMsg := protocol.ClientMessage{
            Type:   protocol.MessageTypeDisconnect,
            UserID: userID,
        }
        encoder := json.NewEncoder(client.conn)
        encoder.Encode(disconnectMsg)
    }()

    buffer := make([]byte, 4096)
    for {
        // Read data from user
        n, err := userConn.Read(buffer)
        if err != nil {
            if err != io.EOF {
                log.Printf("Error reading from user %s: %v", userID, err)
            }
            return
        }

        // Forward data to client
        dataMsg := protocol.ClientMessage{
            Type:   protocol.MessageTypeData,
            UserID: userID,
            Data:   buffer[:n],
        }
        encoder := json.NewEncoder(client.conn)
        if err := encoder.Encode(dataMsg); err != nil {
            log.Printf("Error forwarding user data to client: %v", err)
            return
        }
    }
}

// allocatePort finds and reserves an available port
func (s *RelayServer) allocatePort() (int, error) {
    s.portsMutex.Lock()
    defer s.portsMutex.Unlock()

    if len(s.availablePorts) == 0 {
        return 0, fmt.Errorf("no available ports")
    }

    // Take the first available port
    port := s.availablePorts[0]
    s.availablePorts = s.availablePorts[1:]
    return port, nil
}

// releasePort returns a port to the available pool
func (s *RelayServer) releasePort(port int) {
    s.portsMutex.Lock()
    defer s.portsMutex.Unlock()

    s.availablePorts = append(s.availablePorts, port)
}

// cleanupClient releases all resources associated with a client
func (s *RelayServer) cleanupClient(port int, listener net.Listener) {
    // Close the listener if provided
    if listener != nil {
        listener.Close()
    }

    // Lock for client map modifications
    s.clientsMutex.Lock()
    defer s.clientsMutex.Unlock()

    client, exists := s.clients[port]
    if !exists {
        return
    }

    // Close client connection
    client.conn.Close()

    // Close all user connections
    client.userConnMutex.Lock()
    for _, conn := range client.userConns {
        conn.Close()
    }
    client.userConnMutex.Unlock()

    // Remove client from map and release port
    delete(s.clients, port)
    s.releasePort(port)

    log.Printf("Cleaned up client on port %d", port)
}

// sendErrorResponse sends an error response to the client
func sendErrorResponse(conn net.Conn, message string) {
    resp := protocol.RegistrationResponse{
        Success: false,
        Error:   message,
    }
    encoder := json.NewEncoder(conn)
    encoder.Encode(resp)
}