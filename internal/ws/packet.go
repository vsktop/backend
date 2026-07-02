// wire message types
package ws

import "encoding/json"

type OpCode uint16

const (
	OpHeartbeat        OpCode = 0
	OpHeartbeatAck     OpCode = 1
	OpIdentify         OpCode = 2
	OpReady            OpCode = 3
	OpPresenceUpdate   OpCode = 4
	OpMessageCreate    OpCode = 5
	OpMessageDelete    OpCode = 6
	OpTypingStart      OpCode = 7
	OpVoiceStateUpdate OpCode = 8
	OpVoiceSignal      OpCode = 9
	OpMemberUpdate     OpCode = 10
	OpError            OpCode = 11
)

type Packet struct {
	Op   OpCode          `json:"op"`
	Data json.RawMessage `json:"d,omitempty"`
	Seq  uint64          `json:"s,omitempty"`
}

type HeartbeatData struct {
	Seq uint64 `json:"seq"`
}

type IdentifyData struct {
	Token string `json:"token"`
}

type ReadyData struct {
	SessionID string `json:"session_id"`
	AccountID string `json:"account_id"`
	DeviceID  string `json:"device_id"`
}

type PresenceUpdateData struct {
	Status       int16  `json:"status"`
	CustomStatus string `json:"custom_status,omitempty"`
}

type MessageCreateData struct {
	ChannelID      string `json:"channel_id"`
	SealedEnvelope []byte `json:"sealed_envelope"`
}

type MessageDeleteData struct {
	ChannelID string `json:"channel_id"`
	MessageID string `json:"message_id"`
}

type TypingData struct {
	ChannelID string `json:"channel_id"`
	AccountID string `json:"account_id,omitempty"`
}

type VoiceStateData struct {
	ChannelID string `json:"channel_id"`
	Muted     bool   `json:"muted"`
	Deafened  bool   `json:"deafened"`
}

type VoiceSignalData struct {
	ChannelID string          `json:"channel_id"`
	TargetID  string          `json:"target_id,omitempty"`
	Payload   json.RawMessage `json:"payload"`
}

type MemberUpdateData struct {
	GuildID   string `json:"guild_id"`
	AccountID string `json:"account_id"`
	Roles     int64  `json:"roles"`
}

type ErrorData struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

const (
	CodeDecodeError      = 4001
	CodeUnknownOp        = 4002
	CodeNotAuthenticated = 4003
	CodeAuthFailed       = 4004
	CodeSessionExpired   = 4009
	CodeRateLimited      = 4029
)

func encode(op OpCode, data any, seq uint64) ([]byte, error) {
	d, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	return json.Marshal(Packet{Op: op, Data: d, Seq: seq})
}
