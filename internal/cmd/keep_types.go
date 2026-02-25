//go:build !safety_profile

package cmd

type KeepCmd struct {
	ServiceAccount string `name:"service-account" help:"Path to service account JSON file"`
	Impersonate    string `name:"impersonate" help:"Email to impersonate (required with service-account)"`

	List       KeepListCmd       `cmd:"" default:"withargs" help:"List notes"`
	Get        KeepGetCmd        `cmd:"" name:"get" help:"Get a note"`
	Search     KeepSearchCmd     `cmd:"" name:"search" help:"Search notes by text (client-side)"`
	Attachment KeepAttachmentCmd `cmd:"" name:"attachment" help:"Download an attachment"`
}
