// extract-types is a one-time setup tool that extracts parent command struct
// definitions from their implementation files into separate _types.go files.
//
// This enables the build-tag-based safety profile system:
//   - *_types.go files have //go:build !safety_profile (original full structs)
//   - *_cmd_gen.go files have //go:build safety_profile (trimmed structs)
//   - Original files keep all helper functions and child command implementations
//
// Usage:
//
//	go run ./cmd/extract-types
package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type extraction struct {
	File        string   // source file path
	StructNames []string // struct names to extract
	OutputFile  string   // _types.go output file
	ExtraCode   string   // extra code that must move with the struct (e.g., var declarations)
}

var structPattern = regexp.MustCompile(`^type\s+(\w+)\s+struct\s*\{`)

func main() {
	baseDir := "internal/cmd"

	extractions := []extraction{
		{File: "gmail.go", StructNames: []string{"GmailCmd", "GmailSettingsCmd"}, OutputFile: "gmail_types.go",
			ExtraCode: "var newGmailService = googleapi.NewGmail"},
		{File: "gmail_thread.go", StructNames: []string{"GmailThreadCmd"}, OutputFile: "gmail_thread_types.go"},
		{File: "gmail_drafts.go", StructNames: []string{"GmailDraftsCmd"}, OutputFile: "gmail_drafts_types.go"},
		{File: "gmail_labels.go", StructNames: []string{"GmailLabelsCmd"}, OutputFile: "gmail_labels_types.go"},
		{File: "gmail_batch.go", StructNames: []string{"GmailBatchCmd"}, OutputFile: "gmail_batch_types.go"},
		{File: "calendar.go", StructNames: []string{"CalendarCmd"}, OutputFile: "calendar_types.go"},
		{File: "drive.go", StructNames: []string{"DriveCmd"}, OutputFile: "drive_types.go"},
		{File: "drive_comments.go", StructNames: []string{"DriveCommentsCmd"}, OutputFile: "drive_comments_types.go"},
		{File: "contacts.go", StructNames: []string{"ContactsCmd"}, OutputFile: "contacts_types.go"},
		{File: "contacts_directory.go", StructNames: []string{"ContactsDirectoryCmd", "ContactsOtherCmd"}, OutputFile: "contacts_directory_types.go"},
		{File: "tasks.go", StructNames: []string{"TasksCmd"}, OutputFile: "tasks_types.go"},
		{File: "tasks_lists.go", StructNames: []string{"TasksListsCmd"}, OutputFile: "tasks_lists_types.go"},
		{File: "docs.go", StructNames: []string{"DocsCmd"}, OutputFile: "docs_types.go"},
		{File: "docs_comments.go", StructNames: []string{"DocsCommentsCmd"}, OutputFile: "docs_comments_types.go"},
		{File: "sheets.go", StructNames: []string{"SheetsCmd"}, OutputFile: "sheets_types.go"},
		{File: "slides.go", StructNames: []string{"SlidesCmd"}, OutputFile: "slides_types.go"},
		{File: "chat.go", StructNames: []string{"ChatCmd"}, OutputFile: "chat_types.go"},
		{File: "chat_spaces.go", StructNames: []string{"ChatSpacesCmd"}, OutputFile: "chat_spaces_types.go"},
		{File: "chat_messages.go", StructNames: []string{"ChatMessagesCmd"}, OutputFile: "chat_messages_types.go"},
		{File: "chat_dm.go", StructNames: []string{"ChatDMCmd"}, OutputFile: "chat_dm_types.go"},
		{File: "chat_threads.go", StructNames: []string{"ChatThreadsCmd"}, OutputFile: "chat_threads_types.go"},
		{File: "forms.go", StructNames: []string{"FormsCmd", "FormsResponsesCmd"}, OutputFile: "forms_types.go"},
		{File: "appscript.go", StructNames: []string{"AppScriptCmd"}, OutputFile: "appscript_types.go"},
		{File: "classroom.go", StructNames: []string{"ClassroomCmd"}, OutputFile: "classroom_types.go"},
		{File: "people.go", StructNames: []string{"PeopleCmd"}, OutputFile: "people_types.go"},
		{File: "groups.go", StructNames: []string{"GroupsCmd"}, OutputFile: "groups_types.go"},
		{File: "keep.go", StructNames: []string{"KeepCmd"}, OutputFile: "keep_types.go"},
		{File: "auth.go", StructNames: []string{"AuthCmd", "AuthCredentialsCmd", "AuthTokensCmd"}, OutputFile: "auth_types.go"},
		{File: "auth_alias.go", StructNames: []string{"AuthAliasCmd"}, OutputFile: "auth_alias_types.go"},
		{File: "auth_service_account.go", StructNames: []string{"AuthServiceAccountCmd"}, OutputFile: "auth_service_account_types.go"},
		{File: "root.go", StructNames: []string{"CLI"}, OutputFile: "root_types.go",
			ExtraCode: "NEEDS_KONG_IMPORT"},
	}

	for _, ext := range extractions {
		srcPath := filepath.Join(baseDir, ext.File)
		outPath := filepath.Join(baseDir, ext.OutputFile)

		fmt.Printf("Processing %s -> %s\n", ext.File, ext.OutputFile)

		if err := processExtraction(srcPath, outPath, ext); err != nil {
			fmt.Fprintf(os.Stderr, "Error processing %s: %v\n", ext.File, err)
			os.Exit(1)
		}
	}

	fmt.Println("\nDone! Verify with: go build ./...")
}

func processExtraction(srcPath, outPath string, ext extraction) error {
	lines, err := readLines(srcPath)
	if err != nil {
		return fmt.Errorf("reading %s: %w", srcPath, err)
	}

	// Find struct boundaries
	type structRange struct {
		name       string
		startLine  int // line index of "type X struct {"
		endLine    int // line index of closing "}"
	}

	var ranges []structRange
	for _, name := range ext.StructNames {
		start, end, found := findStructBounds(lines, name)
		if !found {
			return fmt.Errorf("struct %s not found in %s", name, srcPath)
		}
		ranges = append(ranges, structRange{name: name, startLine: start, endLine: end})
	}

	// Extract the structs into the types file
	var typesContent strings.Builder
	typesContent.WriteString("//go:build !safety_profile\n\n")
	typesContent.WriteString("package cmd\n")

	// Check if we need any imports for the types file
	if ext.ExtraCode != "" {
		// Check if the extra code references any imports
		needsImports := detectNeededImports(lines, ext.ExtraCode)
		if len(needsImports) > 0 {
			typesContent.WriteString("\nimport (\n")
			for _, imp := range needsImports {
				typesContent.WriteString(fmt.Sprintf("\t%s\n", imp))
			}
			typesContent.WriteString(")\n")
		}
		// Only write the extra code if it's actual Go code (not a marker)
		if !strings.HasPrefix(ext.ExtraCode, "NEEDS_") {
			typesContent.WriteString("\n")
			typesContent.WriteString(ext.ExtraCode)
			typesContent.WriteString("\n")
		}
	}

	for _, r := range ranges {
		typesContent.WriteString("\n")
		for i := r.startLine; i <= r.endLine; i++ {
			typesContent.WriteString(lines[i])
			typesContent.WriteString("\n")
		}
	}

	if err := os.WriteFile(outPath, []byte(typesContent.String()), 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", outPath, err)
	}

	// Remove the structs from the original file
	// Also remove the extra code line if present
	skipLines := make(map[int]bool)
	for _, r := range ranges {
		for i := r.startLine; i <= r.endLine; i++ {
			skipLines[i] = true
		}
		// Also skip blank lines immediately before the struct
		for i := r.startLine - 1; i >= 0; i-- {
			if strings.TrimSpace(lines[i]) == "" {
				skipLines[i] = true
			} else {
				break
			}
		}
	}

	// Remove extra code from original if it was moved to types file
	if ext.ExtraCode != "" {
		for i, line := range lines {
			if strings.TrimSpace(line) == strings.TrimSpace(ext.ExtraCode) {
				skipLines[i] = true
				// Also skip blank lines after
				for j := i + 1; j < len(lines); j++ {
					if strings.TrimSpace(lines[j]) == "" {
						skipLines[j] = true
					} else {
						break
					}
				}
				break
			}
		}
	}

	var modifiedContent strings.Builder
	for i, line := range lines {
		if skipLines[i] {
			continue
		}
		modifiedContent.WriteString(line)
		modifiedContent.WriteString("\n")
	}

	// Clean up multiple consecutive blank lines
	cleaned := cleanBlankLines(modifiedContent.String())

	if err := os.WriteFile(srcPath, []byte(cleaned), 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", srcPath, err)
	}

	return nil
}

func findStructBounds(lines []string, name string) (int, int, bool) {
	pattern := fmt.Sprintf(`^type\s+%s\s+struct\s*\{`, regexp.QuoteMeta(name))
	re := regexp.MustCompile(pattern)

	for i, line := range lines {
		if re.MatchString(line) {
			// Find the closing brace
			depth := 0
			for j := i; j < len(lines); j++ {
				depth += strings.Count(lines[j], "{") - strings.Count(lines[j], "}")
				if depth == 0 {
					return i, j, true
				}
			}
			return i, len(lines) - 1, true
		}
	}
	return 0, 0, false
}

func readLines(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var lines []string
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	return lines, scanner.Err()
}

func detectNeededImports(lines []string, extraCode string) []string {
	if strings.Contains(extraCode, "googleapi.") {
		return []string{`"github.com/steipete/gogcli/internal/googleapi"`}
	}
	if extraCode == "NEEDS_KONG_IMPORT" {
		return []string{`"github.com/alecthomas/kong"`}
	}
	return nil
}

func cleanBlankLines(s string) string {
	// Replace 3+ consecutive newlines with 2
	for strings.Contains(s, "\n\n\n") {
		s = strings.ReplaceAll(s, "\n\n\n", "\n\n")
	}
	return s
}
