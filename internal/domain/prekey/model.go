package prekey

import "errors"

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
