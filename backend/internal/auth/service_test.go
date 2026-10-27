package auth

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/yourorg/srock-chat/backend/internal/qr"
)

func TestRegisterLoginAndAuthenticatedAccount(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	repository := newMemoryRepository()
	service := newTestService(t, repository)
	app := fiber.New()
	RegisterRoutes(app, service)

	registerChallenge := requestChallenge(t, app, publicKey, PurposeRegister)
	registerResponse := signedRequest(t, app, "/v1/auth/register", registerChallenge, privateKey, "valid")
	if registerResponse.StatusCode != fiber.StatusCreated {
		t.Fatalf("register status = %d, want %d", registerResponse.StatusCode, fiber.StatusCreated)
	}
	var registered tokenResponse
	decodeResponse(t, registerResponse, &registered)
	identity, err := qr.Verify(registered.QRPayload)
	if err != nil {
		t.Fatalf("verify registration QR: %v", err)
	}
	if identity.AccountID != registered.AccountID {
		t.Fatalf("QR account ID = %q, response account ID = %q", identity.AccountID, registered.AccountID)
	}

	meRequest := httptest.NewRequest("GET", "/v1/account/me", nil)
	meRequest.Header.Set(fiber.HeaderAuthorization, "Bearer "+registered.AccessToken)
	meResponse, err := app.Test(meRequest)
	if err != nil {
		t.Fatal(err)
	}
	if meResponse.StatusCode != fiber.StatusOK {
		t.Fatalf("account/me status = %d, want %d", meResponse.StatusCode, fiber.StatusOK)
	}
	var account accountResponse
	decodeResponse(t, meResponse, &account)
	if account.AccountID != registered.AccountID || account.QRPayload != registered.QRPayload {
		t.Fatalf("account/me changed identity: %+v", account)
	}

	loginChallenge := requestChallenge(t, app, publicKey, PurposeLogin)
	loginResponse := signedRequest(t, app, "/v1/auth/login", loginChallenge, privateKey, "")
	if loginResponse.StatusCode != fiber.StatusOK {
		t.Fatalf("login status = %d, want %d", loginResponse.StatusCode, fiber.StatusOK)
	}
	var loggedIn tokenResponse
	decodeResponse(t, loginResponse, &loggedIn)
	if loggedIn.AccountID != registered.AccountID || loggedIn.QRPayload != registered.QRPayload {
		t.Fatalf("login changed identity: %+v", loggedIn)
	}
}

func TestRegistrationRejectsDuplicateKeyAndInvalidProof(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	repository := newMemoryRepository()
	service := newTestService(t, repository)
	app := fiber.New()
	RegisterRoutes(app, service)

	challenge := requestChallenge(t, app, publicKey, PurposeRegister)
	response := signedRequest(t, app, "/v1/auth/register", challenge, privateKey, "invalid")
	if response.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("invalid proof status = %d, want %d", response.StatusCode, fiber.StatusUnauthorized)
	}

	challenge = requestChallenge(t, app, publicKey, PurposeRegister)
	response = signedRequest(t, app, "/v1/auth/register", challenge, privateKey, "valid")
	if response.StatusCode != fiber.StatusCreated {
		t.Fatalf("first registration status = %d, want %d", response.StatusCode, fiber.StatusCreated)
	}
	challenge = requestChallenge(t, app, publicKey, PurposeRegister)
	response = signedRequest(t, app, "/v1/auth/register", challenge, privateKey, "valid")
	if response.StatusCode != fiber.StatusConflict {
		t.Fatalf("duplicate registration status = %d, want %d", response.StatusCode, fiber.StatusConflict)
	}
}

func TestLoginRejectsInvalidSignatureAndToken(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	app := fiber.New()
	RegisterRoutes(app, newTestService(t, newMemoryRepository()))

	challenge := requestChallenge(t, app, publicKey, PurposeLogin)
	response := signedRequest(t, app, "/v1/auth/login", challenge, privateKey, "")
	if response.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("unknown account status = %d, want %d", response.StatusCode, fiber.StatusUnauthorized)
	}

	challenge = requestChallenge(t, app, publicKey, PurposeLogin)
	requestBody := signedChallengeRequest{
		PublicKey: base64.RawURLEncoding.EncodeToString(publicKey),
		Challenge: challenge.ID.String(),
		Signature: base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{1}, ed25519.SignatureSize)),
	}
	body, err := json.Marshal(requestBody)
	if err != nil {
		t.Fatal(err)
	}
	httpRequest := httptest.NewRequest("POST", "/v1/auth/login", bytes.NewReader(body))
	httpRequest.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
	response, err = app.Test(httpRequest)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("invalid signature status = %d, want %d", response.StatusCode, fiber.StatusUnauthorized)
	}

	httpRequest = httptest.NewRequest("GET", "/v1/account/me", nil)
	httpRequest.Header.Set(fiber.HeaderAuthorization, "Bearer invalid")
	response, err = app.Test(httpRequest)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("invalid token status = %d, want %d", response.StatusCode, fiber.StatusUnauthorized)
	}
}

func TestNewServiceRequiresProofVerifierAndStrongJWTSecret(t *testing.T) {
	repository := newMemoryRepository()
	if _, err := NewService(repository, nil, Config{JWTSecret: bytes.Repeat([]byte{1}, 32), JWTIssuer: "test"}); err == nil {
		t.Fatal("NewService accepted nil proof verifier")
	}
	if _, err := NewService(repository, allowProof{}, Config{JWTSecret: []byte("short"), JWTIssuer: "test"}); err == nil {
		t.Fatal("NewService accepted a short JWT secret")
	}
}

type allowProof struct{}

func (allowProof) Verify(_ context.Context, _, _ []byte, proof string) error {
	if proof != "valid" {
		return ErrInvalidProof
	}
	return nil
}

type memoryRepository struct {
	challenges map[uuid.UUID]Challenge
	usersByKey map[string]User
	usersByID  map[uuid.UUID]User
}

func newMemoryRepository() *memoryRepository {
	return &memoryRepository{
		challenges: make(map[uuid.UUID]Challenge),
		usersByKey: make(map[string]User),
		usersByID:  make(map[uuid.UUID]User),
	}
}

func (r *memoryRepository) CreateChallenge(_ context.Context, challenge Challenge) error {
	r.challenges[challenge.ID] = challenge
	return nil
}

func (r *memoryRepository) GetChallenge(_ context.Context, id uuid.UUID) (Challenge, error) {
	challenge, ok := r.challenges[id]
	if !ok || challenge.ExpiresAt.Before(time.Now()) {
		return Challenge{}, ErrChallengeInvalid
	}
	return challenge, nil
}

func (r *memoryRepository) ConsumeChallenge(_ context.Context, id uuid.UUID, publicKey []byte, purpose ChallengePurpose, now time.Time) error {
	challenge, ok := r.challenges[id]
	if !ok || challenge.Purpose != purpose || !equalBytes(challenge.PublicKey, publicKey) || !challenge.ExpiresAt.After(now) {
		return ErrChallengeInvalid
	}
	delete(r.challenges, id)
	return nil
}

func (r *memoryRepository) CreateUser(_ context.Context, user User) error {
	key := string(user.PublicKey)
	if _, exists := r.usersByKey[key]; exists {
		return ErrConflict
	}
	r.usersByKey[key] = user
	r.usersByID[user.AccountID] = user
	return nil
}

func (r *memoryRepository) GetUserByPublicKey(_ context.Context, publicKey []byte) (User, error) {
	user, ok := r.usersByKey[string(publicKey)]
	if !ok {
		return User{}, ErrNotFound
	}
	return user, nil
}

func (r *memoryRepository) GetUserByAccountID(_ context.Context, accountID uuid.UUID) (User, error) {
	user, ok := r.usersByID[accountID]
	if !ok {
		return User{}, ErrNotFound
	}
	return user, nil
}

func newTestService(t *testing.T, repository Repository) *Service {
	t.Helper()
	service, err := NewService(repository, allowProof{}, Config{
		JWTSecret: []byte("0123456789abcdef0123456789abcdef"),
		JWTIssuer: "b1-test",
	})
	if err != nil {
		t.Fatal(err)
	}
	return service
}

func requestChallenge(t *testing.T, app *fiber.App, publicKey []byte, purpose ChallengePurpose) Challenge {
	t.Helper()
	requestBody, err := json.Marshal(challengeRequest{
		PublicKey: base64.RawURLEncoding.EncodeToString(publicKey),
		Purpose:   purpose,
	})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest("POST", "/v1/auth/challenge", bytes.NewReader(requestBody))
	request.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
	response, err := app.Test(request)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != fiber.StatusCreated {
		t.Fatalf("challenge status = %d, want %d", response.StatusCode, fiber.StatusCreated)
	}
	var wire challengeResponse
	decodeResponse(t, response, &wire)
	nonce, err := base64.RawURLEncoding.DecodeString(wire.Nonce)
	if err != nil {
		t.Fatal(err)
	}
	id, err := uuid.Parse(wire.ChallengeID)
	if err != nil {
		t.Fatal(err)
	}
	return Challenge{
		ID:        id,
		PublicKey: publicKey,
		Purpose:   purpose,
		Nonce:     nonce,
		ExpiresAt: wire.ExpiresAt,
	}
}

func signedRequest(t *testing.T, app *fiber.App, path string, challenge Challenge, privateKey ed25519.PrivateKey, proof string) *http.Response {
	t.Helper()
	body, err := json.Marshal(signedChallengeRequest{
		PublicKey: base64.RawURLEncoding.EncodeToString(challenge.PublicKey),
		Challenge: challenge.ID.String(),
		Signature: base64.RawURLEncoding.EncodeToString(ed25519.Sign(privateKey, ChallengeMessage(challenge))),
		Proof:     proof,
	})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest("POST", path, bytes.NewReader(body))
	request.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
	response, err := app.Test(request)
	if err != nil {
		t.Fatal(err)
	}
	return response
}

func decodeResponse(t *testing.T, response *http.Response, target any) {
	t.Helper()
	defer response.Body.Close()
	if err := json.NewDecoder(response.Body).Decode(target); err != nil {
		t.Fatal(err)
	}
}
