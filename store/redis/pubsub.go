package redis

import (
	"context"

	"github.com/redis/rueidis"
)

type PubSub struct {
	client rueidis.Client
}

func NewPubSub(client rueidis.Client) *PubSub {
	return &PubSub{client: client}
}

func (p *PubSub) Publish(ctx context.Context, channel string, payload []byte) error {
	return p.client.Do(ctx,
		p.client.B().Publish().Channel(channel).Message(rueidis.BinaryString(payload)).Build(),
	).Error()
}

func (p *PubSub) Subscribe(ctx context.Context, channel string, handler func(payload []byte)) error {
	return p.client.Receive(ctx,
		p.client.B().Subscribe().Channel(channel).Build(),
		func(msg rueidis.PubSubMessage) {
			handler([]byte(msg.Message))
		},
	)
}

func (p *PubSub) PSubscribe(ctx context.Context, pattern string, handler func(channel string, payload []byte)) error {
	return p.client.Receive(ctx,
		p.client.B().Psubscribe().Pattern(pattern).Build(),
		func(msg rueidis.PubSubMessage) {
			handler(msg.Channel, []byte(msg.Message))
		},
	)
}

func (p *PubSub) SSubscribe(ctx context.Context, channel string, handler func(payload []byte)) error {
	return p.client.Receive(ctx,
		p.client.B().Ssubscribe().Channel(channel).Build(),
		func(msg rueidis.PubSubMessage) {
			handler([]byte(msg.Message))
		},
	)
}
