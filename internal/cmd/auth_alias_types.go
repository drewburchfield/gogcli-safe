//go:build !safety_profile

package cmd

type AuthAliasCmd struct {
	List  AuthAliasListCmd  `cmd:"" name:"list" help:"List account aliases"`
	Set   AuthAliasSetCmd   `cmd:"" name:"set" help:"Set an account alias"`
	Unset AuthAliasUnsetCmd `cmd:"" name:"unset" help:"Remove an account alias"`
}
