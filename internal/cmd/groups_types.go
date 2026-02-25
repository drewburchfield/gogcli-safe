//go:build !safety_profile

package cmd

type GroupsCmd struct {
	List    GroupsListCmd    `cmd:"" name:"list" aliases:"ls" help:"List groups you belong to"`
	Members GroupsMembersCmd `cmd:"" name:"members" help:"List members of a group"`
}
