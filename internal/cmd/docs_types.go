//go:build !safety_profile

package cmd

type DocsCmd struct {
	Export      DocsExportCmd      `cmd:"" name:"export" aliases:"download,dl" help:"Export a Google Doc (pdf|docx|txt)"`
	Info        DocsInfoCmd        `cmd:"" name:"info" aliases:"get,show" help:"Get Google Doc metadata"`
	Create      DocsCreateCmd      `cmd:"" name:"create" aliases:"add,new" help:"Create a Google Doc"`
	Copy        DocsCopyCmd        `cmd:"" name:"copy" aliases:"cp,duplicate" help:"Copy a Google Doc"`
	Cat         DocsCatCmd         `cmd:"" name:"cat" aliases:"text,read" help:"Print a Google Doc as plain text"`
	Comments    DocsCommentsCmd    `cmd:"" name:"comments" help:"Manage comments on a Google Doc"`
	ListTabs    DocsListTabsCmd    `cmd:"" name:"list-tabs" help:"List all tabs in a Google Doc"`
	Write       DocsWriteCmd       `cmd:"" name:"write" help:"Write content to a Google Doc"`
	Insert      DocsInsertCmd      `cmd:"" name:"insert" help:"Insert text at a specific position"`
	Delete      DocsDeleteCmd      `cmd:"" name:"delete" help:"Delete text range from document"`
	FindReplace DocsFindReplaceCmd `cmd:"" name:"find-replace" help:"Find and replace text in document"`
	Update      DocsUpdateCmd      `cmd:"" name:"update" help:"Update content in a Google Doc"`
}
