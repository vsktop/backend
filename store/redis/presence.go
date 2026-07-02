package redis

import (
	"context"
	"encoding/json"
	"strings"
	"sync"

	"github.com/redis/rueidis"
	"github.com/vsktop/backend/internal/domain/presence"
)

const (
	presenceTTL         = 300
	onlineKey           = "presence:online"
	presenceEventPrefix = "presence:events:"
)

func presenceKey(accountID string) string {
	return "presence:" + accountID
}

type PresenceStore struct {
	client rueidis.Client
}

func NewPresenceStore(client rueidis.Client) *PresenceStore {
	return &PresenceStore{client: client}
}

func (s *PresenceStore) Update(ctx context.Context, p *presence.Presence) error {
	b, err := json.Marshal(p)
	if err != nil {
		return err
	}
	cmds := make(rueidis.Commands, 0, 2)
	cmds = append(cmds, s.client.B().Set().Key(presenceKey(p.AccountID)).Value(rueidis.BinaryString(b)).Ex(presenceTTL).Build())
	cmds = append(cmds, s.client.B().Zadd().Key(onlineKey).ScoreMember().ScoreMember(float64(p.UpdatedAt.Unix()), p.AccountID).Build())
	for _, resp := range s.client.DoMulti(ctx, cmds...) {
		if err := resp.Error(); err != nil {
			return err
		}
	}
	return nil
}

func (s *PresenceStore) Get(ctx context.Context, accountID string) (*presence.Presence, error) {
	b, err := s.client.Do(ctx, s.client.B().Get().Key(presenceKey(accountID)).Build()).AsBytes()
	if err != nil {
		if rueidis.IsRedisNil(err) {
			return nil, presence.ErrNotFound
		}
		return nil, err
	}
	var p presence.Presence
	if err := json.Unmarshal(b, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

func (s *PresenceStore) GetAllOnline(ctx context.Context, limit int) ([]*presence.Presence, error) {
	if limit <= 0 {
		return nil, nil
	}
	members, err := s.client.Do(ctx,
		s.client.B().Zrevrangebyscore().Key(onlineKey).Max("+inf").Min("-inf").Limit(0, int64(limit)).Build(),
	).AsStrSlice()
	if err != nil {
		if rueidis.IsRedisNil(err) {
			return nil, nil
		}
		return nil, err
	}
	if len(members) == 0 {
		return nil, nil
	}
	gets := make(rueidis.Commands, 0, len(members))
	for _, accountID := range members {
		gets = append(gets, s.client.B().Get().Key(presenceKey(accountID)).Build())
	}
	result := make([]*presence.Presence, 0, len(members))
	for _, resp := range s.client.DoMulti(ctx, gets...) {
		b, err := resp.AsBytes()
		if err != nil {
			continue
		}
		var p presence.Presence
		if err := json.Unmarshal(b, &p); err != nil {
			continue
		}
		result = append(result, &p)
	}
	return result, nil
}

func (s *PresenceStore) Clear(ctx context.Context, accountID string) error {
	cmds := make(rueidis.Commands, 0, 2)
	cmds = append(cmds, s.client.B().Del().Key(presenceKey(accountID)).Build())
	cmds = append(cmds, s.client.B().Zrem().Key(onlineKey).Member(accountID).Build())
	for _, resp := range s.client.DoMulti(ctx, cmds...) {
		if err := resp.Error(); err != nil {
			return err
		}
	}
	return nil
}

type PresenceHub struct {
	client      rueidis.Client
	mu          sync.RWMutex
	subscribers map[string][]chan presence.PresenceEvent
	ctx         context.Context
	cancel      context.CancelFunc
}

func NewPresenceHub(client rueidis.Client) *PresenceHub {
	ctx, cancel := context.WithCancel(context.Background())
	h := &PresenceHub{
		client:      client,
		subscribers: make(map[string][]chan presence.PresenceEvent),
		ctx:         ctx,
		cancel:      cancel,
	}
	go h.listen()
	return h
}

func (h *PresenceHub) Subscribe(accountID string) chan presence.PresenceEvent {
	ch := make(chan presence.PresenceEvent, 16)
	h.mu.Lock()
	h.subscribers[accountID] = append(h.subscribers[accountID], ch)
	h.mu.Unlock()
	return ch
}

func (h *PresenceHub) Unsubscribe(accountID string) {
	h.mu.Lock()
	chs := h.subscribers[accountID]
	delete(h.subscribers, accountID)
	h.mu.Unlock()
	for _, ch := range chs {
		close(ch)
	}
}

func (h *PresenceHub) Broadcast(event presence.PresenceEvent) {
	b, err := json.Marshal(event)
	if err != nil {
		return
	}
	channel := presenceEventPrefix + event.AccountID
	_ = h.client.Do(h.ctx, h.client.B().Publish().Channel(channel).Message(rueidis.BinaryString(b)).Build()).Error()
}

func (h *PresenceHub) Close() {
	h.cancel()
}

func (h *PresenceHub) listen() {
	_ = h.client.Receive(h.ctx,
		h.client.B().Psubscribe().Pattern(presenceEventPrefix+"*").Build(),
		func(msg rueidis.PubSubMessage) {
			accountID := strings.TrimPrefix(msg.Channel, presenceEventPrefix)
			var event presence.PresenceEvent
			if err := json.Unmarshal([]byte(msg.Message), &event); err != nil {
				return
			}
			h.mu.RLock()
			subs := make([]chan presence.PresenceEvent, len(h.subscribers[accountID]))
			copy(subs, h.subscribers[accountID])
			h.mu.RUnlock()
			for _, ch := range subs {
				select {
				case ch <- event:
				default:
				}
			}
		},
	)
}

var _ presence.Repository = (*PresenceStore)(nil)
var _ presence.Hub = (*PresenceHub)(nil)
