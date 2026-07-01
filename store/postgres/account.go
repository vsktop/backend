package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/vsktop/backend/internal/domain/account"
	"gorm.io/gorm"
)

// AccountModel is the GORM persistence model for accounts.
// It is intentionally separate from domain.Account — never expose
// GORM tags or DB concerns to the domain layer.
type AccountModel struct {
	// account_id = SHA-256(IK_A_public)[0:32], stored as hex string.
	// Never auto-increment — the ID is derived client-side from the
	// identity key so the server cannot assign or reassign identities.
	AccountID string `gorm:"primaryKey;type:char(64);column:account_id"`

	// IK_A — Ed25519 public key, 32 raw bytes stored as bytea.
	// The server only ever sees the public key. Never the private key.
	IdentityKey []byte `gorm:"not null;type:bytea;column:identity_key"`

	// Encrypted profile blob: display name, avatar hash, etc.
	// Encrypted client-side with a key derived from IK_A.
	// The server cannot read this — it is opaque bytes used only
	// for delivery to contacts.
	ProfileBlob []byte `gorm:"type:bytea;column:profile_blob"`

	// Stored at second granularity only. Never finer.
	CreatedAt time.Time `gorm:"not null;autoCreateTime;column:created_at"`

	// NULL means active. Populated only on suspension.
	SuspendedAt *time.Time `gorm:"column:suspended_at"`

	// Short internal code: "spam", "csam", "harassment", etc.
	// Only populated alongside SuspendedAt. Never user-facing.
	SuspensionReason *string `gorm:"type:varchar(64);column:suspension_reason"`

	// Soft-delete timestamp. GORM's DeletedAt enables soft deletes.
	// We use soft delete so revocation lists remain intact even after
	// a user deletes their account.
	DeletedAt gorm.DeletedAt `gorm:"index;column:deleted_at"`
}

func (AccountModel) TableName() string {
	return "accounts"
}

// toDomain maps the GORM model to the pure domain struct.
// Domain code never imports gorm or touches AccountModel directly.
func (m *AccountModel) toDomain() *account.Account {
	return &account.Account{
		AccountID:        m.AccountID,
		IdentityKey:      m.IdentityKey,
		ProfileBlob:      m.ProfileBlob,
		CreatedAt:        m.CreatedAt,
		SuspendedAt:      m.SuspendedAt,
		SuspensionReason: m.SuspensionReason,
	}
}

// fromDomain maps a domain struct back to a GORM model for writes.
func fromDomainAccount(a *account.Account) *AccountModel {
	return &AccountModel{
		AccountID:        a.AccountID,
		IdentityKey:      a.IdentityKey,
		ProfileBlob:      a.ProfileBlob,
		CreatedAt:        a.CreatedAt,
		SuspendedAt:      a.SuspendedAt,
		SuspensionReason: a.SuspensionReason,
	}
}

// AccountRepository implements domain/account.Repository against Postgres.
type AccountRepository struct {
	db *gorm.DB
}

// NewAccountRepository returns a repository backed by db.
func NewAccountRepository(db *gorm.DB) *AccountRepository {
	return &AccountRepository{db: db}
}

// Create inserts a new account.
//
// Returns account.ErrAlreadyExists if the account_id is already taken
// (two devices raced to register with the same identity key, or a replay
// of a prior registration attempt).
func (r *AccountRepository) Create(ctx context.Context, a *account.Account) error {
	model := fromDomainAccount(a)

	result := r.db.WithContext(ctx).Create(model)
	if result.Error != nil {
		if isUniqueViolation(result.Error) {
			return account.ErrAlreadyExists
		}
		return result.Error
	}
	return nil
}

// GetByID fetches an active (non-deleted) account by its account_id.
//
// Returns account.ErrNotFound if no active account matches.
func (r *AccountRepository) GetByID(ctx context.Context, accountID string) (*account.Account, error) {
	var model AccountModel

	result := r.db.WithContext(ctx).
		Where("account_id = ?", accountID).
		First(&model)

	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, account.ErrNotFound
		}
		return nil, result.Error
	}

	return model.toDomain(), nil
}

// GetByIdentityKey fetches an active account by its raw Ed25519 public key bytes.
//
// Used during device-linking flows where the client presents IK_A_public
// and we need to confirm the account exists before accepting a new device cert.
func (r *AccountRepository) GetByIdentityKey(ctx context.Context, identityKey []byte) (*account.Account, error) {
	var model AccountModel

	result := r.db.WithContext(ctx).
		Where("identity_key = ?", identityKey).
		First(&model)

	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, account.ErrNotFound
		}
		return nil, result.Error
	}

	return model.toDomain(), nil
}

// UpdateProfileBlob replaces the encrypted profile blob for an account.
//
// The blob is opaque to the server. This method only validates that the
// account exists and is active before writing — it does not inspect content.
func (r *AccountRepository) UpdateProfileBlob(ctx context.Context, accountID string, blob []byte) error {
	result := r.db.WithContext(ctx).
		Model(&AccountModel{}).
		Where("account_id = ? AND suspended_at IS NULL AND deleted_at IS NULL", accountID).
		Update("profile_blob", blob)

	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return account.ErrNotFound
	}
	return nil
}

// Suspend marks an account as suspended with a reason code.
//
// Suspended accounts can still be fetched (their prekeys remain visible so
// peers can detect the suspension) but cannot authenticate new sessions.
// A suspended account is NOT soft-deleted — deletion is a separate action.
func (r *AccountRepository) Suspend(ctx context.Context, accountID string, reason string) error {
	now := time.Now().UTC()

	result := r.db.WithContext(ctx).
		Model(&AccountModel{}).
		Where("account_id = ? AND suspended_at IS NULL", accountID).
		Updates(map[string]any{
			"suspended_at":      now,
			"suspension_reason": reason,
		})

	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		// Either does not exist, already suspended, or soft-deleted.
		return account.ErrNotFound
	}
	return nil
}

// Unsuspend clears a suspension, reinstating the account.
// Used when an appeal is granted or a ban was issued in error.
func (r *AccountRepository) Unsuspend(ctx context.Context, accountID string) error {
	result := r.db.WithContext(ctx).
		Model(&AccountModel{}).
		Where("account_id = ?", accountID).
		Updates(map[string]any{
			"suspended_at":      nil,
			"suspension_reason": nil,
		})

	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return account.ErrNotFound
	}
	return nil
}

// IsSuspended returns true if the account exists and is currently suspended.
//
// Callers that need the full account should use GetByID and check
// account.IsSuspended(). This method is a cheap check for the auth middleware
// hot path that does not need to load the full record.
func (r *AccountRepository) IsSuspended(ctx context.Context, accountID string) (bool, error) {
	var count int64

	result := r.db.WithContext(ctx).
		Model(&AccountModel{}).
		Where("account_id = ? AND suspended_at IS NOT NULL", accountID).
		Count(&count)

	if result.Error != nil {
		return false, result.Error
	}
	return count > 0, nil
}

// Delete soft-deletes an account (sets deleted_at via GORM's DeletedAt).
//
// Soft delete rather than hard delete because:
//   - Device records reference account_id via FK; hard delete cascades
//     in ways that can race with in-flight messages.
//   - Revocation lists need the account_id to remain for the lifetime of
//     any active session that might reference this account.
//   - Audit trail for abuse investigations (internal only, not user-visible).
//
// After deletion, GetByID and GetByIdentityKey will return ErrNotFound
// because GORM's soft-delete scope filters deleted_at IS NULL automatically.
func (r *AccountRepository) Delete(ctx context.Context, accountID string) error {
	result := r.db.WithContext(ctx).
		Where("account_id = ?", accountID).
		Delete(&AccountModel{})

	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return account.ErrNotFound
	}
	return nil
}

// Exists returns true if an active (non-deleted) account with this ID exists.
// Cheaper than GetByID when you only need presence, not the full record.
func (r *AccountRepository) Exists(ctx context.Context, accountID string) (bool, error) {
	var count int64

	result := r.db.WithContext(ctx).
		Model(&AccountModel{}).
		Where("account_id = ?", accountID).
		Count(&count)

	if result.Error != nil {
		return false, result.Error
	}
	return count > 0, nil
}

// isUniqueViolation checks whether err is a Postgres unique-constraint violation.
// We avoid importing lib/pq or pgx directly here — check the error message
// instead so this compiles regardless of which Postgres driver is wired in.
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	// Postgres SQLSTATE 23505 = unique_violation.
	// Both lib/pq and pgx surface this in the error string.
	return containsCode(err, "23505")
}

func containsCode(err error, code string) bool {
	type hasSQLState interface {
		SQLState() string
	}
	type hasCode interface {
		Code() string
	}

	var sqlState hasSQLState
	if errors.As(err, &sqlState) {
		return sqlState.SQLState() == code
	}
	var hasC hasCode
	if errors.As(err, &hasC) {
		return hasC.Code() == code
	}
	return false
}
