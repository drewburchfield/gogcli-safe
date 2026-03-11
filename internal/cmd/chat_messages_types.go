//go:build !safety_profile

package cmd

type ChatMessagesCmd struct {
	List ChatMessagesListCmd `cmd:"" name:"list" aliases:"ls" help:"List messages"`
	Send ChatMessagesSendCmd `cmd:"" name:"send" aliases:"create,post" help:"Send a message"`
}
