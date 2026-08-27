// Package connectorvault stores encrypted connector credentials without
// exposing plaintext values to API responses, logs, or audit records.
package connectorvault

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"database/sql"
	"fmt"
	"io"

	"github.com/pkg/errors"
)

type Vault struct {
	db         *sql.DB
	key        []byte
	keyVersion string
}

func New(db *sql.DB, key []byte, keyVersion string) (*Vault, error) {
	if db == nil || len(key) != 32 || keyVersion == "" {
		return nil, errors.New("connector vault requires a database, 32-byte key, and key version")
	}
	return &Vault{db: db, key: append([]byte(nil), key...), keyVersion: keyVersion}, nil
}

func (v *Vault) Put(ctx context.Context, organizationID, installationID string, plaintext []byte) error {
	if err := validateIdentity(organizationID, installationID); err != nil {
		return err
	}
	if len(plaintext) == 0 {
		return errors.New("connector credential cannot be empty")
	}
	ciphertext, err := encrypt(v.key, plaintext)
	if err != nil {
		return err
	}
	_, err = v.db.ExecContext(ctx, `
		INSERT INTO a2a888_connector_credential (organization_id,installation_id,ciphertext,key_version)
		VALUES ($1,$2,$3,$4)
		ON CONFLICT (organization_id,installation_id) DO UPDATE SET ciphertext=EXCLUDED.ciphertext,key_version=EXCLUDED.key_version,updated_at=now()
	`, organizationID, installationID, ciphertext, v.keyVersion)
	return errors.Wrap(err, "store connector credential")
}

func (v *Vault) Get(ctx context.Context, organizationID, installationID string) ([]byte, error) {
	if err := validateIdentity(organizationID, installationID); err != nil {
		return nil, err
	}
	var ciphertext []byte
	var keyVersion string
	if err := v.db.QueryRowContext(ctx, `SELECT ciphertext,key_version FROM a2a888_connector_credential WHERE organization_id=$1 AND installation_id=$2`, organizationID, installationID).Scan(&ciphertext, &keyVersion); err != nil {
		return nil, errors.Wrap(err, "read connector credential")
	}
	if keyVersion != v.keyVersion {
		return nil, errors.Errorf("connector credential key version %q requires rotation", keyVersion)
	}
	return decrypt(v.key, ciphertext)
}

func (v *Vault) Rotate(ctx context.Context, organizationID, installationID string, plaintext []byte) error {
	return v.Put(ctx, organizationID, installationID, plaintext)
}

func validateIdentity(organizationID, installationID string) error {
	if organizationID == "" || installationID == "" {
		return errors.New("connector credential organization and installation are required")
	}
	return nil
}

func encrypt(key, plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, errors.Wrap(err, "create connector credential cipher")
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, errors.Wrap(err, "create connector credential AEAD")
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, errors.Wrap(err, "generate connector credential nonce")
	}
	return gcm.Seal(nonce, nonce, plaintext, nil), nil
}

func decrypt(key, ciphertext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, errors.Wrap(err, "create connector credential cipher")
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, errors.Wrap(err, "create connector credential AEAD")
	}
	if len(ciphertext) < gcm.NonceSize() {
		return nil, fmt.Errorf("connector credential ciphertext is truncated")
	}
	nonce, payload := ciphertext[:gcm.NonceSize()], ciphertext[gcm.NonceSize():]
	plaintext, err := gcm.Open(nil, nonce, payload, nil)
	if err != nil {
		return nil, errors.Wrap(err, "decrypt connector credential")
	}
	return plaintext, nil
}
