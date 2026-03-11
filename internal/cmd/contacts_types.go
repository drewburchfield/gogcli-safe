//go:build !safety_profile

package cmd

type ContactsCmd struct {
	Search    ContactsSearchCmd    `cmd:"" name:"search" help:"Search contacts by name/email/phone"`
	List      ContactsListCmd      `cmd:"" name:"list" aliases:"ls" help:"List contacts"`
	Get       ContactsGetCmd       `cmd:"" name:"get" aliases:"info,show" help:"Get a contact"`
	Create    ContactsCreateCmd    `cmd:"" name:"create" aliases:"add,new" help:"Create a contact"`
	Update    ContactsUpdateCmd    `cmd:"" name:"update" aliases:"edit,set" help:"Update a contact"`
	Delete    ContactsDeleteCmd    `cmd:"" name:"delete" aliases:"rm,del,remove" help:"Delete a contact"`
	Directory ContactsDirectoryCmd `cmd:"" name:"directory" help:"Directory contacts"`
	Other     ContactsOtherCmd     `cmd:"" name:"other" help:"Other contacts"`
}
