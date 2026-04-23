//go:build !safety_profile

package cmd

type SheetsChartCmd struct {
	List   SheetsChartListCmd   `cmd:"" default:"withargs" help:"List charts in a spreadsheet"`
	Get    SheetsChartGetCmd    `cmd:"" name:"get" aliases:"show,info" help:"Get full chart definition (spec + position)"`
	Create SheetsChartCreateCmd `cmd:"" name:"create" aliases:"add,new" help:"Create a chart from a JSON spec"`
	Update SheetsChartUpdateCmd `cmd:"" name:"update" aliases:"edit,set" help:"Update a chart spec"`
	Delete SheetsChartDeleteCmd `cmd:"" name:"delete" aliases:"rm,remove,del" help:"Delete a chart"`
}
