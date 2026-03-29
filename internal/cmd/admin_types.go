//go:build !safety_profile

package cmd

type AdminCmd struct {
	Users  AdminUsersCmd  `cmd:"" name:"users" help:"Manage Workspace users"`
	Groups AdminGroupsCmd `cmd:"" name:"groups" help:"Manage Workspace groups"`
}
