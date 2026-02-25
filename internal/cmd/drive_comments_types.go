//go:build !safety_profile

package cmd

type DriveCommentsCmd struct {
	List   DriveCommentsListCmd   `cmd:"" name:"list" aliases:"ls" help:"List comments on a file"`
	Get    DriveCommentsGetCmd    `cmd:"" name:"get" aliases:"info,show" help:"Get a comment by ID"`
	Create DriveCommentsCreateCmd `cmd:"" name:"create" aliases:"add,new" help:"Create a comment on a file"`
	Update DriveCommentsUpdateCmd `cmd:"" name:"update" aliases:"edit,set" help:"Update a comment"`
	Delete DriveCommentsDeleteCmd `cmd:"" name:"delete" aliases:"rm,del,remove" help:"Delete a comment"`
	Reply  DriveCommentReplyCmd   `cmd:"" name:"reply" aliases:"respond" help:"Reply to a comment"`
}
