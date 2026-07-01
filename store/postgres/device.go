package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/vsktop/backend/internal/domain/device"
	"gorm.io/gorm"
)

// DeviceModel is the GORM persistence model for a linked device.
// A device is any client that holds a device keypair signed by IK_A.
// One account can have multiple active devices.
type DeviceModel struct {
	// device_id = hex(SHA-256(IK_D_public)[0:16]) — 32 hex chars.
	// Derived client-side, never assigned by the server.
	DeviceID string `gorm:"primaryKey;type:char(32);column:device_id"`

	// FK to accounts.account_id.
	AccountID string `gorm:"not null;type:char(64);column:account_id;index"`

	// The full device certificate blob:
	//   IK_D_public (32 bytes) || Ed25519 sig by IK_A (64 bytes) = 96 bytes.
	// Stored raw — verified client-side by peers when establishing sessions.
	DeviceCert []byte `gorm:"not null;type:bytea;column:device_cert"`

	// Human-readable label set by the user ("MacBook", "Phone").
	// Stored as an encrypted blob — the server cannot read it.
	// Nil until the user sets one.
	LabelBlob []byte `gorm:"type:bytea;column:label_blob"`

	// Push notification delivery token (APNs / FCM / WebPush).
	// Encrypted at rest — only used for offline push, never logged.
	PushToken     []byte `gorm:"type:bytea;column:push_token"`
	PushTokenType int16  `gorm:"column:push_token_type"` // 0=none 1=APNs 2=FCM 3=WebPush

	// Stored at second granularity only — never finer.
	CreatedAt time.Time `gorm:"not null;autoCreateTime;column:created_at"`

	// last_seen_at truncated to the hour. Updated on each authenticated
	// connection. We never store minute or second granularity here —
	// that would build a precise activity timeline.
	LastSeenAt time.Time `gorm:"column:last_seen_at"`

	// NULL = active. Populated when the owner revokes this device.
	RevokedAt *time.Time `gorm:"column:revoked_at;index"`

	DeletedAt gorm.DeletedAt `gorm:"index;column:deleted_at"`
}

func (DeviceModel) TableName() string { return "devices" }

func (m *DeviceModel) toDomain() *device.Device {
	return &device.Device{
		DeviceID:      m.DeviceID,
		AccountID:     m.AccountID,
		DeviceCert:    m.DeviceCert,
		LabelBlob:     m.LabelBlob,
		PushToken:     m.PushToken,
		PushTokenType: device.PushTokenType(m.PushTokenType),
		CreatedAt:     m.CreatedAt,
		LastSeenAt:    m.LastSeenAt,
		RevokedAt:     m.RevokedAt,
	}
}

func fromDomainDevice(d *device.Device) *DeviceModel {
	return &DeviceModel{
		DeviceID:      d.DeviceID,
		AccountID:     d.AccountID,
		DeviceCert:    d.DeviceCert,
		LabelBlob:     d.LabelBlob,
		PushToken:     d.PushToken,
		PushTokenType: int16(d.PushTokenType),
		CreatedAt:     d.CreatedAt,
		LastSeenAt:    d.LastSeenAt,
		RevokedAt:     d.RevokedAt,
	}
}

// DeviceRepository implements domain/device.Repository against Postgres.
type DeviceRepository struct {
	db *gorm.DB
}

func NewDeviceRepository(db *gorm.DB) *DeviceRepository {
	return &DeviceRepository{db: db}
}

// Create registers a new device under an account.
// Returns device.ErrAlreadyExists if device_id is already registered.
func (r *DeviceRepository) Create(ctx context.Context, d *device.Device) error {
	model := fromDomainDevice(d)
	result := r.db.WithContext(ctx).Create(model)
	if result.Error != nil {
		if isUniqueViolation(result.Error) {
			return device.ErrAlreadyExists
		}
		return result.Error
	}
	return nil
}

// GetByID returns a single active (non-revoked, non-deleted) device.
func (r *DeviceRepository) GetByID(ctx context.Context, deviceID string) (*device.Device, error) {
	var model DeviceModel
	result := r.db.WithContext(ctx).
		Where("device_id = ? AND revoked_at IS NULL", deviceID).
		First(&model)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, device.ErrNotFound
		}
		return nil, result.Error
	}
	return model.toDomain(), nil
}

// ListByAccount returns all active devices for an account, ordered by
// creation time ascending (oldest device first).
func (r *DeviceRepository) ListByAccount(ctx context.Context, accountID string) ([]*device.Device, error) {
	var models []DeviceModel
	result := r.db.WithContext(ctx).
		Where("account_id = ? AND revoked_at IS NULL", accountID).
		Order("created_at ASC").
		Find(&models)
	if result.Error != nil {
		return nil, result.Error
	}

	devices := make([]*device.Device, len(models))
	for i, m := range models {
		m := m // avoid loop variable capture
		devices[i] = m.toDomain()
	}
	return devices, nil
}

// CountByAccount returns how many active devices an account has.
// Used to enforce a per-account device limit (recommended: 10).
func (r *DeviceRepository) CountByAccount(ctx context.Context, accountID string) (int64, error) {
	var count int64
	result := r.db.WithContext(ctx).
		Model(&DeviceModel{}).
		Where("account_id = ? AND revoked_at IS NULL", accountID).
		Count(&count)
	return count, result.Error
}

// Revoke marks a device as revoked. Revoked devices can no longer
// authenticate sessions and their prekeys are ignored by peers.
// Does not hard-delete — we keep the record so the revocation is
// visible to other devices fetching the account's device roster.
func (r *DeviceRepository) Revoke(ctx context.Context, deviceID string) error {
	now := time.Now().UTC()
	result := r.db.WithContext(ctx).
		Model(&DeviceModel{}).
		Where("device_id = ? AND revoked_at IS NULL", deviceID).
		Update("revoked_at", now)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return device.ErrNotFound
	}
	return nil
}

// RevokeAllForAccount revokes every active device on an account except
// the one performing the revocation (exemptDeviceID).
// Used during account recovery to invalidate all old devices at once.
// Pass an empty exemptDeviceID to revoke everything.
func (r *DeviceRepository) RevokeAllForAccount(ctx context.Context, accountID, exemptDeviceID string) error {
	now := time.Now().UTC()
	q := r.db.WithContext(ctx).
		Model(&DeviceModel{}).
		Where("account_id = ? AND revoked_at IS NULL", accountID)

	if exemptDeviceID != "" {
		q = q.Where("device_id != ?", exemptDeviceID)
	}

	return q.Update("revoked_at", now).Error
}

// UpdateLastSeen updates the last_seen_at timestamp for a device,
// truncated to the hour. Called on each authenticated connection.
func (r *DeviceRepository) UpdateLastSeen(ctx context.Context, deviceID string) error {
	// Truncate to the hour — never store minute or second granularity.
	now := time.Now().UTC().Truncate(time.Hour)
	return r.db.WithContext(ctx).
		Model(&DeviceModel{}).
		Where("device_id = ?", deviceID).
		Update("last_seen_at", now).Error
}

// UpdatePushToken replaces the push notification token for a device.
// Pass tokenType=0 and nil token to clear push delivery.
func (r *DeviceRepository) UpdatePushToken(ctx context.Context, deviceID string, tokenType device.PushTokenType, token []byte) error {
	result := r.db.WithContext(ctx).
		Model(&DeviceModel{}).
		Where("device_id = ?", deviceID).
		Updates(map[string]any{
			"push_token":      token,
			"push_token_type": int16(tokenType),
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return device.ErrNotFound
	}
	return nil
}
