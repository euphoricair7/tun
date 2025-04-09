package client

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

// TunnelClient connects to a relay server and forwards traffic to a local service
type TunnelClient struct {
    relayHost     string
    relayPort     int
    localHost     string
    localPort     int
    conn          net.Conn
    publicPort    int
    userConns     map[string]*userConnection
    userConnMutex sync.RWMutex
    shutdown      chan struct{}
    wg            sync.WaitGroup
    pingTicker    *time.Ticker
}

type userConnection struct {
    localConn net.Conn
}

// NewTunnelClient creates a new tunnel client
func NewTunnelClient(relayHost string, relayPort int, localHost string, localPort int) (*TunnelClient, error) {
    return &TunnelClient{
        relayHost:  relayHost,
        relayPort:  relayPort,
        localHost:  localHost,
        localPort:  localPort,
        userConns:  make(map[string]*userConnection),
        shutdown:   make(chan struct{}),
        pingTicker: time.NewTicker(30 * time.Second),
    }, nil
}

// Start initiates connection to the relay server
func (c *TunnelClient) Start() error {
    var err error
    c.conn, err = net.Dial("tcp", fmt.Sprintf("%s:%d", c.relayHost, c.relayPort))
    if err != nil {
        return fmt.Errorf("failed to connect to relay server: %w", err)
    }

    // Send registration request
    req := protocol.RegistrationRequest{
        LocalHost: c.localHost,
        LocalPort: c.localPort,
    }

    encoder := json.NewEncoder(c.conn)
    if err := encoder.Encode(req); err != nil {
        c.conn.Close()
        return fmt.Errorf("failed to send registration request: %w", err)
    }

    // Wait for response
    decoder := json.NewDecoder(c.conn)
    var resp protocol.RegistrationResponse
    if err := decoder.Decode(&resp); err != nil {
        c.conn.Close()
        return fmt.Errorf("failed to read registration response: %w", err)
    }

    if !resp.Success {
        c.conn.Close()
        return fmt.Errorf("registration failed: %s", resp.Error)
    }

    c.publicPort = resp.PublicPort
    log.Printf("Successfully registered! Your service is now available at: %s:%d",
        c.relayHost, c.publicPort)

    // Start processing messages from relay
    c.wg.Add(1)
    go c.handleRelayMessages()

    // Start ping routine to keep connection alive
    c.wg.Add(1)
    go c.keepAlive()

    return nil
}

// Shutdown closes the client connection
func (c *TunnelClient) Shutdown() {
    close(c.shutdown)
    c.pingTicker.Stop()

    // Notify the server we're disconnecting
    if c.conn != nil {
        disconnectMsg := protocol.ClientMessage{
            Type: protocol.MessageTypeDisconnect,
        }
        encoder := json.NewEncoder(c.conn)
        encoder.Encode(disconnectMsg)
        c.conn.Close()
    }

    // Close all user connections
    c.userConnMutex.Lock()
    for _, uc := range c.userConns {
        if uc.localConn != nil {
            uc.localConn.Close()
        }
    }
    c.userConnMutex.Unlock()

    // Wait for goroutines to finish
    c.wg.Wait()
}

// handleRelayMessages processes messages received from the relay server
func (c *TunnelClient) handleRelayMessages() {
    defer c.wg.Done()
    decoder := json.NewDecoder(c.conn)

    for {
        select {
        case <-c.shutdown:
            return
        default:
            var msg protocol.ClientMessage
            if err := decoder.Decode(&msg); err != nil {
                if err != io.EOF {
                    log.Printf("Error decoding message from relay: %v", err)
                }
                log.Println("Connection to relay server closed")
                c.Shutdown()
                return
            }

            // Process message based on type
            switch msg.Type {
            case protocol.MessageTypeConnect:
                // New user connection
                if msg.UserID == "" {
                    continue
                }
                go c.handleUserConnection(msg.UserID)

            case protocol.MessageTypeData:
                // Data for an existing user connection
                if msg.UserID == "" || msg.Data == nil {
                    continue
                }
                c.forwardToLocalService(msg.UserID, msg.Data)

            case protocol.MessageTypeDisconnect:
                // User disconnected
                if msg.UserID == "" {
                    continue
                }
                c.closeUserConnection(msg.UserID)

            case protocol.MessageTypePong:
                // Server responded to our ping
                log.Println("Received pong from relay server")
            }
        }
    }
}

// handleUserConnection creates a connection to the local service for a new user
func (c *TunnelClient) handleUserConnection(userID string) {
    log.Printf("New user connection: %s", userID)

    // Connect to local service
    localConn, err := net.Dial("tcp", fmt.Sprintf("%s:%d", c.localHost, c.localPort))
    if err != nil {
        log.Printf("Failed to connect to local service for user %s: %v", userID, err)
        return
    }

    // Save the connection
    userConn := &userConnection{
        localConn: localConn,
    }
    c.userConnMutex.Lock()
    c.userConns[userID] = userConn
    c.userConnMutex.Unlock()

    // Read responses from local service and send to relay
    c.wg.Add(1)
    go func() {
        defer c.wg.Done()
        buffer := make([]byte, 4096)
        for {
            select {
            case <-c.shutdown:
                return
            default:
                n, err := localConn.Read(buffer)
                if err != nil {
                    if err != io.EOF {
                        log.Printf("Error reading from local service for user %s: %v", userID, err)
                    }
                    c.closeUserConnection(userID)
                    return
                }

                // Send data back to relay
                dataMsg := protocol.ClientMessage{
                    Type:   protocol.MessageTypeData,
                    UserID: userID,
                    Data:   buffer[:n],
                }
                encoder := json.NewEncoder(c.conn)
                if err := encoder.Encode(dataMsg); err != nil {
                    log.Printf("Error sending data to relay: %v", err)
                    c.closeUserConnection(userID)
                    return
                }
            }
        }
    }()
}

// forwardToLocalService sends data to the local service
func (c *TunnelClient) forwardToLocalService(userID string, data []byte) {
    c.userConnMutex.RLock()
    userConn, exists := c.userConns[userID]
    c.userConnMutex.RUnlock()

    if !exists {
        log.Printf("Cannot forward data: user connection %s not found", userID)
        return
    }

    if _, err := userConn.localConn.Write(data); err != nil {
        log.Printf("Error writing to local service for user %s: %v", userID, err)
        c.closeUserConnection(userID)
    }
}

// closeUserConnection closes and cleans up a user connection
func (c *TunnelClient) closeUserConnection(userID string) {
    c.userConnMutex.Lock()
    defer c.userConnMutex.Unlock()

    userConn, exists := c.userConns[userID]
    if !exists {
        return
    }

    if userConn.localConn != nil {
        userConn.localConn.Close()
    }
    delete(c.userConns, userID)
    log.Printf("Closed connection for user %s", userID)
}

// keepAlive sends periodic pings to keep the connection alive
func (c *TunnelClient) keepAlive() {
    defer c.wg.Done()
    for {
        select {
        case <-c.shutdown:
            return
        case <-c.pingTicker.C:
            pingMsg := protocol.ClientMessage{
                Type: protocol.MessageTypePing,
            }
            encoder := json.NewEncoder(c.conn)
            if err := encoder.Encode(pingMsg); err != nil {
                log.Printf("Error sending ping: %v", err)
            } else {
                log.Println("Ping sent to relay server")
            }
        }
    }
}