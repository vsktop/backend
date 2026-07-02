// dispatch by messaage type
package ws

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/vsktop/backend/internal/domain/presence"
)

type HandlerFunc func(ctx context.Context, c *Client, data json.RawMessage) error

type Router struct {
	handlers    map[OpCode]HandlerFunc
	hub         *Hub
	presenceSvc *presence.Service
}

func NewRouter(hub *Hub, presenceSvc *presence.Service) *Router {
	r := &Router{
		handlers:    make(map[OpCode]HandlerFunc),
		hub:         hub,
		presenceSvc: presenceSvc,
	}
	r.on(OpHeartbeat, r.handleHeartbeat)
	r.on(OpPresenceUpdate, r.handlePresenceUpdate)
	r.on(OpTypingStart, r.handleTypingStart)
	r.on(OpVoiceStateUpdate, r.handleVoiceStateUpdate)
	r.on(OpVoiceSignal, r.handleVoiceSignal)
	return r
}

func (r *Router) on(op OpCode, h HandlerFunc) {
	r.handlers[op] = h
}

func (r *Router) Handle(ctx context.Context, c *Client, pkt *Packet) error {
	h, ok := r.handlers[pkt.Op]
	if !ok {
		return fmt.Errorf("ws: unknown opcode %d", pkt.Op)
	}
	return h(ctx, c, pkt.Data)
}

var errBadPayload = errors.New("ws: bad payload")

func (r *Router) handleHeartbeat(_ context.Context, c *Client, data json.RawMessage) error {
	var hb HeartbeatData
	if err := json.Unmarshal(data, &hb); err != nil {
		return errBadPayload
	}
	ack, err := encode(OpHeartbeatAck, HeartbeatData{Seq: hb.Seq}, c.nextSeq())
	if err != nil {
		return err
	}
	c.enqueue(ack)
	return nil
}

func (r *Router) handlePresenceUpdate(ctx context.Context, c *Client, data json.RawMessage) error {
	var upd PresenceUpdateData
	if err := json.Unmarshal(data, &upd); err != nil {
		return errBadPayload
	}
	_, err := r.presenceSvc.UpdatePresence(ctx, c.AccountID,
		presence.Status(upd.Status), upd.CustomStatus, nil)
	return err
}

func (r *Router) handleTypingStart(_ context.Context, c *Client, data json.RawMessage) error {
	var td TypingData
	if err := json.Unmarshal(data, &td); err != nil || td.ChannelID == "" {
		return errBadPayload
	}
	payload, err := encode(OpTypingStart, TypingData{
		ChannelID: td.ChannelID,
		AccountID: c.AccountID,
	}, 0)
	if err != nil {
		return err
	}
	r.hub.Broadcast(c, payload)
	return nil
}

func (r *Router) handleVoiceStateUpdate(_ context.Context, c *Client, data json.RawMessage) error {
	var vs VoiceStateData
	if err := json.Unmarshal(data, &vs); err != nil || vs.ChannelID == "" {
		return errBadPayload
	}
	payload, err := encode(OpVoiceStateUpdate, vs, 0)
	if err != nil {
		return err
	}
	r.hub.Broadcast(c, payload)
	return nil
}

func (r *Router) handleVoiceSignal(_ context.Context, c *Client, data json.RawMessage) error {
	var vs VoiceSignalData
	if err := json.Unmarshal(data, &vs); err != nil || vs.ChannelID == "" {
		return errBadPayload
	}
	payload, err := encode(OpVoiceSignal, vs, 0)
	if err != nil {
		return err
	}
	if vs.TargetID != "" {
		r.hub.Send(vs.TargetID, payload)
	} else {
		r.hub.Broadcast(c, payload)
	}
	return nil
}
