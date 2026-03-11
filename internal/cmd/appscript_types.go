//go:build !safety_profile

package cmd

type AppScriptCmd struct {
	Get     AppScriptGetCmd     `cmd:"" name:"get" aliases:"info,show" help:"Get Apps Script project metadata"`
	Content AppScriptContentCmd `cmd:"" name:"content" aliases:"cat" help:"Get Apps Script project content"`
	Run     AppScriptRunCmd     `cmd:"" name:"run" help:"Run a deployed Apps Script function"`
	Create  AppScriptCreateCmd  `cmd:"" name:"create" aliases:"new" help:"Create an Apps Script project"`
}
