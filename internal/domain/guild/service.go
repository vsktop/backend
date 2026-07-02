package guild

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type Repository interface {
	CreateGuild(ctx context.Context, g *Guild) error
	GetByID(ctx context.Context, id string) (*Guild, error)
	Delete(ctx context.Context, id string) error
	CreateChannel(ctx context.Context, c *Channel) error
	GetChannels(ctx context.Context, guildID string) ([]*Channel, error)
	DeleteChannel(ctx context.Context, id string) error
	CreateMember(ctx context.Context, m *Member) error
	GetMember(ctx context.Context, accountID, guildID string) (*Member, error)
	ListMembers(ctx context.Context, guildID string) ([]*Member, error)
	RemoveMember(ctx context.Context, accountID, guildID string) error
	UpdateRole(ctx context.Context, accountID, guildID string, role Role) error
}

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) CreateGuild(ctx context.Context, ownerID string, configBlob []byte) (*Guild, error) {
	g := &Guild{
		ID:         generateID(),
		OwnerID:    ownerID,
		ConfigBlob: configBlob,
		CreatedAt:  time.Now().UTC(),
		UpdatedAt:  time.Now().UTC(),
	}

	if err := g.Validate(); err != nil {
		return nil, err
	}

	if err := s.repo.CreateGuild(ctx, g); err != nil {
		return nil, err
	}

	m := &Member{
		AccountID: ownerID,
		GuildID:   g.ID,
		Role:      RoleOwner,
		JoinedAt:  g.CreatedAt,
	}
	if err := m.Validate(); err != nil {
		return nil, err
	}
	if err := s.repo.CreateMember(ctx, m); err != nil {
		return nil, err
	}

	return g, nil
}

func (s *Service) GetGuild(ctx context.Context, id string) (*Guild, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *Service) DeleteGuild(ctx context.Context, guildID, actorAccountID string) error {
	g, err := s.repo.GetByID(ctx, guildID)
	if err != nil {
		return err
	}
	if g.OwnerID != actorAccountID {
		return ErrNotOwner
	}
	return s.repo.Delete(ctx, guildID)
}

func (s *Service) CreateChannel(ctx context.Context, guildID, actorAccountID string, chType ChannelType, configBlob []byte) (*Channel, error) {
	member, err := s.repo.GetMember(ctx, actorAccountID, guildID)
	if err != nil {
		return nil, err
	}
	if !member.Role.CanManageMembers() {
		return nil, ErrNotOwner
	}

	c := &Channel{
		ID:         generateID(),
		GuildID:    guildID,
		Type:       chType,
		ConfigBlob: configBlob,
		CreatedAt:  time.Now().UTC(),
	}

	if err := c.Validate(); err != nil {
		return nil, err
	}

	if err := s.repo.CreateChannel(ctx, c); err != nil {
		return nil, err
	}
	return c, nil
}

func (s *Service) GetChannels(ctx context.Context, guildID string) ([]*Channel, error) {
	return s.repo.GetChannels(ctx, guildID)
}

func (s *Service) AddMember(ctx context.Context, guildID, accountID string) error {
	m := &Member{
		AccountID: accountID,
		GuildID:   guildID,
		Role:      RoleMember,
		JoinedAt:  time.Now().UTC(),
	}

	if err := m.Validate(); err != nil {
		return err
	}

	return s.repo.CreateMember(ctx, m)
}

func (s *Service) RemoveMember(ctx context.Context, guildID, actorAccountID, targetAccountID string) error {
	actor, err := s.repo.GetMember(ctx, actorAccountID, guildID)
	if err != nil {
		return err
	}
	if !actor.Role.CanManageMembers() {
		return ErrNotOwner
	}

	return s.repo.RemoveMember(ctx, targetAccountID, guildID)
}

func (s *Service) UpdateMemberRole(ctx context.Context, guildID, actorAccountID, targetAccountID string, role Role) error {
	actor, err := s.repo.GetMember(ctx, actorAccountID, guildID)
	if err != nil {
		return err
	}
	if !actor.Role.CanManageMembers() {
		return ErrNotOwner
	}

	if role < RoleAdmin {
		target, err := s.repo.GetMember(ctx, targetAccountID, guildID)
		if err != nil {
			return err
		}
		if target.Role == RoleOwner && !actor.Role.IsOwner() {
			return ErrNotOwner
		}
	}

	return s.repo.UpdateRole(ctx, targetAccountID, guildID, role)
}

func generateID() string {
	return uuid.New().String()
}
