//go:build !safety_profile

package cmd

type ChatDMCmd struct {
	Send  ChatDMSendCmd  `cmd:"" name:"send" aliases:"create,post" help:"Send a direct message"`
	Space ChatDMSpaceCmd `cmd:"" name:"space" aliases:"find,setup" help:"Find or create a DM space"`
}
