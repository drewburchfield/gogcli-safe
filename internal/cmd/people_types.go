//go:build !safety_profile

package cmd

type PeopleCmd struct {
	Me        PeopleMeCmd        `cmd:"" name:"me" help:"Show your profile (people/me)"`
	Get       PeopleGetCmd       `cmd:"" name:"get" aliases:"info,show" help:"Get a user profile by ID"`
	Search    PeopleSearchCmd    `cmd:"" name:"search" aliases:"find,query" help:"Search the Workspace directory"`
	Relations PeopleRelationsCmd `cmd:"" name:"relations" help:"Get user relations"`
}
