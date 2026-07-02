package messaging

import (
	"context"
	"fmt"
)

type OnlineDeliveryFn func(ctx context.Context, deviceID string, envelope []byte, channelID string) bool

type Repository interface {
	Enqueue(ctx context.Context, msg *QueuedMessage) error

	DrainForDevice(ctx context.Context, deviceID string, limit int) ([]*QueuedMessage, error)

	CountForDevice(ctx context.Context, deviceID string) (int64, error)

	DevicesForAccount(ctx context.Context, accountID string) ([]string, error)

	RecipientExists(ctx context.Context, accountID string) (bool, error)
}

type Service struct {
	repo    Repository
	deliver OnlineDeliveryFn
}

func NewService(repo Repository, opts ...ServiceOption) *Service {
	s := &Service{repo: repo}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

type ServiceOption func(*Service)

func WithOnlineDelivery(fn OnlineDeliveryFn) ServiceOption {
	return func(s *Service) { s.deliver = fn }
}

func (s *Service) Send(ctx context.Context, req SendRequest) (*SendResult, error) {
	if req.RecipientAccountID == "" {
		return nil, fmt.Errorf("send: recipient_account_id required")
	}
	if len(req.SealedEnvelope) == 0 {
		return nil, fmt.Errorf("send: sealed_envelope required")
	}
	if len(req.SealedEnvelope) > MaxEnvelopeSize {
		return nil, ErrEnvelopeTooLarge
	}
	if req.ChannelID == "" {
		return nil, fmt.Errorf("send: channel_id required")
	}

	exists, err := s.repo.RecipientExists(ctx, req.RecipientAccountID)
	if err != nil {
		return nil, fmt.Errorf("send: recipient lookup: %w", err)
	}
	if !exists {
		return nil, ErrRecipientNotFound
	}

	devices, err := s.repo.DevicesForAccount(ctx, req.RecipientAccountID)
	if err != nil {
		return nil, fmt.Errorf("send: device lookup: %w", err)
	}

	var deliveryID uint64
	deliveredOnline := false

	for _, deviceID := range devices {
		if deviceID == req.SenderDeviceID {
			continue
		}

		online := false
		if s.deliver != nil {
			online = s.deliver(ctx, deviceID, req.SealedEnvelope, req.ChannelID)
		}

		if online {
			deliveredOnline = true
			continue
		}

		queued := &QueuedMessage{
			RecipientDeviceID: deviceID,
			SealedEnvelope:    req.SealedEnvelope,
			ChannelID:         req.ChannelID,
			MessageType:       req.MessageType,
		}

		if err := s.repo.Enqueue(ctx, queued); err != nil {
			if err == ErrQueueFull {
				continue
			}
			return nil, fmt.Errorf("send: enqueue for device %s: %w", deviceID, err)
		}

		if deliveryID == 0 {
			deliveryID = queued.ID
		}
	}

	return &SendResult{
		DeliveryID:      deliveryID,
		DeliveredOnline: deliveredOnline,
	}, nil
}

func (s *Service) Drain(ctx context.Context, deviceID string, limit int) ([]*QueuedMessage, error) {
	if deviceID == "" {
		return nil, fmt.Errorf("drain: device_id required")
	}
	if limit <= 0 {
		limit = 100
	}

	msgs, err := s.repo.DrainForDevice(ctx, deviceID, limit)
	if err != nil {
		return nil, fmt.Errorf("drain: %w", err)
	}

	return msgs, nil
}

func (s *Service) QueuedCount(ctx context.Context, deviceID string) (int64, error) {
	if deviceID == "" {
		return 0, fmt.Errorf("queued count: device_id required")
	}

	count, err := s.repo.CountForDevice(ctx, deviceID)
	if err != nil {
		return 0, fmt.Errorf("queued count: %w", err)
	}

	return count, nil
}
