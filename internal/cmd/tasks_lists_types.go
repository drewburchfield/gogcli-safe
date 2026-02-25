//go:build !safety_profile

package cmd

type TasksListsCmd struct {
	List   TasksListsListCmd   `cmd:"" default:"withargs" help:"List task lists"`
	Create TasksListsCreateCmd `cmd:"" name:"create" help:"Create a task list" aliases:"add,new"`
}
