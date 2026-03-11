//go:build !safety_profile

package cmd

type AuthServiceAccountCmd struct {
	Set    AuthServiceAccountSetCmd    `cmd:"" name:"set" help:"Store a service account key for impersonation"`
	Unset  AuthServiceAccountUnsetCmd  `cmd:"" name:"unset" help:"Remove stored service account key"`
	Status AuthServiceAccountStatusCmd `cmd:"" name:"status" help:"Show stored service account key status"`
}
