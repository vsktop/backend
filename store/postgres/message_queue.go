package postgres

import (
	"context"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/vsktop/backend/internal/domain/messaging"
)

type MessageQueueModel struct {
	ID uint64 `gorm:"primaryKey;autoIncrement;column:id"`

	RecipientDeviceID string `gorm:"not null;type:char(32);column:recipient_device_id;index:idx_mq_device_expires"`

	SealedEnvelope []byte `gorm:"not null;type:bytea;column:sealed_envelope"`

	ChannelID   string `gorm:"not null;type:char(36);column:channel_id"`
	MessageType int16  `gorm:"not null;default:1;column:message_type"`

	ExpiresAt time.Time `gorm:"not null;column:expires_at;index:idx_mq_device_expires"`
}

func (MessageQueueModel) TableName() string { return "message_queue" }

type MessageQueueRepository struct {
	db         *gorm.DB
	defaultTTL time.Duration
}

func NewMessageQueueRepository(db *gorm.DB) *MessageQueueRepository {
	return &MessageQueueRepository{
		db:         db,
		defaultTTL: 7 * 24 * time.Hour,
	}
}

func (r *MessageQueueRepository) Enqueue(ctx context.Context, msg *messaging.QueuedMessage) error {
	const maxQueueDepth = 10_000

	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtext(?)::bigint)", msg.RecipientDeviceID).Error; err != nil {
			return err
		}

		var count int64
		if err := tx.Model(&MessageQueueModel{}).
			Where("recipient_device_id = ?", msg.RecipientDeviceID).
			Count(&count).Error; err != nil {
			return err
		}
		if count >= maxQueueDepth {
			return messaging.ErrQueueFull
		}

		now := time.Now().UTC()
		model := &MessageQueueModel{
			RecipientDeviceID: msg.RecipientDeviceID,
			SealedEnvelope:    msg.SealedEnvelope,
			ChannelID:         msg.ChannelID,
			MessageType:       int16(msg.MessageType),
			ExpiresAt:         now.Add(r.defaultTTL),
		}
		if err := tx.Create(model).Error; err != nil {
			return err
		}
		msg.ID = model.ID
		return nil
	})
}

func (r *MessageQueueRepository) DrainForDevice(ctx context.Context, deviceID string, limit int) ([]*messaging.QueuedMessage, error) {
	var models []MessageQueueModel

	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.
			Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("recipient_device_id = ? AND expires_at > ?", deviceID, time.Now().UTC()).
			Order("id ASC").
			Limit(limit).
			Find(&models)
		if result.Error != nil {
			return result.Error
		}
		if len(models) == 0 {
			return nil
		}

		ids := make([]uint64, len(models))
		for i, m := range models {
			ids[i] = m.ID
		}
		return tx.Where("id IN ?", ids).Delete(&MessageQueueModel{}).Error
	})

	if err != nil {
		return nil, err
	}

	msgs := make([]*messaging.QueuedMessage, len(models))
	for i := range models {
		msgs[i] = queueModelToDomain(&models[i])
	}
	return msgs, nil
}

func (r *MessageQueueRepository) CountForDevice(ctx context.Context, deviceID string) (int64, error) {
	var count int64
	result := r.db.WithContext(ctx).
		Model(&MessageQueueModel{}).
		Where("recipient_device_id = ? AND expires_at > ?", deviceID, time.Now().UTC()).
		Count(&count)
	return count, result.Error
}

func (r *MessageQueueRepository) RecipientExists(ctx context.Context, accountID string) (bool, error) {
	var count int64
	result := r.db.WithContext(ctx).
		Model(&AccountModel{}).
		Where("account_id = ? AND suspended_at IS NULL", accountID).
		Count(&count)
	return count > 0, result.Error
}

func (r *MessageQueueRepository) DevicesForAccount(ctx context.Context, accountID string) ([]string, error) {
	var ids []string
	result := r.db.WithContext(ctx).
		Model(&DeviceModel{}).
		Select("device_id").
		Where("account_id = ? AND revoked_at IS NULL", accountID).
		Scan(&ids)
	return ids, result.Error
}

func (r *MessageQueueRepository) DeleteAllForDevice(ctx context.Context, deviceID string) error {
	return r.db.WithContext(ctx).
		Where("recipient_device_id = ?", deviceID).
		Delete(&MessageQueueModel{}).Error
}

func (r *MessageQueueRepository) PurgeExpired(ctx context.Context) (int64, error) {
	result := r.db.WithContext(ctx).
		Where("expires_at <= ?", time.Now().UTC()).
		Delete(&MessageQueueModel{})
	return result.RowsAffected, result.Error
}

func queueModelToDomain(m *MessageQueueModel) *messaging.QueuedMessage {
	return &messaging.QueuedMessage{
		ID:                m.ID,
		RecipientDeviceID: m.RecipientDeviceID,
		SealedEnvelope:    m.SealedEnvelope,
		ChannelID:         m.ChannelID,
		ExpiresAt:         m.ExpiresAt,
	}
}
