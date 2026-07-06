package prekey

import (
	"crypto/ed25519"
	"errors"
	"fmt"
)

var (
	ErrNotFound      = errors.New("prekey: not found")
	ErrOTKExhausted  = errors.New("prekey: one-time prekey pool exhausted")
	ErrInvalidSig    = errors.New("prekey: signature invalid")
	ErrInvalidKeyLen = errors.New("prekey: invalid key length")
)

type SignedPreKey struct {
	DeviceID string `json:"-"` // set server-side; not accepted from clients

	KeyID     uint32 `json:"key_id"`
	PublicKey []byte `json:"public_key"` // base64 X25519 pub, 32 bytes
	Signature []byte `json:"signature"`  // base64 Ed25519 sig by device key
}

type OneTimePreKey struct {
	DeviceID string `json:"-"` // set server-side; not accepted from clients

	KeyID     uint32 `json:"key_id"`
	PublicKey []byte `json:"public_key"` // base64 X25519 pub, 32 bytes
	Signature []byte `json:"signature"`  // base64 Ed25519 sig by device key
}

type Bundle struct {
	DeviceID string
	SPK      *SignedPreKey
	OTK      *OneTimePreKey
}

func (spk *SignedPreKey) Validate() error {
	if spk.DeviceID == "" {
		return errors.New("prekey: device_id required")
	}
	if len(spk.PublicKey) != 32 {
		return fmt.Errorf("%w: SPK public key must be 32 bytes, got %d", ErrInvalidKeyLen, len(spk.PublicKey))
	}
	if len(spk.Signature) != ed25519.SignatureSize {
		return fmt.Errorf("SPK signature must be %d bytes, got %d", ed25519.SignatureSize, len(spk.Signature))
	}
	return nil
}

func (otk *OneTimePreKey) Validate() error {
	if len(otk.PublicKey) != 32 {
		return fmt.Errorf("%w: OTK public key must be 32 bytes, got %d", ErrInvalidKeyLen, len(otk.PublicKey))
	}
	if len(otk.Signature) != ed25519.SignatureSize {
		return fmt.Errorf("OTK signature must be %d bytes, got %d", ed25519.SignatureSize, len(otk.Signature))
	}
	return nil
}
