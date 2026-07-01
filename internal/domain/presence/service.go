package presence

import (
	"context"
	"time"
)

type Repository interface {
	Update(ctx context.Context, p *Presence) error
	Get(ctx context.Context, accountID string) (*Presence, error)
	GetAllOnline(ctx context.Context, limit int) ([]*Presence, error)
	Clear(ctx context.Context, accountID string) error
}

type Hub interface {
	Subscribe(accountID string) chan PresenceEvent
	Unsubscribe(accountID string)
	Broadcast(event PresenceEvent)
}

type PresenceEvent struct {
	AccountID string
	Presence  *Presence
	Timestamp time.Time
}

type Service struct {
	repo Repository
	hub  Hub
}

func NewService(repo Repository, hub Hub) *Service {
	return &Service{repo: repo, hub: hub}
}

func (s *Service) UpdatePresence(ctx context.Context, accountID string, status Status, customStatus string, activity *Activity) (*Presence, error) {
	presence := &Presence{
		AccountID:    accountID,
		Status:       status,
		CustomStatus: customStatus,
		Activity:     activity,
		UpdatedAt:    time.Now().UTC(),
		Cleared:      false,
	}

	if err := presence.Validate(); err != nil {
		return nil, err
	}

	if err := s.repo.Update(ctx, presence); err != nil {
		return nil, err
	}

	s.hub.Broadcast(PresenceEvent{
		AccountID: accountID,
		Presence:  presence,
		Timestamp: presence.UpdatedAt,
	})

	return presence, nil
}

func (s *Service) GetPresence(ctx context.Context, accountID string) (*Presence, error) {
	return s.repo.Get(ctx, accountID)
}

func (s *Service) GetOnlinePresence(ctx context.Context, limit int) ([]*Presence, error) {
	return s.repo.GetAllOnline(ctx, limit)
}

func (s *Service) ClearPresence(ctx context.Context, accountID string) error {
	if err := s.repo.Clear(ctx, accountID); err != nil {
		return err
	}

	s.hub.Broadcast(PresenceEvent{
		AccountID: accountID,
		Presence: &Presence{
			AccountID: accountID,
			Status:    StatusOffline,
			UpdatedAt: time.Now().UTC(),
			Cleared:   true,
		},
		Timestamp: time.Now().UTC(),
	})

	return nil
}

func (s *Service) SetActivity(ctx context.Context, accountID string, activity *Activity) (*Presence, error) {
	presence, err := s.repo.Get(ctx, accountID)
	if err != nil {
		return nil, err
	}

	presence.Activity = activity
	presence.UpdatedAt = time.Now().UTC()

	if presence.Status == StatusOffline {
		presence.Status = StatusOnline
	}

	if err := s.repo.Update(ctx, presence); err != nil {
		return nil, err
	}

	s.hub.Broadcast(PresenceEvent{
		AccountID: accountID,
		Presence:  presence,
		Timestamp: presence.UpdatedAt,
	})

	return presence, nil
}

func (s *Service) ClearActivity(ctx context.Context, accountID string) (*Presence, error) {
	presence, err := s.repo.Get(ctx, accountID)
	if err != nil {
		return nil, err
	}

	presence.Activity = nil
	presence.UpdatedAt = time.Now().UTC()

	if err := s.repo.Update(ctx, presence); err != nil {
		return nil, err
	}

	s.hub.Broadcast(PresenceEvent{
		AccountID: accountID,
		Presence:  presence,
		Timestamp: presence.UpdatedAt,
	})

	return presence, nil
}
