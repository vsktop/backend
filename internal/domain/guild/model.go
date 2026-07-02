package guild

import (
	"errors"
	"time"
)

var (
	ErrNotFound      = errors.New("guild: not found")
	ErrAlreadyExists = errors.New("guild: already exists")
	ErrNotOwner      = errors.New("guild: not the owner")
	ErrInvalidRole   = errors.New("guild: invalid role")
)

type Role int16

const (
	RoleMember Role = iota
	RoleModerator
	RoleAdmin
	RoleOwner
)

type Guild struct {
	ID string

	OwnerID string

	ConfigBlob []byte

	CreatedAt time.Time

	UpdatedAt time.Time

	Deleted bool
}

type ChannelType int16

const (
	ChannelText ChannelType = iota
	ChannelVoice
)

type Channel struct {
	ID string

	GuildID string

	Type ChannelType

	ConfigBlob []byte

	CreatedAt time.Time

	Deleted bool
}

type Member struct {
	AccountID string

	GuildID string

	Role Role

	JoinedAt time.Time

	Revoked bool
}

func (g *Guild) Validate() error {
	if g.ID == "" {
		return errors.New("guild: id is required")
	}
	if g.OwnerID == "" {
		return errors.New("guild: owner_id is required")
	}
	if len(g.ConfigBlob) == 0 {
		return errors.New("guild: config_blob is required")
	}
	return nil
}

func (c *Channel) Validate() error {
	if c.ID == "" {
		return errors.New("channel: id is required")
	}
	if c.GuildID == "" {
		return errors.New("channel: guild_id is required")
	}
	if len(c.ConfigBlob) == 0 {
		return errors.New("channel: config_blob is required")
	}
	if c.Type != ChannelText && c.Type != ChannelVoice {
		return errors.New("channel: invalid type")
	}
	return nil
}

func (m *Member) Validate() error {
	if m.AccountID == "" {
		return errors.New("member: account_id is required")
	}
	if m.GuildID == "" {
		return errors.New("member: guild_id is required")
	}
	if m.Role < RoleMember || m.Role > RoleOwner {
		return ErrInvalidRole
	}
	return nil
}

func (r Role) CanManage() bool {
	return r >= RoleModerator
}

func (r Role) CanManageMembers() bool {
	return r >= RoleAdmin
}

func (r Role) IsOwner() bool {
	return r == RoleOwner
}
