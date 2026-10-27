package qr

import (
	"crypto/ed25519"
	"crypto/rand"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestEncodeAndVerify(t *testing.T) {
	publicKey, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	accountID := uuid.NewString()

	payload, err := Encode(accountID, publicKey)
	if err != nil {
		t.Fatal(err)
	}
	identity, err := Verify(payload)
	if err != nil {
		t.Fatal(err)
	}
	fingerprint, err := KeyFingerprint(publicKey)
	if err != nil {
		t.Fatal(err)
	}
	if identity.AccountID != accountID || identity.KeyFingerprint != fingerprint {
		t.Fatalf("unexpected identity: %+v", identity)
	}
}

func TestVerifyRejectsMalformedOrModifiedPayload(t *testing.T) {
	publicKey, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := Encode(uuid.NewString(), publicKey)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name    string
		payload string
	}{
		{name: "unknown scheme", payload: strings.Replace(payload, "shinyrock://v1/", "https://", 1)},
		{name: "modified fingerprint", payload: payload[:len(payload)-17] + "0" + payload[len(payload)-16:]},
		{name: "bad checksum", payload: payload[:len(payload)-1] + "0"},
		{name: "extra component", payload: payload + ".extra"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := Verify(test.payload); err == nil {
				t.Fatal("Verify() accepted invalid payload")
			}
		})
	}
}

func TestKeyFingerprintRejectsInvalidPublicKeyLength(t *testing.T) {
	if _, err := KeyFingerprint([]byte("too short")); err == nil {
		t.Fatal("KeyFingerprint() accepted an invalid public key")
	}
}
