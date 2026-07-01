package device

import (
	"context"
)

type Repository interface {
	Create(ctx context.Context, d *Device) error
	GetByID(ctx context.Context, deviceID string) (*Device, error)
	ListByAccount(ctx context.Context, accountID string) ([]*Device, error)
	CountByAccount(ctx context.Context, accountID string) (int64, error)
	Revoke(ctx context.Context, deviceID string) error
	RevokeAllForAccount(ctx context.Context, accountID, exemptDeviceID string) error
	UpdateLastSeen(ctx context.Context, deviceID string) error
	UpdatePushToken(ctx context.Context, deviceID string, tokenType PushTokenType, token []byte) error
}

type Service struct {
	repo   Repository
	maxDev int64
}

func NewService(repo Repository, maxDev int64) *Service {
	if maxDev == 0 {
		maxDev = 10
	}
	return &Service{repo: repo, maxDev: maxDev}
}

func (s *Service) Register(ctx context.Context, d *Device) error {
	if err := d.Validate(); err != nil {
		return err
	}

	count, err := s.repo.CountByAccount(ctx, d.AccountID)
	if err != nil {
		return err
	}
	if count >= s.maxDev {
		return ErrLimitExceeded
	}

	return s.repo.Create(ctx, d)
}

func (s *Service) GetByID(ctx context.Context, deviceID string) (*Device, error) {
	return s.repo.GetByID(ctx, deviceID)
}

func (s *Service) ListByAccount(ctx context.Context, accountID string) ([]*Device, error) {
	return s.repo.ListByAccount(ctx, accountID)
}

func (s *Service) Revoke(ctx context.Context, deviceID string) error {
	return s.repo.Revoke(ctx, deviceID)
}

func (s *Service) RevokeAllForAccount(ctx context.Context, accountID, exemptDeviceID string) error {
	return s.repo.RevokeAllForAccount(ctx, accountID, exemptDeviceID)
}

func (s *Service) UpdateLastSeen(ctx context.Context, deviceID string) error {
	return s.repo.UpdateLastSeen(ctx, deviceID)
}

func (s *Service) UpdatePushToken(ctx context.Context, deviceID string, tokenType PushTokenType, token []byte) error {
	return s.repo.UpdatePushToken(ctx, deviceID, tokenType, token)
}
