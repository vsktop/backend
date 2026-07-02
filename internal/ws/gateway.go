// conn lifecycle, upgrade
package ws

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"time"

	"github.com/vsktop/backend/internal/domain/presence"
	rstore "github.com/vsktop/backend/store/redis"
)

const (
	writeWait     = 10 * time.Second
	heartbeatWait = 65 * time.Second
	identifyWait  = 10 * time.Second
	maxFrameBytes = 4096
	textMessage   = 1
)

type Conn interface {
	ReadMessage() (messageType int, p []byte, err error)
	WriteMessage(messageType int, data []byte) error
	SetReadDeadline(t time.Time) error
	SetWriteDeadline(t time.Time) error
	SetReadLimit(limit int64)
	Close() error
	RemoteAddr() net.Addr
}

type Gateway struct {
	hub      *Hub
	router   *Router
	sessions *rstore.SessionStore
	presence *presence.Service
}

func NewGateway(hub *Hub, router *Router, sessions *rstore.SessionStore, presenceSvc *presence.Service) *Gateway {
	return &Gateway{hub: hub, router: router, sessions: sessions, presence: presenceSvc}
}

func (g *Gateway) Serve(ctx context.Context, conn Conn) {
	conn.SetReadLimit(maxFrameBytes)

	client, err := g.identify(ctx, conn)
	if err != nil {
		writeErrorFrame(conn, CodeAuthFailed, err.Error())
		return
	}

	g.hub.Register(client)

	writeDone := make(chan struct{})
	go func() {
		defer close(writeDone)
		g.writePump(conn, client)
	}()

	ready, _ := encode(OpReady, ReadyData{
		SessionID: client.SessionID,
		AccountID: client.AccountID,
		DeviceID:  client.DeviceID,
	}, client.nextSeq())
	conn.SetWriteDeadline(time.Now().Add(writeWait))
	conn.WriteMessage(textMessage, ready)

	g.readPump(ctx, conn, client)

	g.hub.Unregister(client)
	_ = g.presence.ClearPresence(ctx, client.AccountID)
	close(client.send)
	<-writeDone
}

func (g *Gateway) identify(ctx context.Context, conn Conn) (*Client, error) {
	conn.SetReadDeadline(time.Now().Add(identifyWait))

	_, raw, err := conn.ReadMessage()
	if err != nil {
		return nil, err
	}

	var pkt Packet
	if err := json.Unmarshal(raw, &pkt); err != nil {
		return nil, errors.New("malformed frame")
	}
	if pkt.Op != OpIdentify {
		return nil, errors.New("expected identify as first message")
	}

	var id IdentifyData
	if err := json.Unmarshal(pkt.Data, &id); err != nil || id.Token == "" {
		return nil, errors.New("malformed identify payload")
	}

	sess, err := g.sessions.Get(ctx, id.Token)
	if err != nil {
		if errors.Is(err, rstore.ErrSessionNotFound) || errors.Is(err, rstore.ErrSessionExpired) {
			return nil, errors.New("session not found or expired")
		}
		return nil, err
	}

	return &Client{
		AccountID: sess.AccountID,
		DeviceID:  sess.DeviceID,
		SessionID: id.Token,
		send:      make(chan []byte, sendBufSize),
		hub:       g.hub,
	}, nil
}

func (g *Gateway) readPump(ctx context.Context, conn Conn, c *Client) {
	for {
		conn.SetReadDeadline(time.Now().Add(heartbeatWait))

		_, raw, err := conn.ReadMessage()
		if err != nil {
			return
		}

		var pkt Packet
		if err := json.Unmarshal(raw, &pkt); err != nil {
			continue
		}

		_ = g.router.Handle(ctx, c, &pkt)
	}
}

func (g *Gateway) writePump(conn Conn, c *Client) {
	for payload := range c.send {
		conn.SetWriteDeadline(time.Now().Add(writeWait))
		if err := conn.WriteMessage(textMessage, payload); err != nil {
			return
		}
	}
}

func writeErrorFrame(conn Conn, code int, msg string) {
	b, _ := encode(OpError, ErrorData{Code: code, Message: msg}, 0)
	conn.SetWriteDeadline(time.Now().Add(writeWait))
	conn.WriteMessage(textMessage, b)
	conn.Close()
}
