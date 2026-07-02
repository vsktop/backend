package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/vsktop/backend/internal/domain/prekey"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type SignedPreKeyModel struct {
	ID       uint64 `gorm:"primaryKey;autoIncrement;column:id"`
	DeviceID string `gorm:"not null;type:char(32);column:device_id;index"`

	KeyID uint32 `gorm:"not null;column:key_id"`

	PublicKey []byte `gorm:"not null;type:bytea;column:public_key"`

	Signature []byte `gorm:"not null;type:bytea;column:signature"`

	UploadedAt time.Time `gorm:"not null;autoCreateTime;column:uploaded_at"`
}

func (SignedPreKeyModel) TableName() string { return "signed_prekeys" }

type OneTimePreKeyModel struct {
	ID       uint64 `gorm:"primaryKey;autoIncrement;column:id"`
	DeviceID string `gorm:"not null;type:char(32);column:device_id;index"`

	KeyID uint32 `gorm:"not null;column:key_id"`

	PublicKey []byte `gorm:"not null;type:bytea;column:public_key"`

	Signature []byte `gorm:"not null;type:bytea;column:signature"`

	UploadedAt time.Time `gorm:"not null;autoCreateTime;column:uploaded_at"`
}

func (OneTimePreKeyModel) TableName() string { return "one_time_prekeys" }

type PreKeyRepository struct {
	db *gorm.DB
}

func NewPreKeyRepository(db *gorm.DB) *PreKeyRepository {
	return &PreKeyRepository{db: db}
}

func (r *PreKeyRepository) UpsertSignedPreKey(ctx context.Context, spk *prekey.SignedPreKey) error {
	model := &SignedPreKeyModel{
		DeviceID:   spk.DeviceID,
		KeyID:      spk.KeyID,
		PublicKey:  spk.PublicKey,
		Signature:  spk.Signature,
		UploadedAt: time.Now().UTC(),
	}
	result := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "device_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"key_id", "public_key", "signature", "uploaded_at"}),
		}).
		Create(model)

	return result.Error
}

func (r *PreKeyRepository) GetSignedPreKey(ctx context.Context, deviceID string) (*prekey.SignedPreKey, error) {
	var model SignedPreKeyModel
	result := r.db.WithContext(ctx).
		Where("device_id = ?", deviceID).
		Order("uploaded_at DESC").
		First(&model)

	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, prekey.ErrNotFound
		}
		return nil, result.Error
	}

	return &prekey.SignedPreKey{
		DeviceID:  model.DeviceID,
		KeyID:     model.KeyID,
		PublicKey: model.PublicKey,
		Signature: model.Signature,
	}, nil
}

func (r *PreKeyRepository) UploadOneTimePreKeys(ctx context.Context, deviceID string, keys []prekey.OneTimePreKey) error {
	if len(keys) == 0 {
		return nil
	}

	models := make([]OneTimePreKeyModel, len(keys))
	for i, k := range keys {
		models[i] = OneTimePreKeyModel{
			DeviceID:  deviceID,
			KeyID:     k.KeyID,
			PublicKey: k.PublicKey,
			Signature: k.Signature,
		}
	}

	result := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).
		CreateInBatches(models, 50)

	return result.Error
}

func (r *PreKeyRepository) ConsumeOneTimePreKey(ctx context.Context, deviceID string) (*prekey.OneTimePreKey, error) {
	var model OneTimePreKeyModel

	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.
			Set("gorm:query_option", "FOR UPDATE SKIP LOCKED").
			Where("device_id = ?", deviceID).
			Order("id ASC").
			First(&model)

		if result.Error != nil {
			if errors.Is(result.Error, gorm.ErrRecordNotFound) {
				return prekey.ErrOTKExhausted
			}
			return result.Error
		}

		return tx.Delete(&OneTimePreKeyModel{}, model.ID).Error
	})

	if err != nil {
		return nil, err
	}

	return &prekey.OneTimePreKey{
		DeviceID:  model.DeviceID,
		KeyID:     model.KeyID,
		PublicKey: model.PublicKey,
		Signature: model.Signature,
	}, nil
}

func (r *PreKeyRepository) CountOneTimePreKeys(ctx context.Context, deviceID string) (int64, error) {
	var count int64
	result := r.db.WithContext(ctx).
		Model(&OneTimePreKeyModel{}).
		Where("device_id = ?", deviceID).
		Count(&count)
	return count, result.Error
}

func (r *PreKeyRepository) DeleteAllForDevice(ctx context.Context, deviceID string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("device_id = ?", deviceID).
			Delete(&SignedPreKeyModel{}).Error; err != nil {
			return err
		}
		return tx.Where("device_id = ?", deviceID).
			Delete(&OneTimePreKeyModel{}).Error
	})
}

func (r *PreKeyRepository) FetchBundle(ctx context.Context, deviceID string) (*prekey.Bundle, error) {
	spk, err := r.GetSignedPreKey(ctx, deviceID)
	if err != nil {
		return nil, err
	}

	otk, err := r.ConsumeOneTimePreKey(ctx, deviceID)
	if err != nil && !errors.Is(err, prekey.ErrOTKExhausted) {
		return nil, err
	}

	return &prekey.Bundle{
		DeviceID: deviceID,
		SPK:      spk,
		OTK:      otk,
	}, nil
}
