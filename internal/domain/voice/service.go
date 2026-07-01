package voice

import (
	"context"
	"time"
)

type Repository interface {
	CreateSession(ctx context.Context, s *Session) error
	GetActiveSession(ctx context.Context, channelID string) (*Session, error)
	EndSession(ctx context.Context, sessionID string) error
	AddParticipant(ctx context.Context, sessionID, deviceID string) error
	RemoveParticipant(ctx context.Context, sessionID, deviceID string) error
	ListParticipants(ctx context.Context, sessionID string) ([]*Participant, error)
}

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) StartSession(ctx context.Context, channelID, guildID, initiatorDeviceID string) (*Session, error) {

	existing, err := s.repo.GetActiveSession(ctx, channelID)
	if err == nil && existing != nil {

		return existing, nil
	}

	session := &Session{
		ID:           generateID(),
		ChannelID:    channelID,
		GuildID:      guildID,
		State:        SessionActive,
		CreatedAt:    time.Now().UTC(),
		Participants: []string{initiatorDeviceID},
	}

	if err := session.Validate(); err != nil {
		return nil, err
	}

	if err := s.repo.CreateSession(ctx, session); err != nil {
		return nil, err
	}

	p := &Participant{
		DeviceID: initiatorDeviceID,
		VAD:      VADSilent,
		JoinedAt: session.CreatedAt,
	}
	if err := p.Validate(); err != nil {
		return nil, err
	}
	if err := s.repo.AddParticipant(ctx, session.ID, initiatorDeviceID); err != nil {
		return nil, err
	}

	return session, nil
}

func (s *Service) JoinSession(ctx context.Context, channelID, deviceID string) (*Session, error) {
	session, err := s.repo.GetActiveSession(ctx, channelID)
	if err != nil {
		return nil, ErrNotFound
	}
	if session.State != SessionActive {
		return nil, ErrInvalidState
	}

	if err := s.repo.AddParticipant(ctx, session.ID, deviceID); err != nil {
		return nil, err
	}

	session.Participants = append(session.Participants, deviceID)
	return session, nil
}

func (s *Service) LeaveSession(ctx context.Context, channelID, deviceID string) (*Session, error) {
	session, err := s.repo.GetActiveSession(ctx, channelID)
	if err != nil {
		return nil, ErrNotFound
	}

	if err := s.repo.RemoveParticipant(ctx, session.ID, deviceID); err != nil {
		return nil, err
	}

	for i, p := range session.Participants {
		if p == deviceID {
			session.Participants = append(session.Participants[:i], session.Participants[i+1:]...)
			break
		}
	}

	if len(session.Participants) == 0 {
		session.State = SessionEnding
		now := time.Now().UTC()
		session.EndedAt = &now
		if err := s.repo.EndSession(ctx, session.ID); err != nil {
			return nil, err
		}
	}

	return session, nil
}

func (s *Service) UpdateVAD(ctx context.Context, sessionID, deviceID string, vad VADState) error {

	_ = sessionID
	_ = deviceID
	_ = vad
	return nil
}

func (s *Service) UpdateMute(ctx context.Context, sessionID, deviceID string, muted bool) error {
	_ = sessionID
	_ = deviceID
	_ = muted
	return nil
}

func generateID() string {
	return ""
}
