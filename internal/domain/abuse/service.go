package abuse

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type Repository interface {
	CreateIncident(ctx context.Context, i *Incident) error
	GetByID(ctx context.Context, id string) (*Incident, error)
	ListOpen(ctx context.Context, targetAccountID string) ([]*Incident, error)
	Resolve(ctx context.Context, id string) error
	Dismiss(ctx context.Context, id string) error
	CreateScoreEntry(ctx context.Context, s *ScoreEntry) error
	GetScore(ctx context.Context, accountID string, window time.Duration) (int64, error)
	ResetScore(ctx context.Context, accountID string) error
}

type Service struct {
	repo         Repository
	scoreWindow  time.Duration
	banThreshold int64
}

func NewService(repo Repository, opts ...Option) *Service {
	s := &Service{
		repo:         repo,
		scoreWindow:  24 * time.Hour,
		banThreshold: 1000,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

type Option func(*Service)

func WithScoreWindow(d time.Duration) Option {
	return func(s *Service) { s.scoreWindow = d }
}

func WithBanThreshold(t int64) Option {
	return func(s *Service) { s.banThreshold = t }
}

func (s *Service) ReportIncident(ctx context.Context, targetAccountID, targetDeviceID string, reporterAccountID *string, incType IncidentType, reason string, contextData []byte) (*Incident, error) {
	severity := DetermineSeverity(0)
	action := DetermineAction(0)

	incident := &Incident{
		ID:                generateID(),
		TargetAccountID:   targetAccountID,
		TargetDeviceID:    targetDeviceID,
		ReporterAccountID: reporterAccountID,
		Type:              incType,
		Severity:          severity,
		Score:             0,
		Action:            action,
		Reason:            reason,
		Context:           contextData,
		CreatedAt:         time.Now().UTC(),
	}

	if err := incident.Validate(); err != nil {
		return nil, err
	}

	if err := s.repo.CreateIncident(ctx, incident); err != nil {
		return nil, err
	}

	entry := &ScoreEntry{
		AccountID: targetAccountID,
		EventType: incType.String(),
		Points:    calculatePoints(incType),
		CreatedAt: incident.CreatedAt,
	}
	if err := entry.Validate(); err != nil {
		return nil, err
	}
	if err := s.repo.CreateScoreEntry(ctx, entry); err != nil {
		return nil, err
	}

	score, err := s.repo.GetScore(ctx, targetAccountID, s.scoreWindow)
	if err != nil {
		return nil, err
	}
	incident.Score = score
	incident.Severity = DetermineSeverity(score)
	incident.Action = DetermineAction(score)

	if incident.Action != ActionNone {

		_ = incident.Action
	}

	return incident, nil
}

func (s *Service) GetIncident(ctx context.Context, id string) (*Incident, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *Service) GetOpenIncidents(ctx context.Context, targetAccountID string) ([]*Incident, error) {
	return s.repo.ListOpen(ctx, targetAccountID)
}

func (s *Service) ResolveIncident(ctx context.Context, incidentID string) error {
	return s.repo.Resolve(ctx, incidentID)
}

func (s *Service) DismissIncident(ctx context.Context, incidentID string) error {
	return s.repo.Dismiss(ctx, incidentID)
}

func (s *Service) GetScore(ctx context.Context, accountID string) (int64, error) {
	return s.repo.GetScore(ctx, accountID, s.scoreWindow)
}

func (s *Service) ResetScore(ctx context.Context, accountID string) error {
	return s.repo.ResetScore(ctx, accountID)
}

func calculatePoints(incType IncidentType) int64 {
	switch incType {
	case IncidentCSAM:
		return 500
	case IncidentMalware:
		return 300
	case IncidentPhishing:
		return 250
	case IncidentHarassment:
		return 100
	case IncidentSpam:
		return 50
	case IncidentToSViolation:
		return 150
	case IncidentRateLimit:
		return 25
	default:
		return 50
	}
}

func generateID() string {
	return uuid.New().String()
}

func (t IncidentType) String() string {
	switch t {
	case IncidentSpam:
		return "spam"
	case IncidentHarassment:
		return "harassment"
	case IncidentMalware:
		return "malware"
	case IncidentPhishing:
		return "phishing"
	case IncidentCSAM:
		return "csam"
	case IncidentToSViolation:
		return "tos_violation"
	case IncidentRateLimit:
		return "rate_limit"
	default:
		return "unknown"
	}
}
