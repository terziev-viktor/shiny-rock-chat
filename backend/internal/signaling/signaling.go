package signaling

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/websocket/v2"
)

const (
	writeQueueSize = 64
	maxMessageSize = 64 * 1024
)

var (
	ErrOffline      = errors.New("signaling recipient is offline")
	ErrSlowConsumer = errors.New("signaling recipient queue is full")
)

// IdentityAuthenticator validates a B1-issued JWT and returns its account ID.
type IdentityAuthenticator interface {
	Authenticate(ctx context.Context, token string) (accountID string, err error)
}

// ContactLookup returns the authenticated account's current contact IDs.
type ContactLookup interface {
	ListConnections(ctx context.Context, accountID string) ([]string, error)
}

// ConnectHook delivers pending account-level events over an authenticated
// connection. B4 should make delivery and acknowledgement idempotent.
type ConnectHook func(ctx context.Context, accountID string, send func(payload any) error) error

type Hub struct {
	mu       sync.RWMutex
	accounts map[string]*accountConnections
}

type accountConnections struct {
	clients  map[*client]struct{}
	contacts map[string]struct{}
}

type client struct {
	outbound chan []byte
	done     chan struct{}
	mu       sync.Mutex
	closed   bool
}

type incomingEvent struct {
	Type    string          `json:"type"`
	To      string          `json:"to"`
	Payload json.RawMessage `json:"payload"`
}

type relayedEvent struct {
	Type    string          `json:"type"`
	From    string          `json:"from"`
	Payload json.RawMessage `json:"payload"`
}

type presenceEvent struct {
	Type      string `json:"type"`
	AccountID string `json:"accountId"`
	Status    string `json:"status"`
}

type errorEvent struct {
	Type string `json:"type"`
	Code string `json:"code"`
}

// NewHub creates an empty, concurrency-safe connection hub.
func NewHub() *Hub {
	return &Hub{accounts: make(map[string]*accountConnections)}
}

// SendTo sends a JSON-serializable payload to every active WebSocket belonging
// to accountID. It returns ErrOffline when no connection exists and
// ErrSlowConsumer when an active connection's bounded queue is full.
func (h *Hub) SendTo(accountID string, payload any) error {
	message, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal signaling payload: %w", err)
	}
	return h.sendEncoded(accountID, message)
}

// NewHandler returns a Fiber handler for mounting at /v1/ws. Authentication
// uses the Authorization: Bearer <JWT> header; JWT validation and account-ID
// extraction are delegated to IdentityAuthenticator.
func NewHandler(hub *Hub, auth IdentityAuthenticator, contactLookup ContactLookup, onConnect ConnectHook) (fiber.Handler, error) {
	if hub == nil {
		return nil, errors.New("signaling hub is required")
	}
	if auth == nil {
		return nil, errors.New("signaling identity authenticator is required")
	}
	if contactLookup == nil {
		return nil, errors.New("signaling contact lookup is required")
	}

	return func(c *fiber.Ctx) error {
		if !websocket.IsWebSocketUpgrade(c) {
			return fiber.NewError(fiber.StatusUpgradeRequired, "websocket upgrade required")
		}

		token := bearerToken(c.Get(fiber.HeaderAuthorization))
		if token == "" {
			return fiber.NewError(fiber.StatusUnauthorized, "bearer token required")
		}
		accountID, err := auth.Authenticate(c.UserContext(), token)
		if err != nil || strings.TrimSpace(accountID) == "" {
			return fiber.NewError(fiber.StatusUnauthorized, "invalid bearer token")
		}

		ctx := c.UserContext()
		contacts, err := contactLookup.ListConnections(ctx, accountID)
		if err != nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "contact lookup failed")
		}

		return websocket.New(func(conn *websocket.Conn) {
			hub.serve(conn, accountID, contacts, contactLookup, onConnect)
		})(c)
	}, nil
}

func bearerToken(header string) string {
	scheme, token, ok := strings.Cut(strings.TrimSpace(header), " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") {
		return ""
	}
	return strings.TrimSpace(token)
}

func (h *Hub) serve(conn *websocket.Conn, accountID string, contacts []string, contactLookup ContactLookup, onConnect ConnectHook) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	conn.SetReadLimit(maxMessageSize)
	clientConn := &client{
		outbound: make(chan []byte, writeQueueSize),
		done:     make(chan struct{}),
	}
	if err := h.connect(accountID, contacts, clientConn); err != nil {
		log.Printf("signaling: presence notification failed: %v", err)
	}
	defer h.disconnect(accountID, clientConn)

	writerDone := make(chan struct{})
	go func() {
		defer close(writerDone)
		defer conn.Close()
		for {
			select {
			case <-clientConn.done:
				return
			case message := <-clientConn.outbound:
				if err := conn.WriteMessage(websocket.TextMessage, message); err != nil {
					return
				}
			}
		}
	}()
	defer func() {
		clientConn.stop()
		_ = conn.Close()
		<-writerDone
	}()

	if err := runConnectHook(ctx, onConnect, accountID, clientConn); err != nil {
		log.Printf("signaling: authenticated-connect hook failed: %v", err)
		return
	}

	for {
		messageType, message, err := conn.ReadMessage()
		if err != nil {
			return
		}
		if messageType != websocket.TextMessage {
			return
		}

		var event incomingEvent
		if err := json.Unmarshal(message, &event); err != nil || !validIncomingEvent(event) {
			h.sendError(clientConn, "invalid_message")
			continue
		}
		if event.To == accountID {
			h.sendError(clientConn, "invalid_recipient")
			continue
		}

		if code := h.relay(ctx, accountID, event, contactLookup); code != "" {
			h.sendError(clientConn, code)
		}
	}
}

func (h *Hub) relay(ctx context.Context, accountID string, event incomingEvent, contactLookup ContactLookup) string {
	contacts, err := contactLookup.ListConnections(ctx, accountID)
	if err != nil {
		log.Printf("signaling: contact lookup failed while relaying: %v", err)
		return "service_unavailable"
	}
	if !contains(contacts, event.To) {
		return "not_connected"
	}

	message, err := json.Marshal(relayedEvent{
		Type:    event.Type,
		From:    accountID,
		Payload: event.Payload,
	})
	if err != nil {
		return "invalid_message"
	}
	if err := h.sendEncoded(event.To, message); err != nil {
		if errors.Is(err, ErrOffline) {
			return "peer_offline"
		}
		log.Printf("signaling: relay failed: %v", err)
		return "delivery_failed"
	}
	return ""
}

func runConnectHook(ctx context.Context, hook ConnectHook, accountID string, conn *client) error {
	if hook == nil {
		return nil
	}
	send := func(payload any) error {
		message, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("marshal connect-hook payload: %w", err)
		}
		return conn.enqueue(message)
	}
	return hook(ctx, accountID, send)
}

func validIncomingEvent(event incomingEvent) bool {
	if event.To == "" || len(event.Payload) == 0 || !json.Valid(event.Payload) {
		return false
	}
	return event.Type == "signal" || event.Type == "ice"
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func (h *Hub) sendError(conn *client, code string) {
	message, err := json.Marshal(errorEvent{Type: "error", Code: code})
	if err != nil {
		log.Printf("signaling: could not encode protocol error: %v", err)
		return
	}
	if err := conn.enqueue(message); err != nil {
		log.Printf("signaling: could not queue protocol error: %v", err)
	}
}

func (h *Hub) sendEncoded(accountID string, message []byte) error {
	h.mu.RLock()
	defer h.mu.RUnlock()
	account := h.accounts[accountID]
	if account == nil || len(account.clients) == 0 {
		return ErrOffline
	}

	var firstErr error
	for conn := range account.clients {
		if err := conn.enqueue(message); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (h *Hub) connect(accountID string, contacts []string, conn *client) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	account := h.accounts[accountID]
	firstConnection := account == nil
	if firstConnection {
		account = &accountConnections{
			clients:  make(map[*client]struct{}),
			contacts: make(map[string]struct{}),
		}
		for _, contactID := range contacts {
			if contactID != "" && contactID != accountID {
				account.contacts[contactID] = struct{}{}
			}
		}
		h.accounts[accountID] = account
	}
	account.clients[conn] = struct{}{}
	if !firstConnection {
		return nil
	}

	return h.broadcastPresenceLocked(accountID, account.contacts, "online")
}

func (h *Hub) disconnect(accountID string, conn *client) {
	h.mu.Lock()
	defer h.mu.Unlock()

	account := h.accounts[accountID]
	if account == nil {
		return
	}
	delete(account.clients, conn)
	conn.stop()
	if len(account.clients) > 0 {
		return
	}
	delete(h.accounts, accountID)
	if err := h.broadcastPresenceLocked(accountID, account.contacts, "offline"); err != nil {
		log.Printf("signaling: presence notification failed: %v", err)
	}
}

func (h *Hub) broadcastPresenceLocked(accountID string, contacts map[string]struct{}, status string) error {
	message, err := json.Marshal(presenceEvent{
		Type:      "presence",
		AccountID: accountID,
		Status:    status,
	})
	if err != nil {
		return fmt.Errorf("marshal presence event: %w", err)
	}

	var firstErr error
	for contactID := range contacts {
		account := h.accounts[contactID]
		if account == nil {
			continue
		}
		for conn := range account.clients {
			if err := conn.enqueue(message); err != nil && firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

func (c *client) enqueue(message []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return ErrOffline
	}
	select {
	case c.outbound <- message:
		return nil
	default:
		return ErrSlowConsumer
	}
}

func (c *client) stop() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return
	}
	c.closed = true
	close(c.done)
}
