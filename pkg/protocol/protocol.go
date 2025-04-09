package protocol

// Message types for client-server communication
const (
    MessageTypeConnect    = "connect"
    MessageTypeData       = "data"
    MessageTypeDisconnect = "disconnect"
    MessageTypePing       = "ping"
    MessageTypePong       = "pong"
)

// RegistrationRequest represents the initial request from client to relay
type RegistrationRequest struct {
    LocalHost string `json:"local_host"`
    LocalPort int    `json:"local_port"`
}

// RegistrationResponse represents the relay's response to a registration
type RegistrationResponse struct {
    Success    bool   `json:"success"`
    PublicPort int    `json:"public_port,omitempty"`
    Error      string `json:"error,omitempty"`
}

// ClientMessage represents messages exchanged between client and relay
type ClientMessage struct {
    Type   string `json:"type"`
    UserID string `json:"user_id,omitempty"`
    Data   []byte `json:"data,omitempty"`
}





//gcloud compute ssh relay-sever

