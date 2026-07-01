package presence

import (
	"errors"
	"time"
)

var (
	ErrNotFound     = errors.New("presence: not found")
	ErrInvalidState = errors.New("presence: invalid state")
)

type Status int16

const (
	StatusOffline Status = iota
	StatusOnline
	StatusIdle
	StatusDND
	StatusCustom
)

type ActivityType int16

const (
	ActivityNone ActivityType = iota
	ActivityPlaying
	ActivityStreaming
	ActivityListening
	ActivityWatching
	ActivityCustom
)

type Activity struct {
	Type ActivityType

	Name string

	Details string

	State string

	LargeImageKey string

	LargeImageText string

	SmallImageKey string

	SmallImageText string

	PartyID   string
	PartySize [2]uint32

	SessionID string

	StartTimestamp *time.Time
	EndTimestamp   *time.Time

	SpliceInfo []SpliceInfo

	Secrets ActivitySecrets

	MatchID string

	Instance bool
}

type SpliceInfo struct {
	Text      string
	StartTime time.Time
	EndTime   time.Time
}

type ActivitySecrets struct {
	Match    string
	Join     string
	Spectate string
}

type Presence struct {
	AccountID string

	Status Status

	CustomStatus string

	Activity *Activity

	UpdatedAt time.Time

	Cleared bool
}

func (p *Presence) Validate() error {
	if p.AccountID == "" {
		return errors.New("presence: account_id is required")
	}
	if p.Status < StatusOffline || p.Status > StatusCustom {
		return ErrInvalidState
	}
	return nil
}

func (a *Activity) Validate() error {
	if a.Type < ActivityNone || a.Type > ActivityCustom {
		return errors.New("presence: invalid activity type")
	}
	return nil
}
