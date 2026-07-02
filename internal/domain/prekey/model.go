package prekey

import "errors"

var (
	ErrNotFound      = errors.New("prekey: not found")
	ErrOTKExhausted  = errors.New("prekey: one-time prekey pool exhausted")
	ErrInvalidSig    = errors.New("prekey: signature invalid")
	ErrInvalidKeyLen = errors.New("prekey: invalid key length")
)

type SignedPreKey struct {
	DeviceID string

	KeyID uint32

	PublicKey []byte

	Signature []byte
}

type OneTimePreKey struct {
	DeviceID string

	KeyID uint32

	PublicKey []byte

	Signature []byte
}

type Bundle struct {
	DeviceID string
	SPK      *SignedPreKey
	OTK      *OneTimePreKey
}
