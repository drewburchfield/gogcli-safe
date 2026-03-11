//go:build !safety_profile

package cmd

type DriveCmd struct {
	Ls          DriveLsCmd          `cmd:"" name:"ls" help:"List files in a folder (default: root)"`
	Search      DriveSearchCmd      `cmd:"" name:"search" help:"Full-text search across Drive"`
	Get         DriveGetCmd         `cmd:"" name:"get" help:"Get file metadata"`
	Download    DriveDownloadCmd    `cmd:"" name:"download" help:"Download a file (exports Google Docs formats)"`
	Copy        DriveCopyCmd        `cmd:"" name:"copy" help:"Copy a file"`
	Upload      DriveUploadCmd      `cmd:"" name:"upload" help:"Upload a file"`
	Mkdir       DriveMkdirCmd       `cmd:"" name:"mkdir" help:"Create a folder"`
	Delete      DriveDeleteCmd      `cmd:"" name:"delete" help:"Move a file to trash (use --permanent to delete forever)" aliases:"rm,del"`
	Move        DriveMoveCmd        `cmd:"" name:"move" help:"Move a file to a different folder"`
	Rename      DriveRenameCmd      `cmd:"" name:"rename" help:"Rename a file or folder"`
	Share       DriveShareCmd       `cmd:"" name:"share" help:"Share a file or folder"`
	Unshare     DriveUnshareCmd     `cmd:"" name:"unshare" help:"Remove a permission from a file"`
	Permissions DrivePermissionsCmd `cmd:"" name:"permissions" help:"List permissions on a file"`
	URL         DriveURLCmd         `cmd:"" name:"url" help:"Print web URLs for files"`
	Comments    DriveCommentsCmd    `cmd:"" name:"comments" help:"Manage comments on files"`
	Drives      DriveDrivesCmd      `cmd:"" name:"drives" help:"List shared drives (Team Drives)"`
}
