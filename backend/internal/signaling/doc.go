// Package signaling implements authenticated WebSocket signaling and presence.
//
// The shared contracts/ws-protocol.md is still a placeholder. This package
// therefore uses a provisional JSON protocol pending a reviewed contract:
//
//   - Client to server: {"type":"signal"|"ice","to":"account-id","payload":{...}}
//   - Server to peer:   {"type":"signal"|"ice","from":"account-id","payload":{...}}
//   - Presence:         {"type":"presence","accountId":"account-id","status":"online"|"offline"}
//   - Errors:           {"type":"error","code":"invalid_message"|"invalid_recipient"|"not_connected"|"peer_offline"|"service_unavailable"|"delivery_failed"}
//   - Transfer event:   {"type":"transfer","payload":{...}}
//   - Offline message:  {"type":"offline_message","payload":{...}}
//
// The payload is forwarded as opaque JSON; SDP and ICE contents are not
// interpreted or persisted. JWTs are accepted only in the Authorization
// Bearer header. Presence changes and signaling are limited to accounts
// returned by ContactLookup.ListConnections. Presence is emitted on an
// account's first active WebSocket and final disconnect; each connected
// contact's active sockets receive the event.
// Transfer and offline-message payload contents belong to B5 and B4,
// respectively; this package only transports their JSON envelope.
//
// B1 integration contract:
//
//	type IdentityAuthenticator interface {
//	    Authenticate(ctx context.Context, token string) (accountID string, err error)
//	}
//
// B1's claim key is not assumed here because its implementation is not yet
// integrated. Authenticate must validate JWT signature, issuer, expiration,
// and its account identity claim, then return the stable account ID. This
// package does not parse or validate JWTs itself.
//
// B3 integration contract:
//
//	type ContactLookup interface {
//	    ListConnections(ctx context.Context, accountID string) ([]string, error)
//	}
//
// It returns the account IDs currently connected to accountID, or an error.
// The current list is checked for each signaling relay; the initial list is
// used to route online/offline transitions. Contact revocation during an
// existing WebSocket can therefore leave presence routing based on the
// connection-time snapshot until all of that account's WebSockets disconnect.
//
// B4 integration contract:
//
//	type ConnectHook func(ctx context.Context, accountID string, send func(payload any) error) error
//
// The hook runs once per authenticated WebSocket connection, after the socket
// writer starts. It should send opaque offline-message events with the
// provided sender and retain its own acknowledgement/idempotency behavior.
// Payloads are JSON-marshaled by this package and queued for that connection;
// a hook error is logged and closes that socket.
//
// B5 integration contract:
//
//	func (h *Hub) SendTo(accountID string, payload any) error
//
// SendTo broadcasts a JSON-serializable event to all active sockets for the
// account. ErrOffline means no active socket exists; callers should treat that
// as a normal offline condition and must not assume delivery. ErrSlowConsumer
// means at least one socket could not accept the event; other sockets may
// already have received it, so callers should not blindly retry non-idempotent
// events.
package signaling
