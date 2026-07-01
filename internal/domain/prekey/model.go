package prekey

import (
	"errors"
	"time"
)

var (
	ErrNotFound      = errors.New("prekey: not found")
	ErrAlreadyExists = errors.New("prekey: already exists")
	ErrInvalidKeyLen = errors.New("prekey: invalid key length")
)

type PreKeyType int16

const (
	PreKeyOneTime PreKeyType = iota
	PreKeySigned
)

type PreKey struct {
	ID         uint64
	DeviceID   string
	PublicKey  []byte
	PrivateKey []byte
	Type       PreKeyType
	Consumed   bool
	CreatedAt  time.Time
	ExpiresAt  time.Time
}

func (pk *PreKey) Validate() error {
	if len(pk.PublicKey) != 32 {
		return ErrInvalidKeyLen
	}
	if len(pk.PrivateKey) != 32 {
		return ErrInvalidKeyLen
	}
	if pk.DeviceID == "" {
		return errors.New("prekey: device_id is required")
	}
	if pk.Type != PreKeyOneTime && pk.Type != PreKeySigned {
		return errors.New("prekey: invalid type")
	}
	return nil
}
