package prekey

import (
	"context"
	"crypto/ed25519"
	"encoding/binary"
	"errors"
	"fmt"
)

type Repository interface {
	UpsertSignedPreKey(ctx context.Context, spk *SignedPreKey) error
	GetSignedPreKey(ctx context.Context, deviceID string) (*SignedPreKey, error)

	UploadOneTimePreKeys(ctx context.Context, deviceID string, keys []OneTimePreKey) error
	ConsumeOneTimePreKey(ctx context.Context, deviceID string) (*OneTimePreKey, error)
	CountOneTimePreKeys(ctx context.Context, deviceID string) (int64, error)

	FetchBundle(ctx context.Context, deviceID string) (*Bundle, error)

	DeleteAllForDevice(ctx context.Context, deviceID string) error
}

const LowWatermark = 20

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) UploadSignedPreKey(ctx context.Context, deviceIdentityKey ed25519.PublicKey, spk *SignedPreKey) error {
	if err := spk.Validate(); err != nil {
		return fmt.Errorf("upload signed prekey: %w", err)
	}

	if err := verifySPKSignature(deviceIdentityKey, spk); err != nil {
		return fmt.Errorf("upload signed prekey: %w", err)
	}

	if err := s.repo.UpsertSignedPreKey(ctx, spk); err != nil {
		return fmt.Errorf("upload signed prekey: store: %w", err)
	}

	return nil
}

func (s *Service) UploadOneTimePreKeys(ctx context.Context, deviceIdentityKey ed25519.PublicKey, deviceID string, keys []OneTimePreKey) error {
	if len(keys) == 0 {
		return errors.New("upload one-time prekeys: batch is empty")
	}
	if len(keys) > 100 {
		return fmt.Errorf("upload one-time prekeys: batch too large (%d, max 100)", len(keys))
	}

	for i, k := range keys {
		if err := k.Validate(); err != nil {
			return fmt.Errorf("upload one-time prekeys: key[%d]: %w", i, err)
		}
		if err := verifyOTKSignature(deviceIdentityKey, &k); err != nil {
			return fmt.Errorf("upload one-time prekeys: key[%d]: %w", i, err)
		}
	}

	for i := range keys {
		keys[i].DeviceID = deviceID
	}

	if err := s.repo.UploadOneTimePreKeys(ctx, deviceID, keys); err != nil {
		return fmt.Errorf("upload one-time prekeys: store: %w", err)
	}

	return nil
}

func (s *Service) FetchBundle(ctx context.Context, deviceID string) (*Bundle, bool, error) {
	bundle, err := s.repo.FetchBundle(ctx, deviceID)
	if err != nil {
		return nil, false, fmt.Errorf("fetch bundle: %w", err)
	}

	count, err := s.repo.CountOneTimePreKeys(ctx, deviceID)
	if err != nil {
		return bundle, false, nil
	}

	needsRefill := count < LowWatermark
	return bundle, needsRefill, nil
}

func (s *Service) OTKCount(ctx context.Context, deviceID string) (int64, error) {
	count, err := s.repo.CountOneTimePreKeys(ctx, deviceID)
	if err != nil {
		return 0, fmt.Errorf("otk count: %w", err)
	}
	return count, nil
}

func (s *Service) NeedsRefill(ctx context.Context, deviceID string) (bool, error) {
	count, err := s.repo.CountOneTimePreKeys(ctx, deviceID)
	if err != nil {
		return false, fmt.Errorf("needs refill: %w", err)
	}
	return count < LowWatermark, nil
}

func (s *Service) DeleteAllForDevice(ctx context.Context, deviceID string) error {
	if err := s.repo.DeleteAllForDevice(ctx, deviceID); err != nil {
		return fmt.Errorf("delete all for device: %w", err)
	}
	return nil
}

func verifySPKSignature(deviceIdentityKey ed25519.PublicKey, spk *SignedPreKey) error {
	payload := buildPreKeyPayload(spk.KeyID, spk.PublicKey)
	if !ed25519.Verify(deviceIdentityKey, payload, spk.Signature) {
		return ErrInvalidSig
	}
	return nil
}

func verifyOTKSignature(deviceIdentityKey ed25519.PublicKey, otk *OneTimePreKey) error {
	payload := buildPreKeyPayload(otk.KeyID, otk.PublicKey)
	if !ed25519.Verify(deviceIdentityKey, payload, otk.Signature) {
		return ErrInvalidSig
	}
	return nil
}

func buildPreKeyPayload(keyID uint32, publicKey []byte) []byte {
	payload := make([]byte, 4+len(publicKey))
	binary.BigEndian.PutUint32(payload[:4], keyID)
	copy(payload[4:], publicKey)
	return payload
}
