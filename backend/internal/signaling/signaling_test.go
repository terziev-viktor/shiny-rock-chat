package signaling

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
)

type testAuthenticator struct {
	accounts map[string]string
}

func (a testAuthenticator) Authenticate(_ context.Context, token string) (string, error) {
	accountID, ok := a.accounts[token]
	if !ok {
		return "", errors.New("invalid test token")
	}
	return accountID, nil
}

type testContactLookup struct {
	mu        sync.RWMutex
	connected map[string][]string
	err       error
}

func (l *testContactLookup) ListConnections(_ context.Context, accountID string) ([]string, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	if l.err != nil {
		return nil, l.err
	}
	return append([]string(nil), l.connected[accountID]...), nil
}

func newTestApp(t *testing.T, hub *Hub, lookup ContactLookup, hook ConnectHook) *fiber.App {
	t.Helper()
	app := fiber.New()
	handler, err := NewHandler(hub, testAuthenticator{
		accounts: map[string]string{"alice-token": "alice", "bob-token": "bob"},
	}, lookup, hook)
	if err != nil {
		t.Fatal(err)
	}
	app.Get("/v1/ws", handler)
	return app
}

type testWebSocket struct {
	conn   net.Conn
	reader *bufio.Reader
}

func startNetworkTestServer(t *testing.T, hub *Hub, lookup ContactLookup, hook ConnectHook) string {
	t.Helper()
	app := newTestApp(t, hub, lookup, hook)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	serverErrors := make(chan error, 1)
	go func() {
		serverErrors <- app.Listener(listener)
	}()
	t.Cleanup(func() {
		_ = app.Shutdown()
		if err := <-serverErrors; err != nil {
			t.Errorf("Fiber listener failed: %v", err)
		}
	})
	return "ws://" + listener.Addr().String() + "/v1/ws"
}

func dialTestWebSocket(t *testing.T, endpoint, token string) *testWebSocket {
	t.Helper()
	address, path, ok := strings.Cut(strings.TrimPrefix(endpoint, "ws://"), "/")
	if !ok {
		t.Fatalf("invalid test WebSocket URL %q", endpoint)
	}
	conn, err := net.Dial("tcp", address)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		t.Fatal(err)
	}
	key := base64.StdEncoding.EncodeToString(nonce[:])
	if _, err := fmt.Fprintf(conn, "GET /%s HTTP/1.1\r\nHost: %s\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Key: %s\r\nSec-WebSocket-Version: 13\r\nAuthorization: Bearer %s\r\n\r\n", path, address, key, token); err != nil {
		t.Fatal(err)
	}
	reader := bufio.NewReader(conn)
	status, err := reader.ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(status, " 101 ") {
		t.Fatalf("WebSocket upgrade failed: %s", strings.TrimSpace(status))
	}
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatal(err)
		}
		if line == "\r\n" {
			break
		}
	}
	return &testWebSocket{conn: conn, reader: reader}
}

func (c *testWebSocket) writeText(t *testing.T, payload []byte) {
	t.Helper()
	frame := []byte{0x81}
	switch {
	case len(payload) < 126:
		frame = append(frame, byte(0x80|len(payload)))
	case len(payload) <= 65535:
		frame = append(frame, 0x80|126, byte(len(payload)>>8), byte(len(payload)))
	default:
		t.Fatalf("test frame exceeds 65535 bytes")
	}
	var mask [4]byte
	if _, err := rand.Read(mask[:]); err != nil {
		t.Fatal(err)
	}
	frame = append(frame, mask[:]...)
	for i, b := range payload {
		frame = append(frame, b^mask[i%len(mask)])
	}
	for len(frame) > 0 {
		n, err := c.conn.Write(frame)
		if err != nil {
			t.Fatal(err)
		}
		if n == 0 {
			t.Fatal(io.ErrShortWrite)
		}
		frame = frame[n:]
	}
}

func (c *testWebSocket) readText(t *testing.T) []byte {
	t.Helper()
	if err := c.conn.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	var header [2]byte
	if _, err := io.ReadFull(c.reader, header[:]); err != nil {
		t.Fatal(err)
	}
	if header[0]&0x0f != 1 {
		t.Fatalf("expected a text frame, got opcode %d", header[0]&0x0f)
	}
	length := uint64(header[1] & 0x7f)
	switch length {
	case 126:
		var extended [2]byte
		if _, err := io.ReadFull(c.reader, extended[:]); err != nil {
			t.Fatal(err)
		}
		length = uint64(binary.BigEndian.Uint16(extended[:]))
	case 127:
		var extended [8]byte
		if _, err := io.ReadFull(c.reader, extended[:]); err != nil {
			t.Fatal(err)
		}
		length = binary.BigEndian.Uint64(extended[:])
	}
	if length > maxMessageSize {
		t.Fatalf("server frame exceeds test limit: %d bytes", length)
	}
	payload := make([]byte, int(length))
	if _, err := io.ReadFull(c.reader, payload); err != nil {
		t.Fatal(err)
	}
	if header[1]&0x80 != 0 {
		t.Fatal("server sent a masked frame")
	}
	return payload
}

func TestWebSocketClientsRelaySignalAndICE(t *testing.T) {
	lookup := &testContactLookup{connected: map[string][]string{
		"alice": {"bob"},
		"bob":   {"alice"},
	}}
	endpoint := startNetworkTestServer(t, NewHub(), lookup, nil)
	alice := dialTestWebSocket(t, endpoint, "alice-token")
	bob := dialTestWebSocket(t, endpoint, "bob-token")

	var online presenceEvent
	if err := json.Unmarshal(alice.readText(t), &online); err != nil {
		t.Fatal(err)
	}
	if online.AccountID != "bob" || online.Status != "online" {
		t.Fatalf("unexpected Bob online event: %#v", online)
	}

	for _, input := range []string{
		`{"type":"signal","to":"bob","payload":{"description":{"type":"offer","sdp":"opaque-sdp"}}}`,
		`{"type":"ice","to":"bob","payload":{"candidate":"opaque-candidate"}}`,
	} {
		var sent incomingEvent
		if err := json.Unmarshal([]byte(input), &sent); err != nil {
			t.Fatal(err)
		}
		alice.writeText(t, []byte(input))
		var event relayedEvent
		if err := json.Unmarshal(bob.readText(t), &event); err != nil {
			t.Fatal(err)
		}
		if event.From != "alice" || event.Type != sent.Type || string(event.Payload) != string(sent.Payload) {
			t.Fatalf("unexpected relayed event: %#v", event)
		}
	}

	_ = bob.conn.Close()
	var offline presenceEvent
	if err := json.Unmarshal(alice.readText(t), &offline); err != nil {
		t.Fatal(err)
	}
	if offline.AccountID != "bob" || offline.Status != "offline" {
		t.Fatalf("unexpected Bob offline event: %#v", offline)
	}
}

func TestHandlerRequiresBearerJWTAndWebSocketUpgrade(t *testing.T) {
	app := newTestApp(t, NewHub(), &testContactLookup{connected: map[string][]string{}}, nil)
	request := httptest.NewRequest(http.MethodGet, "/v1/ws", nil)
	response, err := app.Test(request)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != fiber.StatusUpgradeRequired {
		t.Fatalf("expected %d, got %d", fiber.StatusUpgradeRequired, response.StatusCode)
	}

	request = httptest.NewRequest(http.MethodGet, "/v1/ws", nil)
	request.Header.Set("Upgrade", "websocket")
	request.Header.Set("Connection", "Upgrade")
	request.Header.Set("Authorization", "Bearer unknown-token")
	response, err = app.Test(request)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("expected %d, got %d", fiber.StatusUnauthorized, response.StatusCode)
	}
}

func TestConnectHookSendsPendingPayload(t *testing.T) {
	hook := func(_ context.Context, accountID string, send func(any) error) error {
		if accountID != "alice" {
			t.Errorf("hook account ID = %q, want alice", accountID)
		}
		return send(map[string]any{
			"type":    "offline_message",
			"payload": map[string]string{"messageId": "message-1"},
		})
	}
	endpoint := startNetworkTestServer(t, NewHub(), &testContactLookup{connected: map[string][]string{}}, hook)
	alice := dialTestWebSocket(t, endpoint, "alice-token")
	message := alice.readText(t)
	var event map[string]any
	if err := json.Unmarshal(message, &event); err != nil {
		t.Fatal(err)
	}
	payload, ok := event["payload"].(map[string]any)
	if event["type"] != "offline_message" || !ok || payload["messageId"] != "message-1" {
		t.Fatalf("unexpected hook payload: %#v", event)
	}
}

func TestSendToOfflineAndActiveRecipient(t *testing.T) {
	hub := NewHub()
	if err := hub.SendTo("offline", map[string]string{"type": "transfer"}); !errors.Is(err, ErrOffline) {
		t.Fatalf("SendTo offline error = %v, want ErrOffline", err)
	}

	conn := &client{outbound: make(chan []byte, 1), done: make(chan struct{})}
	if err := hub.connect("recipient", nil, conn); err != nil {
		t.Fatal(err)
	}
	if err := hub.SendTo("recipient", map[string]string{"type": "transfer"}); err != nil {
		t.Fatalf("SendTo active recipient: %v", err)
	}
	var event map[string]string
	if err := json.Unmarshal(<-conn.outbound, &event); err != nil {
		t.Fatal(err)
	}
	if event["type"] != "transfer" {
		t.Fatalf("unexpected payload: %#v", event)
	}
	hub.disconnect("recipient", conn)
	if err := hub.SendTo("recipient", map[string]string{"type": "transfer"}); !errors.Is(err, ErrOffline) {
		t.Fatalf("SendTo after disconnect error = %v, want ErrOffline", err)
	}
}

func TestPresenceOnlyReachesConnectedContacts(t *testing.T) {
	hub := NewHub()
	bob := &client{outbound: make(chan []byte, 2), done: make(chan struct{})}
	charlie := &client{outbound: make(chan []byte, 2), done: make(chan struct{})}
	alice := &client{outbound: make(chan []byte, 2), done: make(chan struct{})}

	if err := hub.connect("bob", nil, bob); err != nil {
		t.Fatal(err)
	}
	if err := hub.connect("charlie", nil, charlie); err != nil {
		t.Fatal(err)
	}
	if err := hub.connect("alice", []string{"bob"}, alice); err != nil {
		t.Fatal(err)
	}

	var event presenceEvent
	if err := json.Unmarshal(<-bob.outbound, &event); err != nil {
		t.Fatal(err)
	}
	if event.AccountID != "alice" || event.Status != "online" {
		t.Fatalf("unexpected contact presence: %#v", event)
	}
	select {
	case data := <-charlie.outbound:
		t.Fatalf("unrelated account received presence: %s", data)
	default:
	}

	hub.disconnect("alice", alice)
	if err := json.Unmarshal(<-bob.outbound, &event); err != nil {
		t.Fatal(err)
	}
	if event.AccountID != "alice" || event.Status != "offline" {
		t.Fatalf("unexpected contact offline event: %#v", event)
	}
	select {
	case data := <-charlie.outbound:
		t.Fatalf("unrelated account received offline presence: %s", data)
	default:
	}
	hub.disconnect("bob", bob)
	hub.disconnect("charlie", charlie)
}

func TestRelayRefusesNonContact(t *testing.T) {
	hub := NewHub()
	charlie := &client{outbound: make(chan []byte, 1), done: make(chan struct{})}
	if err := hub.connect("charlie", nil, charlie); err != nil {
		t.Fatal(err)
	}
	lookup := &testContactLookup{connected: map[string][]string{"alice": {"bob"}}}
	event := incomingEvent{
		Type:    "signal",
		To:      "charlie",
		Payload: json.RawMessage(`{"description":{"type":"offer"}}`),
	}
	if code := hub.relay(context.Background(), "alice", event, lookup); code != "not_connected" {
		t.Fatalf("relay code = %q, want not_connected", code)
	}
	select {
	case data := <-charlie.outbound:
		t.Fatalf("non-contact received signaling payload: %s", data)
	default:
	}
	hub.disconnect("charlie", charlie)
}

func TestConcurrentConnectSendAndDisconnect(t *testing.T) {
	hub := NewHub()
	const workers = 32
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			conn := &client{outbound: make(chan []byte, writeQueueSize), done: make(chan struct{})}
			if err := hub.connect("shared-account", nil, conn); err != nil {
				t.Errorf("connect: %v", err)
				return
			}
			for j := 0; j < 5; j++ {
				if err := hub.SendTo("shared-account", map[string]int{"event": j}); err != nil && !errors.Is(err, ErrSlowConsumer) {
					t.Errorf("SendTo: %v", err)
				}
				select {
				case <-conn.outbound:
				default:
				}
			}
			hub.disconnect("shared-account", conn)
		}()
	}
	wg.Wait()
	if err := hub.SendTo("shared-account", map[string]string{"type": "after"}); !errors.Is(err, ErrOffline) {
		t.Fatalf("SendTo after concurrent disconnect error = %v, want ErrOffline", err)
	}
}
