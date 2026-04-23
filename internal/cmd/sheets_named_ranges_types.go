//go:build !safety_profile

package cmd

type SheetsNamedRangesCmd struct {
	List   SheetsNamedRangesListCmd   `cmd:"" default:"withargs" help:"List named ranges"`
	Get    SheetsNamedRangesGetCmd    `cmd:"" name:"get" aliases:"show,info" help:"Get a named range"`
	Add    SheetsNamedRangesAddCmd    `cmd:"" name:"add" aliases:"create,new" help:"Add a named range"`
	Update SheetsNamedRangesUpdateCmd `cmd:"" name:"update" aliases:"edit,set" help:"Update a named range"`
	Delete SheetsNamedRangesDeleteCmd `cmd:"" name:"delete" aliases:"rm,remove,del" help:"Delete a named range"`
}
