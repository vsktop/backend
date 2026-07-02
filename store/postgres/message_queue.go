package postgres

import (
	"context"
	"errors"
	"strconv"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/vsktop/backend/internal/domain/messaging"
)

var errQueueFull = errors.New("message queue full")

type MessageQueueModel struct {
	ID uint64 `gorm:"primaryKey;autoIncrement;column:id"`

	RecipientDeviceID string `gorm:"not null;type:char(32);column:recipient_device_id;index:idx_mq_device_expires"`

	SealedEnvelope []byte `gorm:"not null;type:bytea;column:sealed_envelope"`

	ChannelID   string `gorm:"not null;type:char(36);column:channel_id"`
	MessageType int16  `gorm:"not null;default:1;column:message_type"`

	QueuedAt  time.Time `gorm:"not null;autoCreateTime;column:queued_at"`
	ExpiresAt time.Time `gorm:"not null;column:expires_at;index:idx_mq_device_expires"`
}

func (MessageQueueModel) TableName() string { return "message_queue" }

const (
	msgTypeDirect int16 = 1
	msgTypeGuild  int16 = 2
)

func domainTypeToInt16(t string) int16 {
	if t == messaging.MessageTypeGuild {
		return msgTypeGuild
	}
	return msgTypeDirect
}

func int16ToDomainType(t int16) string {
	if t == msgTypeGuild {
		return messaging.MessageTypeGuild
	}
	return messaging.MessageTypeDirect
}

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

func (r *MessageQueueRepository) Enqueue(ctx context.Context, msg *messaging.Message) error {
	const MaxQueueDepth = 10_000

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
		if count >= MaxQueueDepth {
			return errQueueFull
		}

		now := time.Now().UTC()
		model := &MessageQueueModel{
			RecipientDeviceID: msg.RecipientDeviceID,
			SealedEnvelope:    msg.Payload,
			ChannelID:         msg.ChannelID,
			MessageType:       domainTypeToInt16(msg.Type),
			QueuedAt:          now,
			ExpiresAt:         now.Add(r.defaultTTL),
		}
		return tx.Create(model).Error
	})
}

func (r *MessageQueueRepository) DrainForDevice(ctx context.Context, deviceID string) ([]*messaging.Message, error) {
	var models []MessageQueueModel

	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.
			Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("recipient_device_id = ? AND expires_at > ?", deviceID, time.Now().UTC()).
			Order("id ASC").
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

	msgs := make([]*messaging.Message, len(models))
	for i, m := range models {
		m := m
		msgs[i] = queueModelToDomain(&m)
	}
	return msgs, nil
}

func (r *MessageQueueRepository) PeekForDevice(ctx context.Context, deviceID string, limit int) ([]*messaging.Message, error) {
	var models []MessageQueueModel
	result := r.db.WithContext(ctx).
		Where("recipient_device_id = ? AND expires_at > ?", deviceID, time.Now().UTC()).
		Order("id ASC").
		Limit(limit).
		Find(&models)
	if result.Error != nil {
		return nil, result.Error
	}

	msgs := make([]*messaging.Message, len(models))
	for i, m := range models {
		m := m
		msgs[i] = queueModelToDomain(&m)
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

func queueModelToDomain(m *MessageQueueModel) *messaging.Message {
	return &messaging.Message{
		ID:                strconv.FormatUint(m.ID, 10),
		RecipientDeviceID: m.RecipientDeviceID,
		Payload:           m.SealedEnvelope,
		ChannelID:         m.ChannelID,
		Type:              int16ToDomainType(m.MessageType),
		CreatedAt:         m.QueuedAt,
	}
}
