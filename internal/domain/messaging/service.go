package messaging

import (
	"context"
	"time"
)

type Repository interface {
	Create(ctx context.Context, m *Message) error
	GetByID(ctx context.Context, id string) (*Message, error)
	GetByChannel(ctx context.Context, guildID, channelID string, limit, offset int) ([]*Message, error)
	GetByDirect(ctx context.Context, deviceID1, deviceID2 string, limit, offset int) ([]*Message, error)
	Delete(ctx context.Context, id string) error
	DeleteAllForChannel(ctx context.Context, guildID, channelID string) error
}

type FanoutTarget struct {
	DeviceID  string
	AccountID string
}

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) SendDirect(ctx context.Context, senderDeviceID, senderAccountID, recipientDeviceID string, payload []byte) (*Message, error) {
	msg := &Message{
		ID:                generateMessageID(),
		SenderDeviceID:    senderDeviceID,
		SenderAccountID:   senderAccountID,
		Type:              MessageTypeDirect,
		RecipientDeviceID: recipientDeviceID,
		Payload:           payload,
		CreatedAt:         time.Now().UTC(),
	}

	if err := msg.Validate(); err != nil {
		return nil, err
	}

	if err := s.repo.Create(ctx, msg); err != nil {
		return nil, err
	}
	return msg, nil
}

func (s *Service) SendGuild(ctx context.Context, senderDeviceID, senderAccountID, guildID, channelID string, payload []byte) (*Message, error) {
	msg := &Message{
		ID:              generateMessageID(),
		SenderDeviceID:  senderDeviceID,
		SenderAccountID: senderAccountID,
		Type:            MessageTypeGuild,
		GuildID:         guildID,
		ChannelID:       channelID,
		Payload:         payload,
		CreatedAt:       time.Now().UTC(),
	}

	if err := msg.Validate(); err != nil {
		return nil, err
	}

	if err := s.repo.Create(ctx, msg); err != nil {
		return nil, err
	}
	return msg, nil
}

func (s *Service) GetDirectMessages(ctx context.Context, deviceID1, deviceID2 string, limit, offset int) ([]*Message, error) {
	return s.repo.GetByDirect(ctx, deviceID1, deviceID2, limit, offset)
}

func (s *Service) GetGuildMessages(ctx context.Context, guildID, channelID string, limit, offset int) ([]*Message, error) {
	return s.repo.GetByChannel(ctx, guildID, channelID, limit, offset)
}

func (s *Service) DeleteMessage(ctx context.Context, messageID string) error {
	return s.repo.Delete(ctx, messageID)
}

func (s *Service) FanoutToDevices(ctx context.Context, msg *Message, targets []FanoutTarget) error {

	_ = targets
	return nil
}

func generateMessageID() string {
	return ""
}
