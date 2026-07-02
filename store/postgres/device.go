package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/vsktop/backend/internal/domain/device"
	"gorm.io/gorm"
)

type DeviceModel struct {
	DeviceID string `gorm:"primaryKey;type:char(32);column:device_id"`

	AccountID string `gorm:"not null;type:char(64);column:account_id;index"`

	DeviceCert []byte `gorm:"not null;type:bytea;column:device_cert"`
	LabelBlob  []byte `gorm:"type:bytea;column:label_blob"`

	PushToken     []byte `gorm:"type:bytea;column:push_token"`
	PushTokenType int16  `gorm:"column:push_token_type"` // 0=none 1=APNs 2=FCM 3=WebPush

	CreatedAt time.Time `gorm:"not null;autoCreateTime;column:created_at"`

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
		RevokedAt:     d.RevokedAt,
	}
}

type DeviceRepository struct {
	db *gorm.DB
}

func NewDeviceRepository(db *gorm.DB) *DeviceRepository {
	return &DeviceRepository{db: db}
}

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
		m := m
		devices[i] = m.toDomain()
	}
	return devices, nil
}

func (r *DeviceRepository) CountByAccount(ctx context.Context, accountID string) (int64, error) {
	var count int64
	result := r.db.WithContext(ctx).
		Model(&DeviceModel{}).
		Where("account_id = ? AND revoked_at IS NULL", accountID).
		Count(&count)
	return count, result.Error
}

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
