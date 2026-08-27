package connectorvault

import (
	"bytes"
	"testing"
)

func TestCredentialEncryptionRoundTripAndTamperRejection(t *testing.T) {
	key := bytes.Repeat([]byte{7}, 32)
	ciphertext, err := encrypt(key, []byte("credential-value"))
	if err != nil {
		t.Fatal(err)
	}
	plaintext, err := decrypt(key, ciphertext)
	if err != nil || string(plaintext) != "credential-value" {
		t.Fatalf("round trip failed: %q, %v", plaintext, err)
	}
	ciphertext[len(ciphertext)-1] ^= 1
	if _, err := decrypt(key, ciphertext); err == nil {
		t.Fatal("tampered credential decrypted successfully")
	}
}

func TestNewRejectsInvalidKey(t *testing.T) {
	if _, err := New(nil, []byte("short"), "v1"); err == nil {
		t.Fatal("invalid vault configuration was accepted")
	}
}
