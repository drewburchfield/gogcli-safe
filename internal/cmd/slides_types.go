//go:build !safety_profile

package cmd

type SlidesCmd struct {
	Export             SlidesExportCmd             `cmd:"" name:"export" aliases:"download,dl" help:"Export a Google Slides deck (pdf|pptx)"`
	Info               SlidesInfoCmd               `cmd:"" name:"info" aliases:"get,show" help:"Get Google Slides presentation metadata"`
	Create             SlidesCreateCmd             `cmd:"" name:"create" aliases:"add,new" help:"Create a Google Slides presentation"`
	CreateFromMarkdown SlidesCreateFromMarkdownCmd `cmd:"" name:"create-from-markdown" help:"Create a Google Slides presentation from markdown"`
	Copy               SlidesCopyCmd               `cmd:"" name:"copy" aliases:"cp,duplicate" help:"Copy a Google Slides presentation"`
	AddSlide           SlidesAddSlideCmd           `cmd:"" name:"add-slide" help:"Add a slide with a full-bleed image and optional speaker notes"`
	ListSlides         SlidesListSlidesCmd         `cmd:"" name:"list-slides" help:"List all slides with their object IDs"`
	DeleteSlide        SlidesDeleteSlideCmd        `cmd:"" name:"delete-slide" help:"Delete a slide by object ID"`
	ReadSlide          SlidesReadSlideCmd          `cmd:"" name:"read-slide" help:"Read slide content: speaker notes, text elements, and images"`
	UpdateNotes        SlidesUpdateNotesCmd        `cmd:"" name:"update-notes" help:"Update speaker notes on an existing slide"`
	ReplaceSlide       SlidesReplaceSlideCmd       `cmd:"" name:"replace-slide" help:"Replace the image on an existing slide in-place"`
}
