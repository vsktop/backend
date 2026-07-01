package device

import (
	"errors"
	"time"
)

var (
	ErrNotFound      = errors.New("device: not found")
	ErrAlreadyExists = errors.New("device: already exists")
	ErrLimitExceeded = errors.New("device: limit exceeded")
	ErrInvalidCert   = errors.New("device: invalid device certificate")
)

type PushTokenType int16

const (
	PushTokenNone PushTokenType = iota
	PushTokenAPNs
	PushTokenFCM
	PushTokenWebPush
)

type Device struct {
	DeviceID string

	AccountID string

	DeviceCert []byte

	LabelBlob []byte

	PushToken     []byte
	PushTokenType PushTokenType

	CreatedAt time.Time

	LastSeenAt time.Time

	RevokedAt *time.Time
}

func (d *Device) Validate() error {
	if d.DeviceID == "" {
		return errors.New("device: device_id is required")
	}
	if len(d.DeviceID) != 32 {
		return errors.New("device: device_id must be 32 hex chars")
	}
	if d.AccountID == "" {
		return errors.New("device: account_id is required")
	}
	if len(d.DeviceCert) != 96 {
		return ErrInvalidCert
	}
	if d.RevokedAt != nil && !d.RevokedAt.IsZero() {
		return errors.New("device: cannot create a revoked device")
	}
	return nil
}
