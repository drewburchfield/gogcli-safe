//go:build !safety_profile

package cmd

type SheetsCmd struct {
	Get           SheetsGetCmd           `cmd:"" name:"get" aliases:"read,show" help:"Get values from a range"`
	Update        SheetsUpdateCmd        `cmd:"" name:"update" aliases:"edit,set" help:"Update values in a range"`
	Append        SheetsAppendCmd        `cmd:"" name:"append" aliases:"add" help:"Append values to a range"`
	Insert        SheetsInsertCmd        `cmd:"" name:"insert" help:"Insert empty rows or columns into a sheet"`
	Clear         SheetsClearCmd         `cmd:"" name:"clear" help:"Clear values in a range"`
	Format        SheetsFormatCmd        `cmd:"" name:"format" help:"Apply cell formatting to a range"`
	Merge         SheetsMergeCmd         `cmd:"" name:"merge" help:"Merge cells in a range"`
	Unmerge       SheetsUnmergeCmd       `cmd:"" name:"unmerge" help:"Unmerge cells in a range"`
	NumberFormat  SheetsNumberFormatCmd  `cmd:"" name:"number-format" help:"Apply number format to a range"`
	Freeze        SheetsFreezeCmd        `cmd:"" name:"freeze" help:"Freeze rows and columns on a sheet"`
	ResizeColumns SheetsResizeColumnsCmd `cmd:"" name:"resize-columns" help:"Resize sheet columns"`
	ResizeRows    SheetsResizeRowsCmd    `cmd:"" name:"resize-rows" help:"Resize sheet rows"`
	ReadFormat    SheetsReadFormatCmd    `cmd:"" name:"read-format" aliases:"get-format,format-read" help:"Read cell formatting from a range"`
	Notes         SheetsNotesCmd         `cmd:"" name:"notes" help:"Get cell notes from a range"`
	UpdateNote    SheetsUpdateNoteCmd    `cmd:"" name:"update-note" aliases:"set-note" help:"Set or clear a cell note"`
	FindReplace   SheetsFindReplaceCmd   `cmd:"" name:"find-replace" help:"Find and replace text across a spreadsheet"`
	Links         SheetsLinksCmd         `cmd:"" name:"links" aliases:"hyperlinks" help:"Get cell hyperlinks from a range"`
	Named         SheetsNamedRangesCmd   `cmd:"" name:"named-ranges" aliases:"namedranges,nr" help:"Manage named ranges"`
	Metadata      SheetsMetadataCmd      `cmd:"" name:"metadata" aliases:"info" help:"Get spreadsheet metadata"`
	Create        SheetsCreateCmd        `cmd:"" name:"create" aliases:"new" help:"Create a new spreadsheet"`
	Copy          SheetsCopyCmd          `cmd:"" name:"copy" aliases:"cp,duplicate" help:"Copy a Google Sheet"`
	Export        SheetsExportCmd        `cmd:"" name:"export" aliases:"download,dl" help:"Export a Google Sheet (pdf|xlsx|csv) via Drive"`
	AddTab        SheetsAddTabCmd        `cmd:"" name:"add-tab" help:"Add a new tab/sheet to a spreadsheet"`
	RenameTab     SheetsRenameTabCmd     `cmd:"" name:"rename-tab" help:"Rename a tab/sheet in a spreadsheet"`
	DeleteTab     SheetsDeleteTabCmd     `cmd:"" name:"delete-tab" help:"Delete a tab/sheet from a spreadsheet (use --force to skip confirmation)"`
}
