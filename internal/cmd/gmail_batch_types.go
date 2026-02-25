//go:build !safety_profile

package cmd

type GmailBatchCmd struct {
	Delete GmailBatchDeleteCmd `cmd:"" name:"delete" aliases:"rm,del,remove" help:"Permanently delete multiple messages"`
	Modify GmailBatchModifyCmd `cmd:"" name:"modify" aliases:"update,edit,set" help:"Modify labels on multiple messages"`
}
