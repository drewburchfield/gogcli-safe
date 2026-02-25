//go:build !safety_profile

package cmd

type DocsCommentsCmd struct {
	List    DocsCommentsListCmd    `cmd:"" name:"list" aliases:"ls" help:"List comments on a Google Doc"`
	Get     DocsCommentsGetCmd     `cmd:"" name:"get" aliases:"info,show" help:"Get a comment by ID"`
	Add     DocsCommentsAddCmd     `cmd:"" name:"add" aliases:"create,new" help:"Add a comment to a Google Doc"`
	Reply   DocsCommentsReplyCmd   `cmd:"" name:"reply" aliases:"respond" help:"Reply to a comment"`
	Resolve DocsCommentsResolveCmd `cmd:"" name:"resolve" help:"Resolve a comment (mark as done)"`
	Delete  DocsCommentsDeleteCmd  `cmd:"" name:"delete" aliases:"rm,del,remove" help:"Delete a comment"`
}
