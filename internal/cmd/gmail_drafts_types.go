//go:build !safety_profile

package cmd

type GmailDraftsCmd struct {
	List   GmailDraftsListCmd   `cmd:"" name:"list" aliases:"ls" help:"List drafts"`
	Get    GmailDraftsGetCmd    `cmd:"" name:"get" aliases:"info,show" help:"Get draft details"`
	Delete GmailDraftsDeleteCmd `cmd:"" name:"delete" aliases:"rm,del,remove" help:"Delete a draft"`
	Send   GmailDraftsSendCmd   `cmd:"" name:"send" aliases:"post" help:"Send a draft"`
	Create GmailDraftsCreateCmd `cmd:"" name:"create" aliases:"add,new" help:"Create a draft"`
	Update GmailDraftsUpdateCmd `cmd:"" name:"update" aliases:"edit,set" help:"Update a draft"`
}
