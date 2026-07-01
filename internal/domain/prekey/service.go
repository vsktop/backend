package prekey

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"time"
)

type Repository interface {
	Create(ctx context.Context, pk *PreKey) error
	GetByID(ctx context.Context, id uint64) (*PreKey, error)
	GetByDeviceID(ctx context.Context, deviceID string, consumed bool) ([]*PreKey, error)
	GetSignedPrekey(ctx context.Context, deviceID string) (*PreKey, error)
	MarkConsumed(ctx context.Context, id uint64) error
	Revoke(ctx context.Context, id uint64) error
	RevokeAllForDevice(ctx context.Context, deviceID string) error
	ExpireOld(ctx context.Context, before time.Time) (int64, error)
}

type Service struct {
	repo      Repository
	clockFn   func() time.Time
	expiry    time.Duration
	batchSize int
}

func NewService(repo Repository, opts ...Option) *Service {
	s := &Service{
		repo:      repo,
		clockFn:   time.Now,
		expiry:    30 * 24 * time.Hour,
		batchSize: 100,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

type Option func(*Service)

func WithExpiry(d time.Duration) Option {
	return func(s *Service) { s.expiry = d }
}

func WithBatchSize(n int) Option {
	return func(s *Service) { s.batchSize = n }
}

func WithClock(fn func() time.Time) Option {
	return func(s *Service) { s.clockFn = fn }
}

func (s *Service) GenerateOneTimePrekeys(ctx context.Context, deviceID string) ([]PreKey, error) {
	now := s.clockFn().UTC()
	expiresAt := now.Add(s.expiry)

	prekeys := make([]PreKey, s.batchSize)
	for i := 0; i < s.batchSize; i++ {
		pub, priv, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return nil, errors.New("prekey: failed to generate key pair")
		}
		prekeys[i] = PreKey{
			DeviceID:   deviceID,
			PublicKey:  pub,
			PrivateKey: priv,
			Type:       PreKeyOneTime,
			Consumed:   false,
			CreatedAt:  now,
			ExpiresAt:  expiresAt,
		}
	}
	return prekeys, nil
}

func (s *Service) GenerateSignedPrekey(ctx context.Context, deviceID string) (*PreKey, error) {
	now := s.clockFn().UTC()
	expiresAt := now.Add(s.expiry)

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, errors.New("prekey: failed to generate signed prekey")
	}

	return &PreKey{
		DeviceID:   deviceID,
		PublicKey:  pub,
		PrivateKey: priv,
		Type:       PreKeySigned,
		Consumed:   false,
		CreatedAt:  now,
		ExpiresAt:  expiresAt,
	}, nil
}

func (s *Service) UploadPrekeyBundle(ctx context.Context, deviceID string, signedPrekey *PreKey, oneTimePrekeys []PreKey) error {
	if err := signedPrekey.Validate(); err != nil {
		return err
	}

	if err := s.repo.RevokeAllForDevice(ctx, deviceID); err != nil && !errors.Is(err, ErrNotFound) {
		return err
	}

	signedPrekey.Consumed = false
	if err := s.repo.Create(ctx, signedPrekey); err != nil {
		return err
	}

	for i := range oneTimePrekeys {
		oneTimePrekeys[i].DeviceID = deviceID
		oneTimePrekeys[i].Consumed = false
		if err := s.repo.Create(ctx, &oneTimePrekeys[i]); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) FetchPrekeyBundle(ctx context.Context, deviceID string) (*PreKey, []PreKey, error) {
	signedPrekey, err := s.repo.GetSignedPrekey(ctx, deviceID)
	if err != nil {
		return nil, nil, err
	}

	oneTimePrekeys, err := s.repo.GetByDeviceID(ctx, deviceID, false)
	if err != nil {
		return nil, nil, err
	}

	var available []PreKey
	now := s.clockFn().UTC()
	for _, pk := range oneTimePrekeys {
		if !pk.Consumed && pk.ExpiresAt.After(now) {
			available = append(available, *pk)
		}
	}

	return signedPrekey, available, nil
}

func (s *Service) ConsumePrekey(ctx context.Context, prekeyID uint64) error {
	pk, err := s.repo.GetByID(ctx, prekeyID)
	if err != nil {
		return err
	}
	if pk.Type != PreKeyOneTime {
		return errors.New("prekey: cannot consume a non-one-time prekey")
	}
	if pk.Consumed {
		return errors.New("prekey: prekey already consumed")
	}
	return s.repo.MarkConsumed(ctx, prekeyID)
}

func ExportIdentityKeyPEM(key ed25519.PublicKey) ([]byte, error) {
	block := &pem.Block{
		Type:  "ED25519 PUBLIC KEY",
		Bytes: key,
	}
	return pem.EncodeToMemory(block), nil
}

func ParseIdentityKeyPEM(data []byte) (ed25519.PublicKey, error) {
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, errors.New("prekey: failed to parse PEM block")
	}
	if block.Type != "ED25519 PUBLIC KEY" {
		return nil, errors.New("prekey: unsupported PEM type")
	}
	if len(block.Bytes) != ed25519.PublicKeySize {
		return nil, ErrInvalidKeyLen
	}
	return ed25519.PublicKey(block.Bytes), nil
}
