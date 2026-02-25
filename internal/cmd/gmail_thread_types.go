//go:build !safety_profile

package cmd

type GmailThreadCmd struct {
	Get         GmailThreadGetCmd         `cmd:"" name:"get" aliases:"info,show" default:"withargs" help:"Get a thread with all messages (optionally download attachments)"`
	Modify      GmailThreadModifyCmd      `cmd:"" name:"modify" aliases:"update,edit,set" help:"Modify labels on all messages in a thread"`
	Attachments GmailThreadAttachmentsCmd `cmd:"" name:"attachments" aliases:"files" help:"List all attachments in a thread"`
}
