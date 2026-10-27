package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresRepository implements Repository using PostgreSQL.
type PostgresRepository struct {
	pool *pgxpool.Pool
}

// NewPostgresRepository creates a repository backed by pool.
func NewPostgresRepository(pool *pgxpool.Pool) (*PostgresRepository, error) {
	if pool == nil {
		return nil, errors.New("database pool is required")
	}
	return &PostgresRepository{pool: pool}, nil
}

func (r *PostgresRepository) CreateChallenge(ctx context.Context, challenge Challenge) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO auth_challenges (challenge_id, public_key, purpose, nonce, expires_at)
		VALUES ($1, $2, $3, $4, $5)`,
		challenge.ID, challenge.PublicKey, string(challenge.Purpose), challenge.Nonce, challenge.ExpiresAt)
	return err
}

func (r *PostgresRepository) GetChallenge(ctx context.Context, id uuid.UUID) (Challenge, error) {
	var challenge Challenge
	var purpose string
	err := r.pool.QueryRow(ctx, `
		SELECT challenge_id, public_key, purpose, nonce, expires_at
		FROM auth_challenges
		WHERE challenge_id = $1 AND consumed_at IS NULL AND expires_at > now()`, id,
	).Scan(&challenge.ID, &challenge.PublicKey, &purpose, &challenge.Nonce, &challenge.ExpiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Challenge{}, ErrChallengeInvalid
	}
	if err != nil {
		return Challenge{}, fmt.Errorf("get auth challenge: %w", err)
	}
	challenge.Purpose = ChallengePurpose(purpose)
	return challenge, nil
}

func (r *PostgresRepository) ConsumeChallenge(ctx context.Context, id uuid.UUID, publicKey []byte, purpose ChallengePurpose, now time.Time) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE auth_challenges
		SET consumed_at = $4
		WHERE challenge_id = $1
		  AND public_key = $2
		  AND purpose = $3
		  AND consumed_at IS NULL
		  AND expires_at > $4`,
		id, publicKey, string(purpose), now)
	if err != nil {
		return fmt.Errorf("consume auth challenge: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return ErrChallengeInvalid
	}
	return nil
}

func (r *PostgresRepository) CreateUser(ctx context.Context, user User) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO users (account_id, public_key, key_fingerprint, created_at, qr_payload, shiny_rock)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		user.AccountID, user.PublicKey, user.KeyFingerprint, user.CreatedAt, user.QRPayload, user.ShinyRock)
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return ErrConflict
	}
	return fmt.Errorf("create user: %w", err)
}

func (r *PostgresRepository) GetUserByPublicKey(ctx context.Context, publicKey []byte) (User, error) {
	return r.getUser(ctx, `WHERE public_key = $1`, publicKey)
}

func (r *PostgresRepository) GetUserByAccountID(ctx context.Context, accountID uuid.UUID) (User, error) {
	return r.getUser(ctx, `WHERE account_id = $1`, accountID)
}

func (r *PostgresRepository) getUser(ctx context.Context, where string, arg any) (User, error) {
	query := `
		SELECT account_id, public_key, key_fingerprint, created_at, qr_payload, shiny_rock
		FROM users ` + where
	var user User
	err := r.pool.QueryRow(ctx, query, arg).Scan(
		&user.AccountID, &user.PublicKey, &user.KeyFingerprint, &user.CreatedAt, &user.QRPayload, &user.ShinyRock)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrNotFound
	}
	if err != nil {
		return User{}, fmt.Errorf("get user: %w", err)
	}
	return user, nil
}
