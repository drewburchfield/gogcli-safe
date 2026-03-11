//go:build !safety_profile

package cmd

type FormsCmd struct {
	Get       FormsGetCmd       `cmd:"" name:"get" aliases:"info,show" help:"Get a form"`
	Create    FormsCreateCmd    `cmd:"" name:"create" aliases:"new" help:"Create a form"`
	Responses FormsResponsesCmd `cmd:"" name:"responses" help:"Form responses"`
}

type FormsResponsesCmd struct {
	List FormsResponsesListCmd `cmd:"" name:"list" aliases:"ls" help:"List form responses"`
	Get  FormsResponseGetCmd   `cmd:"" name:"get" aliases:"info,show" help:"Get a form response"`
}
