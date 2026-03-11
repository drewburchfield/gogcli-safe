//go:build !safety_profile

package cmd

type AuthCmd struct {
	Credentials AuthCredentialsCmd    `cmd:"" name:"credentials" help:"Manage OAuth client credentials"`
	Add         AuthAddCmd            `cmd:"" name:"add" help:"Authorize and store a refresh token"`
	Services    AuthServicesCmd       `cmd:"" name:"services" help:"List supported auth services and scopes"`
	List        AuthListCmd           `cmd:"" name:"list" help:"List stored accounts"`
	Aliases     AuthAliasCmd          `cmd:"" name:"alias" help:"Manage account aliases"`
	Status      AuthStatusCmd         `cmd:"" name:"status" help:"Show auth configuration and keyring backend"`
	Keyring     AuthKeyringCmd        `cmd:"" name:"keyring" help:"Configure keyring backend"`
	Remove      AuthRemoveCmd         `cmd:"" name:"remove" help:"Remove a stored refresh token"`
	Tokens      AuthTokensCmd         `cmd:"" name:"tokens" help:"Manage stored refresh tokens"`
	Manage      AuthManageCmd         `cmd:"" name:"manage" help:"Open accounts manager in browser" aliases:"login"`
	ServiceAcct AuthServiceAccountCmd `cmd:"" name:"service-account" help:"Configure service account (Workspace only; domain-wide delegation)"`
	Keep        AuthKeepCmd           `cmd:"" name:"keep" help:"Configure service account for Google Keep (Workspace only)"`
}

type AuthCredentialsCmd struct {
	Set  AuthCredentialsSetCmd  `cmd:"" default:"withargs" help:"Store OAuth client credentials"`
	List AuthCredentialsListCmd `cmd:"" name:"list" help:"List stored OAuth client credentials"`
}

type AuthTokensCmd struct {
	List   AuthTokensListCmd   `cmd:"" name:"list" help:"List stored tokens (by key only)"`
	Delete AuthTokensDeleteCmd `cmd:"" name:"delete" help:"Delete a stored refresh token"`
	Export AuthTokensExportCmd `cmd:"" name:"export" help:"Export a refresh token to a file (contains secrets)"`
	Import AuthTokensImportCmd `cmd:"" name:"import" help:"Import a refresh token file into keyring (contains secrets)"`
}
