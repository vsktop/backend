package account

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
)

type RegistrationRequest struct {
	IdentityKey []byte

	DevicePublicKey []byte

	DeviceCertSignature []byte

	Timestamp int64

	PowNonce []byte
	PowHash  []byte
}

type UpdateProfileRequest struct {
	AccountID   string
	ProfileBlob []byte
}

type SuspendRequest struct {
	AccountID string

	Reason string
}

type Service struct {
	repo Repository

	powDifficulty uint8

	clockFn func() time.Time

	maxTimestampSkew time.Duration
}

func NewService(repo Repository, opts ...Option) *Service {
	s := &Service{
		repo:             repo,
		powDifficulty:    20,
		clockFn:          time.Now,
		maxTimestampSkew: 30 * time.Second,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

type Option func(*Service)

func WithPowDifficulty(bits uint8) Option {
	return func(s *Service) { s.powDifficulty = bits }
}

func WithClock(fn func() time.Time) Option {
	return func(s *Service) { s.clockFn = fn }
}

func WithMaxTimestampSkew(d time.Duration) Option {
	return func(s *Service) { s.maxTimestampSkew = d }
}

func (s *Service) Register(ctx context.Context, req RegistrationRequest) (*Account, error) {

	if err := s.validatePoW(req.PowNonce, req.PowHash); err != nil {
		return nil, fmt.Errorf("registration rejected: %w", err)
	}

	if err := s.validateTimestamp(req.Timestamp); err != nil {
		return nil, fmt.Errorf("registration rejected: %w", err)
	}

	if len(req.IdentityKey) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("registration rejected: identity key must be %d bytes, got %d",
			ed25519.PublicKeySize, len(req.IdentityKey))
	}
	if len(req.DevicePublicKey) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("registration rejected: device key must be %d bytes, got %d",
			ed25519.PublicKeySize, len(req.DevicePublicKey))
	}

	if err := s.verifyDeviceCert(req); err != nil {
		return nil, fmt.Errorf("registration rejected: %w", err)
	}

	accountID := deriveAccountID(req.IdentityKey)

	a := &Account{
		AccountID:   accountID,
		IdentityKey: req.IdentityKey,
		CreatedAt:   s.clockFn().UTC(),
	}

	if err := s.repo.Create(ctx, a); err != nil {
		if errors.Is(err, ErrAlreadyExists) {
			return nil, ErrAlreadyExists
		}
		return nil, fmt.Errorf("register: store: %w", err)
	}

	return a, nil
}

func (s *Service) GetByID(ctx context.Context, accountID string) (*Account, error) {
	a, err := s.repo.GetByID(ctx, accountID)
	if err != nil {
		return nil, err
	}

	if a.IsSuspended() {
		return a, ErrSuspended
	}

	return a, nil
}

func (s *Service) GetByIdentityKey(ctx context.Context, identityKey []byte) (*Account, error) {
	if len(identityKey) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("get by identity key: invalid key length %d", len(identityKey))
	}

	a, err := s.repo.GetByIdentityKey(ctx, identityKey)
	if err != nil {
		return nil, err
	}

	if a.IsSuspended() {
		return a, ErrSuspended
	}

	return a, nil
}

func (s *Service) UpdateProfile(ctx context.Context, req UpdateProfileRequest) error {
	if req.AccountID == "" {
		return errors.New("update profile: account_id required")
	}

	const maxBlobSize = 4096
	if len(req.ProfileBlob) > maxBlobSize {
		return fmt.Errorf("update profile: blob too large (%d bytes, max %d)",
			len(req.ProfileBlob), maxBlobSize)
	}

	if err := s.repo.UpdateProfileBlob(ctx, req.AccountID, req.ProfileBlob); err != nil {
		return fmt.Errorf("update profile: %w", err)
	}

	return nil
}

func (s *Service) Suspend(ctx context.Context, req SuspendRequest) error {
	if req.AccountID == "" {
		return errors.New("suspend: account_id required")
	}
	if err := validateSuspensionReason(req.Reason); err != nil {
		return fmt.Errorf("suspend: %w", err)
	}

	err := s.repo.Suspend(ctx, req.AccountID, req.Reason)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return ErrNotFound
		}
		return fmt.Errorf("suspend: store: %w", err)
	}

	return nil
}

func (s *Service) Unsuspend(ctx context.Context, accountID string) error {
	if accountID == "" {
		return errors.New("unsuspend: account_id required")
	}

	if err := s.repo.Unsuspend(ctx, accountID); err != nil {
		if errors.Is(err, ErrNotFound) {
			return ErrNotFound
		}
		return fmt.Errorf("unsuspend: store: %w", err)
	}

	return nil
}

func (s *Service) Delete(ctx context.Context, accountID string) error {
	if accountID == "" {
		return errors.New("delete: account_id required")
	}

	if err := s.repo.Delete(ctx, accountID); err != nil {
		if errors.Is(err, ErrNotFound) {
			return ErrNotFound
		}
		return fmt.Errorf("delete: store: %w", err)
	}

	return nil
}

func (s *Service) Exists(ctx context.Context, accountID string) (bool, error) {
	if accountID == "" {
		return false, errors.New("exists: account_id required")
	}

	exists, err := s.repo.Exists(ctx, accountID)
	if err != nil {
		return false, fmt.Errorf("exists: store: %w", err)
	}

	return exists, nil
}

func (s *Service) IsSuspended(ctx context.Context, accountID string) (bool, error) {
	if accountID == "" {
		return false, errors.New("is_suspended: account_id required")
	}

	suspended, err := s.repo.IsSuspended(ctx, accountID)
	if err != nil {
		return false, fmt.Errorf("is_suspended: store: %w", err)
	}

	return suspended, nil
}

func deriveAccountID(identityKey []byte) string {
	hash := sha256.Sum256(identityKey)
	return hex.EncodeToString(hash[:])
}

func (s *Service) verifyDeviceCert(req RegistrationRequest) error {
	payload := buildDeviceCertPayload(req.DevicePublicKey, req.Timestamp)

	if !ed25519.Verify(
		ed25519.PublicKey(req.IdentityKey),
		payload,
		req.DeviceCertSignature,
	) {
		return errors.New("device certificate signature invalid")
	}

	return nil
}

func buildDeviceCertPayload(devicePubKey []byte, timestamp int64) []byte {
	payload := make([]byte, len(devicePubKey)+8)
	copy(payload, devicePubKey)
	payload[len(devicePubKey)+0] = byte(timestamp >> 56)
	payload[len(devicePubKey)+1] = byte(timestamp >> 48)
	payload[len(devicePubKey)+2] = byte(timestamp >> 40)
	payload[len(devicePubKey)+3] = byte(timestamp >> 32)
	payload[len(devicePubKey)+4] = byte(timestamp >> 24)
	payload[len(devicePubKey)+5] = byte(timestamp >> 16)
	payload[len(devicePubKey)+6] = byte(timestamp >> 8)
	payload[len(devicePubKey)+7] = byte(timestamp)
	return payload
}

func (s *Service) validateTimestamp(ts int64) error {
	now := s.clockFn().Unix()
	diff := now - ts
	if diff < 0 {
		diff = -diff
	}
	skew := time.Duration(diff) * time.Second
	if skew > s.maxTimestampSkew {
		return fmt.Errorf("timestamp skew %s exceeds maximum %s", skew, s.maxTimestampSkew)
	}
	return nil
}

func (s *Service) validatePoW(nonce, challengeHash []byte) error {
	if len(nonce) == 0 {
		return errors.New("proof-of-work nonce missing")
	}
	if len(challengeHash) != sha256.Size {
		return fmt.Errorf("proof-of-work challenge hash must be %d bytes", sha256.Size)
	}

	input := make([]byte, len(nonce)+len(challengeHash))
	copy(input, nonce)
	copy(input[len(nonce):], challengeHash)
	result := sha256.Sum256(input)

	if !hasLeadingZeroBits(result[:], s.powDifficulty) {
		return fmt.Errorf("proof-of-work solution invalid (difficulty %d)", s.powDifficulty)
	}

	return nil
}

func hasLeadingZeroBits(b []byte, bits uint8) bool {
	fullBytes := bits / 8
	remainder := bits % 8

	for i := uint8(0); i < fullBytes; i++ {
		if i >= uint8(len(b)) || b[i] != 0 {
			return false
		}
	}

	if remainder == 0 {
		return true
	}

	if int(fullBytes) >= len(b) {
		return false
	}

	mask := byte(0xFF) << (8 - remainder)
	return b[fullBytes]&mask == 0
}

func validateSuspensionReason(reason string) error {
	valid := map[string]bool{
		"spam":        true,
		"csam":        true,
		"harassment":  true,
		"ban_evasion": true,
		"other":       true,
	}
	if !valid[reason] {
		return fmt.Errorf("unknown suspension reason %q", reason)
	}
	return nil
}
