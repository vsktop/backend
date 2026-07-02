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

func deviceSessionsKey(deviceID string) string {
	return "device_sessions:" + deviceID
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
	cmds := make(rueidis.Commands, 0, 3)
	cmds = append(cmds, s.client.B().Set().Key(sessionKey(token)).Value(rueidis.BinaryString(b)).Nx().ExSeconds(ttl).Build())
	cmds = append(cmds, s.client.B().Sadd().Key(deviceSessionsKey(session.DeviceID)).Member(token).Build())
	cmds = append(cmds, s.client.B().Expire().Key(deviceSessionsKey(session.DeviceID)).Seconds(ttl).Gt().Build())
	for _, resp := range s.client.DoMulti(ctx, cmds...) {
		if err := resp.Error(); err != nil {
			return err
		}
	}
	return nil
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
	b, err := s.client.Do(ctx, s.client.B().Get().Key(sessionKey(token)).Build()).AsBytes()
	if err != nil {
		if rueidis.IsRedisNil(err) {
			return nil
		}
		return err
	}
	var sess Session
	if err := json.Unmarshal(b, &sess); err != nil {
		return err
	}
	cmds := make(rueidis.Commands, 0, 2)
	cmds = append(cmds, s.client.B().Del().Key(sessionKey(token)).Build())
	cmds = append(cmds, s.client.B().Srem().Key(deviceSessionsKey(sess.DeviceID)).Member(token).Build())
	for _, resp := range s.client.DoMulti(ctx, cmds...) {
		if err := resp.Error(); err != nil {
			return err
		}
	}
	return nil
}

func (s *SessionStore) Extend(ctx context.Context, token string, d time.Duration) error {
	secs := int64(d.Seconds())
	if secs <= 0 {
		return errors.New("session: extend duration must be positive")
	}
	b, err := s.client.Do(ctx, s.client.B().Get().Key(sessionKey(token)).Build()).AsBytes()
	if err != nil {
		if rueidis.IsRedisNil(err) {
			return ErrSessionNotFound
		}
		return err
	}
	var sess Session
	if err := json.Unmarshal(b, &sess); err != nil {
		return err
	}
	sess.ExpiresAt = time.Now().Add(d)
	newB, err := json.Marshal(sess)
	if err != nil {
		return err
	}
	return s.client.Do(ctx,
		s.client.B().Set().Key(sessionKey(token)).Value(rueidis.BinaryString(newB)).Xx().ExSeconds(secs).Build(),
	).Error()
}

func (s *SessionStore) DeleteAllForDevice(ctx context.Context, deviceID string) error {
	tokens, err := s.client.Do(ctx, s.client.B().Smembers().Key(deviceSessionsKey(deviceID)).Build()).AsStrSlice()
	if err != nil {
		if rueidis.IsRedisNil(err) {
			return nil
		}
		return err
	}
	if len(tokens) == 0 {
		return nil
	}
	keys := make([]string, 0, len(tokens)+1)
	for _, t := range tokens {
		keys = append(keys, sessionKey(t))
	}
	keys = append(keys, deviceSessionsKey(deviceID))
	return s.client.Do(ctx, s.client.B().Del().Key(keys...).Build()).Error()
}

