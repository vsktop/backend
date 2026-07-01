package guild

import (
	"fmt"
)

type PermissionBitfield uint64

const (
	PermReadMessages   PermissionBitfield = 1 << 0
	PermSendMessages   PermissionBitfield = 1 << 1
	PermEmbedLinks     PermissionBitfield = 1 << 2
	PermAttachFiles    PermissionBitfield = 1 << 3
	PermAddReactions   PermissionBitfield = 1 << 4
	PermUseVoice       PermissionBitfield = 1 << 5
	PermMuteMembers    PermissionBitfield = 1 << 6
	PermDeafenMembers  PermissionBitfield = 1 << 7
	PermMoveMembers    PermissionBitfield = 1 << 8
	PermManageChannels PermissionBitfield = 1 << 9
	PermManageRoles    PermissionBitfield = 1 << 10
	PermManageGuild    PermissionBitfield = 1 << 11
	PermKickMembers    PermissionBitfield = 1 << 12
	PermBanMembers     PermissionBitfield = 1 << 13
	PermAdministrator  PermissionBitfield = 1 << 14
)

func (p PermissionBitfield) Has(perm PermissionBitfield) bool {
	return p&perm != 0
}

func (p PermissionBitfield) With(perm PermissionBitfield) PermissionBitfield {
	return p | perm
}

func (p PermissionBitfield) Without(perm PermissionBitfield) PermissionBitfield {
	return p &^ perm
}

func DefaultRolePermissions(role Role) PermissionBitfield {
	switch role {
	case RoleMember:
		return PermReadMessages | PermSendMessages | PermEmbedLinks | PermAttachFiles | PermAddReactions | PermUseVoice
	case RoleModerator:
		return DefaultRolePermissions(RoleMember) | PermMuteMembers | PermDeafenMembers | PermMoveMembers
	case RoleAdmin:
		return DefaultRolePermissions(RoleModerator) | PermManageChannels | PermManageRoles | PermKickMembers | PermBanMembers
	case RoleOwner:
		return PermManageGuild | PermManageRoles | PermManageChannels | PermKickMembers | PermBanMembers | PermAdministrator
	default:
		return 0
	}
}

func ResolvePermissions(member *Member, everyonePerms PermissionBitfield) PermissionBitfield {
	rolePerms := DefaultRolePermissions(member.Role)
	return everyonePerms | rolePerms
}

func (p PermissionBitfield) String() string {
	var perms []string
	for _, perm := range []struct {
		bit  PermissionBitfield
		name string
	}{
		{PermReadMessages, "read_messages"},
		{PermSendMessages, "send_messages"},
		{PermEmbedLinks, "embed_links"},
		{PermAttachFiles, "attach_files"},
		{PermAddReactions, "add_reactions"},
		{PermUseVoice, "use_voice"},
		{PermMuteMembers, "mute_members"},
		{PermDeafenMembers, "deafen_members"},
		{PermMoveMembers, "move_members"},
		{PermManageChannels, "manage_channels"},
		{PermManageRoles, "manage_roles"},
		{PermManageGuild, "manage_guild"},
		{PermKickMembers, "kick_members"},
		{PermBanMembers, "ban_members"},
		{PermAdministrator, "administrator"},
	} {
		if p.Has(perm.bit) {
			perms = append(perms, perm.name)
		}
	}
	return fmt.Sprintf("[%s]", perms)
}
