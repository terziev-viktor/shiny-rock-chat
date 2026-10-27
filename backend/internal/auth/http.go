package auth

import (
	"encoding/base64"
	"errors"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

const accountIDLocalKey = "auth.accountID"

type challengeRequest struct {
	PublicKey string           `json:"publicKey"`
	Purpose   ChallengePurpose `json:"purpose"`
}

type signedChallengeRequest struct {
	PublicKey string `json:"publicKey"`
	Challenge string `json:"challengeId"`
	Signature string `json:"signature"`
	Proof     string `json:"proof,omitempty"`
}

type challengeResponse struct {
	ChallengeID string    `json:"challengeId"`
	Purpose     string    `json:"purpose"`
	Nonce       string    `json:"nonce"`
	ExpiresAt   time.Time `json:"expiresAt"`
}

type tokenResponse struct {
	AccountID      string    `json:"accountId"`
	KeyFingerprint string    `json:"keyFingerprint"`
	QRPayload      string    `json:"qrPayload"`
	AccessToken    string    `json:"accessToken"`
	TokenType      string    `json:"tokenType"`
	ExpiresAt      time.Time `json:"expiresAt"`
}

type accountResponse struct {
	AccountID      string    `json:"accountId"`
	KeyFingerprint string    `json:"keyFingerprint"`
	QRPayload      string    `json:"qrPayload"`
	CreatedAt      time.Time `json:"createdAt"`
	ShinyRock      int64     `json:"shinyRock"`
}

// RegisterRoutes installs B1 auth and account endpoints on router.
func RegisterRoutes(router fiber.Router, service *Service) {
	router.Post("/v1/auth/challenge", service.handleChallenge)
	router.Post("/v1/auth/register", service.handleRegister)
	router.Post("/v1/auth/login", service.handleLogin)
	router.Get("/v1/account/me", service.Middleware(), service.handleMe)
}

// Middleware validates an HS256 access token and stores the account UUID on Fiber locals.
func (s *Service) Middleware() fiber.Handler {
	return func(c *fiber.Ctx) error {
		header := c.Get(fiber.HeaderAuthorization)
		if !strings.HasPrefix(header, "Bearer ") {
			return unauthorized(c)
		}
		rawToken := strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
		if rawToken == "" {
			return unauthorized(c)
		}
		claims := &jwt.RegisteredClaims{}
		token, err := jwt.ParseWithClaims(rawToken, claims, func(token *jwt.Token) (any, error) {
			if token.Method != jwt.SigningMethodHS256 {
				return nil, errors.New("unexpected JWT signing method")
			}
			return s.jwtSecret, nil
		}, jwt.WithIssuer(s.issuer), jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}))
		if err != nil || token == nil || !token.Valid {
			return unauthorized(c)
		}
		accountID, err := uuid.Parse(claims.Subject)
		if err != nil || accountID.String() != claims.Subject {
			return unauthorized(c)
		}
		c.Locals(accountIDLocalKey, accountID.String())
		return c.Next()
	}
}

// AccountIDFromContext returns the authenticated account ID set by Middleware.
func AccountIDFromContext(c *fiber.Ctx) (uuid.UUID, bool) {
	accountID, ok := c.Locals(accountIDLocalKey).(string)
	if !ok {
		return uuid.Nil, false
	}
	parsed, err := uuid.Parse(accountID)
	return parsed, err == nil
}

func (s *Service) handleChallenge(c *fiber.Ctx) error {
	var request challengeRequest
	if err := c.BodyParser(&request); err != nil {
		return badRequest(c, "invalid JSON body")
	}
	publicKey, err := decodeBase64(request.PublicKey)
	if err != nil {
		return badRequest(c, "publicKey must be base64url-encoded Ed25519 public key")
	}
	challenge, err := s.CreateChallenge(c.UserContext(), publicKey, request.Purpose)
	if err != nil {
		if errors.Is(err, ErrInvalidChallenge) {
			return badRequest(c, "invalid challenge request")
		}
		return err
	}
	return c.Status(fiber.StatusCreated).JSON(challengeResponse{
		ChallengeID: challenge.ID.String(),
		Purpose:     string(challenge.Purpose),
		Nonce:       base64.RawURLEncoding.EncodeToString(challenge.Nonce),
		ExpiresAt:   challenge.ExpiresAt,
	})
}

func (s *Service) handleRegister(c *fiber.Ctx) error {
	request, challengeID, publicKey, signature, err := parseSignedChallenge(c)
	if err != nil {
		return badRequest(c, err.Error())
	}
	user, token, expiresAt, err := s.Register(c.UserContext(), challengeID, publicKey, signature, request.Proof)
	if err != nil {
		return respondAuthError(c, err)
	}
	return c.Status(fiber.StatusCreated).JSON(tokenResponse{
		AccountID:      user.AccountID.String(),
		KeyFingerprint: user.KeyFingerprint,
		QRPayload:      user.QRPayload,
		AccessToken:    token,
		TokenType:      "Bearer",
		ExpiresAt:      expiresAt,
	})
}

func (s *Service) handleLogin(c *fiber.Ctx) error {
	_, challengeID, publicKey, signature, err := parseSignedChallenge(c)
	if err != nil {
		return badRequest(c, err.Error())
	}
	user, token, expiresAt, err := s.Login(c.UserContext(), challengeID, publicKey, signature)
	if err != nil {
		return respondAuthError(c, err)
	}
	return c.JSON(tokenResponse{
		AccountID:      user.AccountID.String(),
		KeyFingerprint: user.KeyFingerprint,
		QRPayload:      user.QRPayload,
		AccessToken:    token,
		TokenType:      "Bearer",
		ExpiresAt:      expiresAt,
	})
}

func (s *Service) handleMe(c *fiber.Ctx) error {
	accountID, ok := AccountIDFromContext(c)
	if !ok {
		return unauthorized(c)
	}
	user, err := s.repository.GetUserByAccountID(c.UserContext(), accountID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return unauthorized(c)
		}
		return err
	}
	return c.JSON(accountResponse{
		AccountID:      user.AccountID.String(),
		KeyFingerprint: user.KeyFingerprint,
		QRPayload:      user.QRPayload,
		CreatedAt:      user.CreatedAt,
		ShinyRock:      user.ShinyRock,
	})
}

func parseSignedChallenge(c *fiber.Ctx) (signedChallengeRequest, uuid.UUID, []byte, []byte, error) {
	var request signedChallengeRequest
	if err := c.BodyParser(&request); err != nil {
		return request, uuid.Nil, nil, nil, errors.New("invalid JSON body")
	}
	publicKey, err := decodeBase64(request.PublicKey)
	if err != nil || len(publicKey) != 32 {
		return request, uuid.Nil, nil, nil, errors.New("publicKey must be base64url-encoded Ed25519 public key")
	}
	challengeID, err := uuid.Parse(request.Challenge)
	if err != nil || challengeID.String() != request.Challenge {
		return request, uuid.Nil, nil, nil, errors.New("challengeId must be a UUID")
	}
	signature, err := decodeBase64(request.Signature)
	if err != nil || len(signature) != 64 {
		return request, uuid.Nil, nil, nil, errors.New("signature must be base64url-encoded Ed25519 signature")
	}
	return request, challengeID, publicKey, signature, nil
}

func decodeBase64(encoded string) ([]byte, error) {
	if encoded == "" {
		return nil, errors.New("value is required")
	}
	return base64.RawURLEncoding.DecodeString(encoded)
}

func respondAuthError(c *fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, ErrConflict):
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "account already exists"})
	case errors.Is(err, ErrInvalidCredential), errors.Is(err, ErrChallengeInvalid):
		return unauthorized(c)
	case errors.Is(err, ErrInvalidProof):
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "registration proof is invalid"})
	default:
		return err
	}
}

func badRequest(c *fiber.Ctx, message string) error {
	return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": message})
}

func unauthorized(c *fiber.Ctx) error {
	return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
}
