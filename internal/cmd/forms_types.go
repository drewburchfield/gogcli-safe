//go:build !safety_profile

package cmd

type FormsCmd struct {
	Get            FormsGetCmd            `cmd:"" name:"get" aliases:"info,show" help:"Get a form"`
	Create         FormsCreateCmd         `cmd:"" name:"create" aliases:"new" help:"Create a form"`
	Update         FormsUpdateCmd         `cmd:"" name:"update" aliases:"edit" help:"Update form title, description, or settings"`
	AddQuestion    FormsAddQuestionCmd    `cmd:"" name:"add-question" aliases:"add-q,aq" help:"Add a question to a form"`
	DeleteQuestion FormsDeleteQuestionCmd `cmd:"" name:"delete-question" aliases:"delete-q,dq,rm-q" help:"Delete a question by index"`
	MoveQuestion   FormsMoveQuestionCmd   `cmd:"" name:"move-question" aliases:"move-q,mq" help:"Move a question to a new position"`
	Responses      FormsResponsesCmd      `cmd:"" name:"responses" help:"Form responses"`
	Watch          FormsWatchCmd          `cmd:"" name:"watch" aliases:"watches" help:"Response watches (push notifications)"`
}

type FormsResponsesCmd struct {
	List FormsResponsesListCmd `cmd:"" name:"list" aliases:"ls" help:"List form responses"`
	Get  FormsResponseGetCmd   `cmd:"" name:"get" aliases:"info,show" help:"Get a form response"`
}
