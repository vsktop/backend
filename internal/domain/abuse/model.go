package abuse

import (
	"errors"
	"time"
)

var (
	ErrNotFound     = errors.New("abuse: not found")
	ErrInvalidScore = errors.New("abuse: invalid score")
)

type IncidentType int16

const (
	IncidentSpam IncidentType = iota
	IncidentHarassment
	IncidentMalware
	IncidentPhishing
	IncidentCSAM
	IncidentToSViolation
	IncidentRateLimit
)

type Severity int16

const (
	SeverityLow Severity = iota
	SeverityMedium
	SeverityHigh
	SeverityCritical
)

type Action int16

const (
	ActionNone Action = iota
	ActionWarn
	ActionRateLimit
	ActionMute
	ActionBan
	ActionPermaBan
	ActionEscalate
)

type Incident struct {
	ID string

	TargetAccountID string

	TargetDeviceID string

	ReporterAccountID *string

	Type IncidentType

	Severity Severity

	Score int64

	Action Action

	Reason string

	Context []byte

	CreatedAt time.Time

	ResolvedAt *time.Time

	Dismissed bool
}

type ScoreEntry struct {
	AccountID string

	EventType string

	Points int64

	CreatedAt time.Time
}

func (i *Incident) Validate() error {
	if i.ID == "" {
		return errors.New("abuse: id is required")
	}
	if i.TargetAccountID == "" {
		return errors.New("abuse: target_account_id is required")
	}
	if i.Type < IncidentSpam || i.Type > IncidentRateLimit {
		return errors.New("abuse: invalid incident type")
	}
	if i.Severity < SeverityLow || i.Severity > SeverityCritical {
		return errors.New("abuse: invalid severity")
	}
	if i.Score < 0 {
		return ErrInvalidScore
	}
	return nil
}

func (s *ScoreEntry) Validate() error {
	if s.AccountID == "" {
		return errors.New("abuse: account_id is required")
	}
	if s.EventType == "" {
		return errors.New("abuse: event_type is required")
	}
	return nil
}

func DetermineAction(score int64) Action {
	switch {
	case score >= 900:
		return ActionPermaBan
	case score >= 700:
		return ActionBan
	case score >= 500:
		return ActionMute
	case score >= 300:
		return ActionRateLimit
	case score >= 100:
		return ActionWarn
	default:
		return ActionNone
	}
}

func DetermineSeverity(score int64) Severity {
	switch {
	case score >= 750:
		return SeverityCritical
	case score >= 500:
		return SeverityHigh
	case score >= 250:
		return SeverityMedium
	default:
		return SeverityLow
	}
}
