//go:build !safety_profile

package cmd

type ChatThreadsCmd struct {
	List ChatThreadsListCmd `cmd:"" name:"list" help:"List threads in a space"`
}
