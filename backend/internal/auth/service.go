package auth

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/yourorg/srock-chat/backend/internal/qr"
)

const (
	defaultChallengeTTL = 5 * time.Minute
	defaultTokenTTL     = 15 * time.Minute
	nonceSize           = 32
)

// Config contains the secret and lifetimes used by the auth service.
type Config struct {
	JWTSecret    []byte
	JWTIssuer    string
	ChallengeTTL time.Duration
	TokenTTL     time.Duration
}

// Service owns authentication, registration, and account identity operations.
type Service struct {
	repository   Repository
	proof        ProofVerifier
	jwtSecret    []byte
	issuer       string
	challengeTTL time.Duration
	tokenTTL     time.Duration
	now          func() time.Time
}

// NewService creates an auth service. A proof verifier is required so registration
// cannot silently bypass the plan's proof-of-work gate.
func NewService(repository Repository, proof ProofVerifier, config Config) (*Service, error) {
	if repository == nil {
		return nil, errors.New("auth repository is required")
	}
	if proof == nil {
		return nil, errors.New("registration proof verifier is required")
	}
	if len(config.JWTSecret) < 32 {
		return nil, errors.New("JWT secret must be at least 32 bytes")
	}
	if config.JWTIssuer == "" {
		return nil, errors.New("JWT issuer is required")
	}
	if config.ChallengeTTL <= 0 {
		config.ChallengeTTL = defaultChallengeTTL
	}
	if config.TokenTTL <= 0 {
		config.TokenTTL = defaultTokenTTL
	}
	return &Service{
		repository:   repository,
		proof:        proof,
		jwtSecret:    append([]byte(nil), config.JWTSecret...),
		issuer:       config.JWTIssuer,
		challengeTTL: config.ChallengeTTL,
		tokenTTL:     config.TokenTTL,
		now:          time.Now,
	}, nil
}

// CreateChallenge stores a one-time challenge bound to the given Ed25519 key.
func (s *Service) CreateChallenge(ctx context.Context, publicKey []byte, purpose ChallengePurpose) (Challenge, error) {
	if len(publicKey) != ed25519.PublicKeySize {
		return Challenge{}, fmt.Errorf("%w: Ed25519 public key must be 32 bytes", ErrInvalidChallenge)
	}
	if purpose != PurposeRegister && purpose != PurposeLogin {
		return Challenge{}, fmt.Errorf("%w: unsupported purpose", ErrInvalidChallenge)
	}
	nonce := make([]byte, nonceSize)
	if _, err := rand.Read(nonce); err != nil {
		return Challenge{}, fmt.Errorf("generate auth nonce: %w", err)
	}
	id, err := uuid.NewRandom()
	if err != nil {
		return Challenge{}, fmt.Errorf("generate challenge ID: %w", err)
	}
	challenge := Challenge{
		ID:        id,
		PublicKey: append([]byte(nil), publicKey...),
		Purpose:   purpose,
		Nonce:     nonce,
		ExpiresAt: s.now().Add(s.challengeTTL),
	}
	if err := s.repository.CreateChallenge(ctx, challenge); err != nil {
		return Challenge{}, fmt.Errorf("store auth challenge: %w", err)
	}
	return challenge, nil
}

// ChallengeMessage is the exact UTF-8 message clients sign with their Ed25519 private key.
func ChallengeMessage(challenge Challenge) []byte {
	return []byte("shiny-rock-auth-v1\n" +
		string(challenge.Purpose) + "\n" +
		challenge.ID.String() + "\n" +
		base64.RawURLEncoding.EncodeToString(challenge.Nonce) + "\n" +
		base64.RawURLEncoding.EncodeToString(challenge.PublicKey))
}

// Register verifies proof-of-possession and proof-of-work before creating an account.
func (s *Service) Register(ctx context.Context, challengeID uuid.UUID, publicKey, signature []byte, proof string) (User, string, time.Time, error) {
	challenge, err := s.validChallenge(ctx, challengeID, publicKey, PurposeRegister)
	if err != nil {
		return User{}, "", time.Time{}, challengeError(err)
	}
	if !ed25519.Verify(ed25519.PublicKey(publicKey), ChallengeMessage(challenge), signature) {
		return User{}, "", time.Time{}, ErrInvalidCredential
	}
	if err := s.proof.Verify(ctx, publicKey, challenge.Nonce, proof); err != nil {
		if errors.Is(err, ErrInvalidProof) {
			return User{}, "", time.Time{}, ErrInvalidProof
		}
		return User{}, "", time.Time{}, fmt.Errorf("verify registration proof: %w", err)
	}
	now := s.now()
	if err := s.repository.ConsumeChallenge(ctx, challenge.ID, publicKey, PurposeRegister, now); err != nil {
		return User{}, "", time.Time{}, challengeError(err)
	}
	accountID, err := uuid.NewRandom()
	if err != nil {
		return User{}, "", time.Time{}, fmt.Errorf("generate account ID: %w", err)
	}
	fingerprint, err := qr.KeyFingerprint(publicKey)
	if err != nil {
		return User{}, "", time.Time{}, err
	}
	payload, err := qr.Encode(accountID.String(), publicKey)
	if err != nil {
		return User{}, "", time.Time{}, err
	}
	user := User{
		AccountID:      accountID,
		PublicKey:      append([]byte(nil), publicKey...),
		KeyFingerprint: fingerprint,
		CreatedAt:      now,
		QRPayload:      payload,
	}
	token, expiresAt, err := s.issueToken(user.AccountID, now)
	if err != nil {
		return User{}, "", time.Time{}, err
	}
	if err := s.repository.CreateUser(ctx, user); err != nil {
		return User{}, "", time.Time{}, err
	}
	return user, token, expiresAt, nil
}

// Login verifies a fresh identity-key challenge and returns an access token.
func (s *Service) Login(ctx context.Context, challengeID uuid.UUID, publicKey, signature []byte) (User, string, time.Time, error) {
	challenge, err := s.validChallenge(ctx, challengeID, publicKey, PurposeLogin)
	if err != nil {
		return User{}, "", time.Time{}, challengeError(err)
	}
	if !ed25519.Verify(ed25519.PublicKey(publicKey), ChallengeMessage(challenge), signature) {
		return User{}, "", time.Time{}, ErrInvalidCredential
	}
	now := s.now()
	if err := s.repository.ConsumeChallenge(ctx, challenge.ID, publicKey, PurposeLogin, now); err != nil {
		return User{}, "", time.Time{}, challengeError(err)
	}
	user, err := s.repository.GetUserByPublicKey(ctx, publicKey)
	if errors.Is(err, ErrNotFound) {
		return User{}, "", time.Time{}, ErrInvalidCredential
	}
	if err != nil {
		return User{}, "", time.Time{}, fmt.Errorf("load account: %w", err)
	}
	token, expiresAt, err := s.issueToken(user.AccountID, now)
	if err != nil {
		return User{}, "", time.Time{}, err
	}
	return user, token, expiresAt, nil
}

func (s *Service) validChallenge(ctx context.Context, id uuid.UUID, publicKey []byte, purpose ChallengePurpose) (Challenge, error) {
	if len(publicKey) != ed25519.PublicKeySize {
		return Challenge{}, ErrChallengeInvalid
	}
	challenge, err := s.repository.GetChallenge(ctx, id)
	if err != nil {
		return Challenge{}, err
	}
	if challenge.Purpose != purpose || !equalBytes(challenge.PublicKey, publicKey) || !challenge.ExpiresAt.After(s.now()) {
		return Challenge{}, ErrChallengeInvalid
	}
	return challenge, nil
}

func (s *Service) issueToken(accountID uuid.UUID, now time.Time) (string, time.Time, error) {
	expiresAt := now.Add(s.tokenTTL)
	tokenID, err := uuid.NewRandom()
	if err != nil {
		return "", time.Time{}, fmt.Errorf("generate JWT ID: %w", err)
	}
	claims := jwt.RegisteredClaims{
		Issuer:    s.issuer,
		Subject:   accountID.String(),
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(expiresAt),
		ID:        tokenID.String(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(s.jwtSecret)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("sign access token: %w", err)
	}
	return signed, expiresAt, nil
}

func equalBytes(left, right []byte) bool {
	if len(left) != len(right) {
		return false
	}
	var diff byte
	for index := range left {
		diff |= left[index] ^ right[index]
	}
	return diff == 0
}

func challengeError(err error) error {
	if errors.Is(err, ErrChallengeInvalid) {
		return ErrInvalidCredential
	}
	return fmt.Errorf("validate auth challenge: %w", err)
}
