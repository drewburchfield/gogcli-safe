//go:build !safety_profile

package cmd

type ChatSpacesCmd struct {
	List   ChatSpacesListCmd   `cmd:"" name:"list" aliases:"ls" help:"List spaces"`
	Find   ChatSpacesFindCmd   `cmd:"" name:"find" aliases:"search,query" help:"Find spaces by display name"`
	Create ChatSpacesCreateCmd `cmd:"" name:"create" aliases:"add,new" help:"Create a space"`
}
