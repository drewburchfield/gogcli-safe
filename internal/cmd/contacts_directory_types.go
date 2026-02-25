//go:build !safety_profile

package cmd

type ContactsDirectoryCmd struct {
	List   ContactsDirectoryListCmd   `cmd:"" name:"list" help:"List people from the Workspace directory"`
	Search ContactsDirectorySearchCmd `cmd:"" name:"search" help:"Search people in the Workspace directory"`
}

type ContactsOtherCmd struct {
	List   ContactsOtherListCmd   `cmd:"" name:"list" help:"List other contacts"`
	Search ContactsOtherSearchCmd `cmd:"" name:"search" help:"Search other contacts"`
	Delete ContactsOtherDeleteCmd `cmd:"" name:"delete" help:"Delete an other contact"`
}
