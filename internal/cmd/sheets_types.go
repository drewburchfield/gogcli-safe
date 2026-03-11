//go:build !safety_profile

package cmd

type SheetsCmd struct {
	Get      SheetsGetCmd      `cmd:"" name:"get" aliases:"read,show" help:"Get values from a range"`
	Update   SheetsUpdateCmd   `cmd:"" name:"update" aliases:"edit,set" help:"Update values in a range"`
	Append   SheetsAppendCmd   `cmd:"" name:"append" aliases:"add" help:"Append values to a range"`
	Insert   SheetsInsertCmd   `cmd:"" name:"insert" help:"Insert empty rows or columns into a sheet"`
	Clear    SheetsClearCmd    `cmd:"" name:"clear" help:"Clear values in a range"`
	Format   SheetsFormatCmd   `cmd:"" name:"format" help:"Apply cell formatting to a range"`
	Notes    SheetsNotesCmd    `cmd:"" name:"notes" help:"Get cell notes from a range"`
	Links    SheetsLinksCmd    `cmd:"" name:"links" aliases:"hyperlinks" help:"Get cell hyperlinks from a range"`
	Metadata SheetsMetadataCmd `cmd:"" name:"metadata" aliases:"info" help:"Get spreadsheet metadata"`
	Create   SheetsCreateCmd   `cmd:"" name:"create" aliases:"new" help:"Create a new spreadsheet"`
	Copy     SheetsCopyCmd     `cmd:"" name:"copy" aliases:"cp,duplicate" help:"Copy a Google Sheet"`
	Export   SheetsExportCmd   `cmd:"" name:"export" aliases:"download,dl" help:"Export a Google Sheet (pdf|xlsx|csv) via Drive"`
}
