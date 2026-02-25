//go:build !safety_profile

package cmd

import (
	"github.com/alecthomas/kong"
)

type CLI struct {
	RootFlags `embed:""`

	Version kong.VersionFlag `help:"Print version and exit"`

	// Action-first desire paths (agent-friendly shortcuts).
	Send     GmailSendCmd     `cmd:"" name:"send" help:"Send an email (alias for 'gmail send')"`
	Ls       DriveLsCmd       `cmd:"" name:"ls" aliases:"list" help:"List Drive files (alias for 'drive ls')"`
	Search   DriveSearchCmd   `cmd:"" name:"search" aliases:"find" help:"Search Drive files (alias for 'drive search')"`
	Open     OpenCmd          `cmd:"" name:"open" aliases:"browse" help:"Print a best-effort web URL for a Google URL/ID (offline)"`
	Download DriveDownloadCmd `cmd:"" name:"download" aliases:"dl" help:"Download a Drive file (alias for 'drive download')"`
	Upload   DriveUploadCmd   `cmd:"" name:"upload" aliases:"up,put" help:"Upload a file to Drive (alias for 'drive upload')"`
	Login    AuthAddCmd       `cmd:"" name:"login" help:"Authorize and store a refresh token (alias for 'auth add')"`
	Logout   AuthRemoveCmd    `cmd:"" name:"logout" help:"Remove a stored refresh token (alias for 'auth remove')"`
	Status   AuthStatusCmd    `cmd:"" name:"status" aliases:"st" help:"Show auth/config status (alias for 'auth status')"`
	Me       PeopleMeCmd      `cmd:"" name:"me" help:"Show your profile (alias for 'people me')"`
	Whoami   PeopleMeCmd      `cmd:"" name:"whoami" aliases:"who-am-i" help:"Show your profile (alias for 'people me')"`

	Auth       AuthCmd               `cmd:"" help:"Auth and credentials"`
	Groups     GroupsCmd             `cmd:"" aliases:"group" help:"Google Groups"`
	Admin      AdminCmd              `cmd:"" help:"Google Workspace Admin (Directory API) - requires domain-wide delegation"`
	Drive      DriveCmd              `cmd:"" aliases:"drv" help:"Google Drive"`
	Docs       DocsCmd               `cmd:"" aliases:"doc" help:"Google Docs (export via Drive)"`
	Slides     SlidesCmd             `cmd:"" aliases:"slide" help:"Google Slides"`
	Calendar   CalendarCmd           `cmd:"" aliases:"cal" help:"Google Calendar"`
	Classroom  ClassroomCmd          `cmd:"" aliases:"class" help:"Google Classroom"`
	Time       TimeCmd               `cmd:"" help:"Local time utilities"`
	Gmail      GmailCmd              `cmd:"" aliases:"mail,email" help:"Gmail"`
	Chat       ChatCmd               `cmd:"" help:"Google Chat"`
	Contacts   ContactsCmd           `cmd:"" aliases:"contact" help:"Google Contacts"`
	Tasks      TasksCmd              `cmd:"" aliases:"task" help:"Google Tasks"`
	People     PeopleCmd             `cmd:"" aliases:"person" help:"Google People"`
	Keep       KeepCmd               `cmd:"" help:"Google Keep (Workspace only)"`
	Sheets     SheetsCmd             `cmd:"" aliases:"sheet" help:"Google Sheets"`
	Forms      FormsCmd              `cmd:"" aliases:"form" help:"Google Forms"`
	AppScript  AppScriptCmd          `cmd:"" name:"appscript" aliases:"script,apps-script" help:"Google Apps Script"`
	Config     ConfigCmd             `cmd:"" help:"Manage configuration"`
	ExitCodes  AgentExitCodesCmd     `cmd:"" name:"exit-codes" aliases:"exitcodes" help:"Print stable exit codes (alias for 'agent exit-codes')"`
	Agent      AgentCmd              `cmd:"" help:"Agent-friendly helpers"`
	Schema     SchemaCmd             `cmd:"" help:"Machine-readable command/flag schema" aliases:"help-json,helpjson"`
	VersionCmd VersionCmd            `cmd:"" name:"version" help:"Print version"`
	Completion CompletionCmd         `cmd:"" help:"Generate shell completion scripts"`
	Complete   CompletionInternalCmd `cmd:"" name:"__complete" hidden:"" help:"Internal completion helper"`
}
