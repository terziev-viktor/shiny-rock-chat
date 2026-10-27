package qr

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/uuid"
)

const (
	scheme       = "shinyrock://v1/"
	fingerprintN = sha256.Size * 2
	checksumN    = 16
)

var fingerprintPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

// Identity is the account identity encoded by a QR payload.
type Identity struct {
	AccountID      string
	KeyFingerprint string
}

// KeyFingerprint returns the lowercase hexadecimal SHA-256 fingerprint of an Ed25519 public key.
func KeyFingerprint(publicKey []byte) (string, error) {
	if len(publicKey) != 32 {
		return "", errors.New("Ed25519 public key must be 32 bytes")
	}
	sum := sha256.Sum256(publicKey)
	return hex.EncodeToString(sum[:]), nil
}

// Encode produces the stable v1 QR URI for an account and its public key.
func Encode(accountID string, publicKey []byte) (string, error) {
	id, err := uuid.Parse(accountID)
	if err != nil {
		return "", fmt.Errorf("invalid account ID: %w", err)
	}
	fingerprint, err := KeyFingerprint(publicKey)
	if err != nil {
		return "", err
	}
	canonicalID := id.String()
	return scheme + canonicalID + "." + fingerprint + "." + checksum(canonicalID, fingerprint), nil
}

// Verify validates the payload structure and checksum and returns its identity.
// The checksum detects accidental corruption; it is not a server signature.
func Verify(payload string) (Identity, error) {
	if !strings.HasPrefix(payload, scheme) {
		return Identity{}, errors.New("unsupported QR payload scheme")
	}
	parts := strings.Split(strings.TrimPrefix(payload, scheme), ".")
	if len(parts) != 3 {
		return Identity{}, errors.New("invalid QR payload")
	}
	id, err := uuid.Parse(parts[0])
	if err != nil || id.String() != parts[0] {
		return Identity{}, errors.New("invalid QR account ID")
	}
	if !fingerprintPattern.MatchString(parts[1]) {
		return Identity{}, errors.New("invalid QR key fingerprint")
	}
	expected := checksum(parts[0], parts[1])
	if len(parts[2]) != checksumN || parts[2] != expected {
		return Identity{}, errors.New("invalid QR checksum")
	}
	return Identity{AccountID: parts[0], KeyFingerprint: parts[1]}, nil
}

func checksum(accountID, fingerprint string) string {
	sum := sha256.Sum256([]byte("shiny-rock-qr-v1\x00" + accountID + "\x00" + fingerprint))
	return hex.EncodeToString(sum[:checksumN/2])
}
