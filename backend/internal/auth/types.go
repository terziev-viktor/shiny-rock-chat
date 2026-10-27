package auth

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrNotFound          = errors.New("record not found")
	ErrConflict          = errors.New("record already exists")
	ErrChallengeInvalid  = errors.New("challenge is invalid or expired")
	ErrInvalidChallenge  = errors.New("invalid auth challenge request")
	ErrInvalidProof      = errors.New("registration proof is invalid")
	ErrInvalidCredential = errors.New("invalid credentials")
)

// ChallengePurpose distinguishes account creation from login challenges.
type ChallengePurpose string

const (
	PurposeRegister ChallengePurpose = "register"
	PurposeLogin    ChallengePurpose = "login"
)

// User contains only server-side public identity and account fields.
type User struct {
	AccountID      uuid.UUID
	PublicKey      []byte
	KeyFingerprint string
	CreatedAt      time.Time
	QRPayload      string
	ShinyRock      int64
}

type Challenge struct {
	ID        uuid.UUID
	PublicKey []byte
	Purpose   ChallengePurpose
	Nonce     []byte
	ExpiresAt time.Time
}

// Repository is the persistence boundary required by the auth service.
type Repository interface {
	CreateChallenge(context.Context, Challenge) error
	GetChallenge(context.Context, uuid.UUID) (Challenge, error)
	ConsumeChallenge(context.Context, uuid.UUID, []byte, ChallengePurpose, time.Time) error
	CreateUser(context.Context, User) error
	GetUserByPublicKey(context.Context, []byte) (User, error)
	GetUserByAccountID(context.Context, uuid.UUID) (User, error)
}

// ProofVerifier verifies registration proof-of-work bound to a challenge.
type ProofVerifier interface {
	Verify(context.Context, []byte, []byte, string) error
}
