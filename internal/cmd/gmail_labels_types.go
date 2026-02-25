//go:build !safety_profile

package cmd

type GmailLabelsCmd struct {
	List   GmailLabelsListCmd   `cmd:"" name:"list" aliases:"ls" help:"List labels"`
	Get    GmailLabelsGetCmd    `cmd:"" name:"get" aliases:"info,show" help:"Get label details (including counts)"`
	Create GmailLabelsCreateCmd `cmd:"" name:"create" aliases:"add,new" help:"Create a new label"`
	Rename GmailLabelsRenameCmd `cmd:"" name:"rename" aliases:"mv" help:"Rename a label"`
	Modify GmailLabelsModifyCmd `cmd:"" name:"modify" aliases:"update,edit,set" help:"Modify labels on threads"`
	Delete GmailLabelsDeleteCmd `cmd:"" name:"delete" aliases:"rm,del" help:"Delete a label"`
}
