package connectorvault

import (
	"bytes"
	"testing"
)

func TestCredentialEncryptionRoundTripAndTamperRejection(t *testing.T) {
	key := bytes.Repeat([]byte{7}, 32)
	ciphertext, err := encrypt(key, []byte("credential-value"), []byte("org-a\x00install-a"))
	if err != nil {
		t.Fatal(err)
	}
	plaintext, err := decrypt(key, ciphertext, []byte("org-a\x00install-a"))
	if err != nil || string(plaintext) != "credential-value" {
		t.Fatalf("round trip failed: %q, %v", plaintext, err)
	}
	ciphertext[len(ciphertext)-1] ^= 1
	if _, err := decrypt(key, ciphertext, []byte("org-a\x00install-a")); err == nil {
		t.Fatal("tampered credential decrypted successfully")
	}
	if _, err := decrypt(key, ciphertext, []byte("org-b\x00install-b")); err == nil {
		t.Fatal("credential moved across tenants decrypted successfully")
	}
}

func TestNewRejectsInvalidKey(t *testing.T) {
	if _, err := New(nil, []byte("short"), "v1"); err == nil {
		t.Fatal("invalid vault configuration was accepted")
	}
}

func TestRevokeRequiresTenantScopedIdentity(t *testing.T) {
	vault := &Vault{}
	if err := vault.Revoke(nil, "", "install-a"); err == nil {
		t.Fatal("credential revocation without tenant was accepted")
	}
	if err := vault.Revoke(nil, "org-a", ""); err == nil {
		t.Fatal("credential revocation without installation was accepted")
	}
}
