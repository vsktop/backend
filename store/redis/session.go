package redis

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/redis/rueidis"
)

var (
	ErrSessionNotFound = errors.New("session: not found")
	ErrSessionExpired  = errors.New("session: expired")
)

type Session struct {
	AccountID string    `json:"account_id"`
	DeviceID  string    `json:"device_id"`
	IssuedAt  time.Time `json:"issued_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

type SessionStore struct {
	client rueidis.Client
}

func NewSessionStore(client rueidis.Client) *SessionStore {
	return &SessionStore{client: client}
}

func sessionKey(token string) string {
	return "session:" + token
}

func (s *SessionStore) Create(ctx context.Context, token string, session Session) error {
	b, err := json.Marshal(session)
	if err != nil {
		return err
	}
	ttl := int64(time.Until(session.ExpiresAt).Seconds())
	if ttl <= 0 {
		return ErrSessionExpired
	}
	return s.client.Do(ctx,
		s.client.B().Set().Key(sessionKey(token)).Value(rueidis.BinaryString(b)).Nx().ExSeconds(ttl).Build(),
	).Error()
}

func (s *SessionStore) Get(ctx context.Context, token string) (Session, error) {
	b, err := s.client.Do(ctx,
		s.client.B().Get().Key(sessionKey(token)).Build(),
	).AsBytes()
	if err != nil {
		if rueidis.IsRedisNil(err) {
			return Session{}, ErrSessionNotFound
		}
		return Session{}, err
	}
	var session Session
	if err := json.Unmarshal(b, &session); err != nil {
		return Session{}, err
	}
	if time.Now().After(session.ExpiresAt) {
		return Session{}, ErrSessionExpired
	}
	return session, nil
}

func (s *SessionStore) Delete(ctx context.Context, token string) error {
	return s.client.Do(ctx,
		s.client.B().Del().Key(sessionKey(token)).Build(),
	).Error()
}

func (s *SessionStore) Extend(ctx context.Context, token string, d time.Duration) error {
	secs := int64(d.Seconds())
	if secs <= 0 {
		return errors.New("session: extend duration must be positive")
	}
	n, err := s.client.Do(ctx,
		s.client.B().Expire().Key(sessionKey(token)).Seconds(secs).Build(),
	).AsInt64()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrSessionNotFound
	}
	return nil
}

func (s *SessionStore) DeleteAllForDevice(ctx context.Context, deviceID string) error {
	var cursor uint64
	pattern := "session:*"
	for {
		scan, err := s.client.Do(ctx,
			s.client.B().Scan().Cursor(cursor).Match(pattern).Count(100).Build(),
		).AsScanEntry()
		if err != nil {
			return err
		}
		if len(scan.Elements) > 0 {
			toDelete := make([]string, 0, len(scan.Elements))
			gets := make(rueidis.Commands, 0, len(scan.Elements))
			for _, key := range scan.Elements {
				gets = append(gets, s.client.B().Get().Key(key).Build())
			}
			for i, resp := range s.client.DoMulti(ctx, gets...) {
				b, err := resp.AsBytes()
				if err != nil {
					continue
				}
				var sess Session
				if err := json.Unmarshal(b, &sess); err != nil {
					continue
				}
				if sess.DeviceID == deviceID {
					toDelete = append(toDelete, scan.Elements[i])
				}
			}
			if len(toDelete) > 0 {
				if err := s.client.Do(ctx, s.client.B().Del().Key(toDelete...).Build()).Error(); err != nil {
					return err
				}
			}
		}
		cursor = scan.Cursor
		if cursor == 0 {
			break
		}
	}
	return nil
}
