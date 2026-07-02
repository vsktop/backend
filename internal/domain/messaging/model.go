package messaging

import (
	"errors"
	"time"
)

var (
	ErrRecipientNotFound = errors.New("messaging: recipient not found")
	ErrQueueFull         = errors.New("messaging: recipient queue full")
	ErrEnvelopeTooLarge  = errors.New("messaging: envelope too large")
	ErrNotFound          = errors.New("messaging: not found")
)

const MaxEnvelopeSize = 64 * 1024

type MessageType int16

const (
	MessageTypeText    MessageType = 1
	MessageTypeFile    MessageType = 2
	MessageTypeReact   MessageType = 3
	MessageTypeControl MessageType = 4
)

type SendRequest struct {
	SenderDeviceID string

	RecipientAccountID string

	SealedEnvelope []byte

	ChannelID string

	MessageType MessageType
}

type SendResult struct {
	DeliveryID uint64

	DeliveredOnline bool
}

type QueuedMessage struct {
	ID                uint64
	RecipientDeviceID string
	SealedEnvelope    []byte
	ChannelID         string
	MessageType       MessageType
	QueuedAt          time.Time
	ExpiresAt         time.Time
}
