package messaging

import (
	"errors"
	"time"
)

var (
	ErrNotFound     = errors.New("messaging: not found")
	ErrInvalidRoute = errors.New("messaging: invalid route")
	ErrTooLarge     = errors.New("messaging: message too large")
)

const (
	MaxMessageSize = 65536

	MessageTypeDirect = "direct"

	MessageTypeGuild = "guild"
)

type Message struct {
	ID string

	SenderDeviceID string

	SenderAccountID string

	Type string

	RecipientDeviceID string

	GuildID   string
	ChannelID string

	Payload []byte

	CreatedAt time.Time

	EditedAt *time.Time

	Deleted bool
}

func (m *Message) Validate() error {
	if m.ID == "" {
		return errors.New("messaging: message ID is required")
	}
	if m.SenderDeviceID == "" {
		return errors.New("messaging: sender_device_id is required")
	}
	if m.SenderAccountID == "" {
		return errors.New("messaging: sender_account_id is required")
	}
	if m.Type != MessageTypeDirect && m.Type != MessageTypeGuild {
		return ErrInvalidRoute
	}
	if len(m.Payload) > MaxMessageSize {
		return ErrTooLarge
	}
	if m.Type == MessageTypeDirect && m.RecipientDeviceID == "" {
		return errors.New("messaging: recipient_device_id is required for direct messages")
	}
	if m.Type == MessageTypeGuild && (m.GuildID == "" || m.ChannelID == "") {
		return errors.New("messaging: guild_id and channel_id are required for guild messages")
	}
	return nil
}
