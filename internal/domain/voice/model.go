package voice

import (
	"errors"
	"time"
)

var (
	ErrNotFound     = errors.New("voice: not found")
	ErrNotConnected = errors.New("voice: not connected")
	ErrInvalidState = errors.New("voice: invalid state")
)

type Session struct {
	ID string

	ChannelID string

	GuildID string

	Participants []string

	CreatedAt time.Time

	EndedAt *time.Time

	State SessionState
}

type SessionState int16

const (
	SessionIdle SessionState = iota
	SessionActive
	SessionEnding
)

type VADState int16

const (
	VADSilent VADState = iota
	VADSpeaking
)

type Participant struct {
	DeviceID string

	VAD VADState

	Muted bool

	Deafened bool

	JoinedAt time.Time

	LastSpokeAt *time.Time
}

type AudioPacket struct {
	SenderDeviceID string

	SessionID string

	Sequence uint16

	Timestamp uint32

	Payload []byte

	CreatedAt time.Time
}

func (s *Session) Validate() error {
	if s.ID == "" {
		return errors.New("voice: session id is required")
	}
	if s.ChannelID == "" {
		return errors.New("voice: channel_id is required")
	}
	if s.GuildID == "" {
		return errors.New("voice: guild_id is required")
	}
	if s.State != SessionIdle && s.State != SessionActive && s.State != SessionEnding {
		return ErrInvalidState
	}
	return nil
}

func (p *Participant) Validate() error {
	if p.DeviceID == "" {
		return errors.New("voice: device_id is required")
	}
	if p.VAD != VADSilent && p.VAD != VADSpeaking {
		return ErrInvalidState
	}
	return nil
}

func (ap *AudioPacket) Validate() error {
	if ap.SenderDeviceID == "" {
		return errors.New("voice: sender_device_id is required")
	}
	if ap.SessionID == "" {
		return errors.New("voice: session_id is required")
	}
	if len(ap.Payload) == 0 {
		return errors.New("voice: payload is required")
	}
	if len(ap.Payload) > 65507 {
		return errors.New("voice: payload too large")
	}
	return nil
}
