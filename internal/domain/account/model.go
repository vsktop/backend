package account

import (
	"context"
	"errors"
	"time"
)

var (
	ErrNotFound      = errors.New("account: not found")
	ErrAlreadyExists = errors.New("account: already exists")
	ErrSuspended     = errors.New("account: suspended")
)

type Account struct {
	AccountID string

	IdentityKey []byte

	ProfileBlob []byte

	CreatedAt        time.Time
	SuspendedAt      *time.Time
	SuspensionReason *string
}

func (a *Account) IsActive() bool {
	return a.SuspendedAt == nil
}

func (a *Account) IsSuspended() bool {
	return a.SuspendedAt != nil
}

type Repository interface {
	Create(ctx context.Context, a *Account) error
	GetByID(ctx context.Context, accountID string) (*Account, error)
	GetByIdentityKey(ctx context.Context, identityKey []byte) (*Account, error)
	UpdateProfileBlob(ctx context.Context, accountID string, blob []byte) error
	Suspend(ctx context.Context, accountID string, reason string) error
	Unsuspend(ctx context.Context, accountID string) error
	IsSuspended(ctx context.Context, accountID string) (bool, error)
	Delete(ctx context.Context, accountID string) error
	Exists(ctx context.Context, accountID string) (bool, error)
}
