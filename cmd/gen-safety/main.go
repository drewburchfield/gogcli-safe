// gen-safety reads a safety-profile.yaml and generates Go source files
// that define parent command structs with only the enabled subcommands.
//
// Generated files get a //go:build safety_profile constraint so they
// replace the original structs when built with -tags safety_profile.
//
// Usage:
//
//	go run ./cmd/gen-safety [profile.yaml]
//	go run ./cmd/gen-safety safety-profiles/readonly.yaml
package main

import (
	"bytes"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// field represents a Kong command struct field.
type field struct {
	GoName  string // e.g. "Send"
	GoType  string // e.g. "GmailSendCmd"
	Tag     string // full struct tag string
	YAMLKey string // key in safety-profile.yaml
}

// serviceSpec defines a top-level service and how to generate its parent struct.
type serviceSpec struct {
	YAMLKey     string  // key in safety-profile.yaml
	StructName  string  // Go struct name (e.g. "GmailCmd")
	File        string  // output file (e.g. "gmail_cmd_gen.go")
	Fields      []field // all possible subcommand fields
	Imports     []string
	ExtraCode    string // extra code to include (e.g. var declarations)
	NonCmdPrefix string // literal field lines for non-command fields (e.g. KeepCmd flags)
}

// warnings accumulates non-fatal issues found during generation.
// All warnings are printed at the end; with --strict they become fatal.
var warnings []string

func warn(msg string, args ...any) {
	w := fmt.Sprintf(msg, args...)
	warnings = append(warnings, w)
}

func main() {
	profilePath := "safety-profile.yaml"
	strict := false
	for _, arg := range os.Args[1:] {
		if arg == "--strict" {
			strict = true
		} else if !strings.HasPrefix(arg, "-") {
			profilePath = arg
		}
	}

	data, err := os.ReadFile(profilePath)
	if err != nil {
		fatal("reading profile: %v", err)
	}

	var profile map[string]any
	if err := yaml.Unmarshal(data, &profile); err != nil {
		fatal("parsing YAML: %v", err)
	}

	outputDir := "internal/cmd"

	specs := allServiceSpecs()

	// Validate YAML keys against known specs to catch typos.
	knownKeys := buildKnownKeys(specs)
	validateYAMLKeys(profile, knownKeys, "")

	for _, spec := range specs {
		if err := generateServiceFile(outputDir, spec, profile); err != nil {
			fatal("generating %s: %v", spec.File, err)
		}
	}

	if err := generateCLIFile(outputDir, profile); err != nil {
		fatal("generating cli_cmd_gen.go: %v", err)
	}

	fmt.Printf("Generated %d files in %s/\n", len(specs)+1, outputDir)

	// Print build summary so users can verify their profile
	fmt.Printf("\nSafety profile summary:\n")
	for _, spec := range specs {
		if isServiceDisabled(profile, spec.YAMLKey) {
			fmt.Printf("  %-20s DISABLED (entire service)\n", spec.YAMLKey)
			continue
		}
		svcConfig := resolveDottedSection(profile, spec.YAMLKey)
		enabled := resolveEnabledFields(spec.Fields, svcConfig, profile, spec.YAMLKey)
		disabled := len(spec.Fields) - len(enabled)
		fmt.Printf("  %-20s %d enabled, %d disabled\n", spec.YAMLKey, len(enabled), disabled)
	}

	// Print consolidated warnings
	if len(warnings) > 0 {
		fmt.Fprintf(os.Stderr, "\ngen-safety: %d warning(s):\n", len(warnings))
		for _, w := range warnings {
			fmt.Fprintf(os.Stderr, "  - %s\n", w)
		}
		if strict {
			fatal("aborting due to warnings (remove --strict to allow)")
		}
	}
}

func generateServiceFile(dir string, spec serviceSpec, profile map[string]any) error {
	svcConfig := resolveDottedSection(profile, spec.YAMLKey)

	// If the entire service is disabled (set to false at the top level), generate an empty struct
	if isServiceDisabled(profile, spec.YAMLKey) {
		return writeGenFile(dir, spec.File, buildEmptyStruct(spec))
	}

	// If service is set to `true` (bool shorthand), include ALL fields.
	// resolveDottedSection returns nil for bools, so we check explicitly.
	enabledFields := resolveEnabledFields(spec.Fields, svcConfig, profile, spec.YAMLKey)

	var buf bytes.Buffer
	buf.WriteString(genHeader)

	if len(spec.Imports) > 0 {
		buf.WriteString("import (\n")
		for _, imp := range spec.Imports {
			fmt.Fprintf(&buf, "\t%q\n", imp)
		}
		buf.WriteString(")\n\n")
	}

	if spec.ExtraCode != "" {
		buf.WriteString(spec.ExtraCode)
		buf.WriteString("\n\n")
	}

	fmt.Fprintf(&buf, "type %s struct {\n", spec.StructName)

	if spec.NonCmdPrefix != "" {
		buf.WriteString(spec.NonCmdPrefix)
		buf.WriteString("\n")
	}

	for _, f := range enabledFields {
		writeStructField(&buf, f)
	}
	buf.WriteString("}\n")

	return writeGenFile(dir, spec.File, buf.String())
}

func generateCLIFile(dir string, profile map[string]any) error {
	aliases := resolveDottedSection(profile, "aliases")
	cliFields := cliFieldDefs()

	var buf bytes.Buffer
	buf.WriteString(genHeader)

	buf.WriteString("import \"github.com/alecthomas/kong\"\n\n")

	buf.WriteString("type CLI struct {\n")
	buf.WriteString("\tRootFlags `embed:\"\"`\n\n")
	buf.WriteString("\tVersion kong.VersionFlag `help:\"Print version and exit\"`\n\n")

	// Action-first aliases
	buf.WriteString("\t// Action-first desire paths (agent-friendly shortcuts).\n")
	for _, f := range cliFields {
		if isEnabled(aliases, f.YAMLKey) {
			writeStructField(&buf, f)
		}
	}
	buf.WriteString("\n")

	// Service commands: include services that have at least one enabled command.
	// Fields without a YAMLKey (utility commands) are always included.
	for _, f := range cliServiceFields() {
		if f.YAMLKey != "" && isServiceDisabled(profile, f.YAMLKey) {
			continue
		}
		writeStructField(&buf, f)
	}

	buf.WriteString("}\n")

	return writeGenFile(dir, "cli_cmd_gen.go", buf.String())
}

func buildEmptyStruct(spec serviceSpec) string {
	var buf bytes.Buffer
	buf.WriteString(genHeader)
	if len(spec.Imports) > 0 {
		buf.WriteString("import (\n")
		for _, imp := range spec.Imports {
			fmt.Fprintf(&buf, "\t%q\n", imp)
		}
		buf.WriteString(")\n\n")
	}
	if spec.ExtraCode != "" {
		buf.WriteString(spec.ExtraCode)
		buf.WriteString("\n\n")
	}
	fmt.Fprintf(&buf, "type %s struct{}\n", spec.StructName)
	return buf.String()
}

// resolveEnabledFields returns the enabled fields for a spec, handling the
// `service: true` bool shorthand (include all fields) and normal map config.
func resolveEnabledFields(fields []field, svcConfig map[string]any, profile map[string]any, yamlKey string) []field {
	if svcConfig == nil && isServiceEnabledBool(profile, yamlKey) {
		// Bool shorthand: `service: true` means include all fields.
		return fields
	}
	return filterFields(fields, svcConfig)
}

// isServiceEnabledBool checks if a dotted key resolves to a `true` boolean.
func isServiceEnabledBool(config map[string]any, key string) bool {
	parts := strings.Split(key, ".")
	current := config
	for i, part := range parts {
		if current == nil {
			return false
		}
		v, ok := current[part]
		if !ok {
			return false
		}
		if b, ok := v.(bool); ok && i == len(parts)-1 {
			return b
		}
		if m, ok := v.(map[string]any); ok {
			current = m
		} else {
			return false
		}
	}
	return false
}

func filterFields(fields []field, config map[string]any) []field {
	var out []field
	for _, f := range fields {
		if isEnabled(config, f.YAMLKey) {
			out = append(out, f)
		}
	}
	return out
}

func isEnabled(config map[string]any, key string) bool {
	if config == nil {
		// Fail-closed: if no config section exists, disable commands.
		// This prevents new upstream commands from silently appearing in
		// safety-profiled builds when the YAML doesn't mention them.
		warn("no config section for key %q, EXCLUDING (fail-closed)", key)
		return false
	}
	v, ok := config[key]
	if !ok {
		warn("key %q not in profile, EXCLUDING (fail-closed)", key)
		return false
	}
	switch val := v.(type) {
	case bool:
		return val
	case map[string]any:
		// Recursively check: enabled if at least one leaf bool is true.
		return mapHasEnabledLeaf(val)
	default:
		fatal("invalid value for key %q: got %T (%v), expected bool or map", key, v, v)
		return false // unreachable
	}
}

// mapHasEnabledLeaf recursively checks whether a nested map contains
// at least one boolean leaf set to true.
func mapHasEnabledLeaf(m map[string]any) bool {
	for _, v := range m {
		switch val := v.(type) {
		case bool:
			if val {
				return true
			}
		case map[string]any:
			if mapHasEnabledLeaf(val) {
				return true
			}
		}
	}
	return false
}

// buildKnownKeys constructs a set of all valid YAML key paths from the specs.
func buildKnownKeys(specs []serviceSpec) map[string]bool {
	known := make(map[string]bool)
	// Top-level sections recognized by the generator
	known["aliases"] = true
	// Utility commands (always included, YAML keys ignored but tolerated)
	known["config"] = true
	known["time"] = true
	known["open"] = true
	for _, spec := range specs {
		addSpecKeys(known, spec.YAMLKey, spec.Fields)
	}
	// Alias sub-keys
	for _, f := range cliFieldDefs() {
		known["aliases."+f.YAMLKey] = true
	}
	return known
}

func addSpecKeys(known map[string]bool, prefix string, fields []field) {
	known[prefix] = true
	for _, f := range fields {
		known[prefix+"."+f.YAMLKey] = true
	}
}

// validateYAMLKeys walks the YAML tree and warns about any keys not in the known set.
func validateYAMLKeys(config map[string]any, known map[string]bool, prefix string) {
	for key, val := range config {
		fullKey := key
		if prefix != "" {
			fullKey = prefix + "." + key
		}
		if !known[fullKey] {
			warn("unrecognized key %q in profile (typo?)", fullKey)
		}
		if m, ok := val.(map[string]any); ok {
			validateYAMLKeys(m, known, fullKey)
		}
	}
}

// resolveDottedSection resolves a dotted key path like "gmail.settings"
// by walking the YAML tree one level at a time.
func resolveDottedSection(config map[string]any, key string) map[string]any {
	parts := strings.Split(key, ".")
	current := config
	for _, part := range parts {
		if current == nil {
			return nil
		}
		v, ok := current[part]
		if !ok {
			return nil
		}
		if m, ok := v.(map[string]any); ok {
			current = m
		} else {
			return nil
		}
	}
	return current
}

// isServiceDisabled checks if a dotted key path resolves to `false` at any level.
// Returns true (disabled) when the key is missing, matching fail-closed semantics.
func isServiceDisabled(config map[string]any, key string) bool {
	parts := strings.Split(key, ".")
	current := config
	for _, part := range parts {
		if current == nil {
			return true // missing section = disabled (fail-closed)
		}
		v, ok := current[part]
		if !ok {
			return true // missing key = disabled (fail-closed)
		}
		if b, ok := v.(bool); ok {
			return !b
		}
		if m, ok := v.(map[string]any); ok {
			current = m
		} else {
			return true // unexpected type = disabled (fail-closed)
		}
	}
	return false
}

const genHeader = "//go:build safety_profile\n\npackage cmd\n\n"

func writeStructField(buf *bytes.Buffer, f field) {
	fmt.Fprintf(buf, "\t%s %s %s\n", f.GoName, f.GoType, f.Tag)
}

func writeGenFile(dir, filename, content string) error {
	formatted, err := format.Source([]byte(content))
	if err != nil {
		// Write unformatted so we can debug
		path := filepath.Join(dir, filename)
		if writeErr := os.WriteFile(path, []byte(content), 0o644); writeErr != nil {
			return fmt.Errorf("formatting %s: %w (also failed to write debug file: %v)", filename, err, writeErr)
		}
		return fmt.Errorf("formatting %s: %w\n\nUnformatted content written to %s for debugging.", filename, err, path)
	}
	path := filepath.Join(dir, filename)
	return os.WriteFile(path, formatted, 0o644)
}

func fatal(msg string, args ...any) {
	fmt.Fprintf(os.Stderr, "gen-safety: "+msg+"\n", args...)
	os.Exit(1)
}

// Below: all the field definitions mapping YAML keys to Go struct fields.
// This is the "registry" that maps the safety profile to the code.

func allServiceSpecs() []serviceSpec {
	return []serviceSpec{
		gmailSpec(),
		gmailSettingsSpec(),
		calendarSpec(),
		driveSpec(),
		driveCommentsSpec(),
		contactsSpec(),
		contactsDirectorySpec(),
		contactsOtherSpec(),
		tasksSpec(),
		tasksListsSpec(),
		docsSpec(),
		docsCommentsSpec(),
		sheetsSpec(),
		slidesSpec(),
		chatSpec(),
		chatSpacesSpec(),
		chatMessagesSpec(),
		chatDMSpec(),
		chatThreadsSpec(),
		formsSpec(),
		formsResponsesSpec(),
		appscriptSpec(),
		classroomSpec(),
		peopleSpec(),
		groupsSpec(),
		keepSpec(),
		authSpec(),
		authCredentialsSpec(),
		authTokensSpec(),
		authAliasSpec(),
		authServiceAccountSpec(),
		gmailThreadSpec(),
		gmailDraftsSpec(),
		gmailLabelsSpec(),
		gmailBatchSpec(),
	}
}

func gmailSpec() serviceSpec {
	return serviceSpec{
		YAMLKey:    "gmail",
		StructName: "GmailCmd",
		File:       "gmail_cmd_gen.go",
		ExtraCode:  "var newGmailService = googleapi.NewGmail",
		Imports:    []string{"github.com/steipete/gogcli/internal/googleapi"},
		Fields: []field{
			{GoName: "Search", GoType: "GmailSearchCmd", Tag: "`cmd:\"\" name:\"search\" aliases:\"find,query,ls,list\" group:\"Read\" help:\"Search threads using Gmail query syntax\"`", YAMLKey: "search"},
			{GoName: "Messages", GoType: "GmailMessagesCmd", Tag: "`cmd:\"\" name:\"messages\" aliases:\"message,msg,msgs\" group:\"Read\" help:\"Message operations\"`", YAMLKey: "messages"},
			{GoName: "Thread", GoType: "GmailThreadCmd", Tag: "`cmd:\"\" name:\"thread\" aliases:\"threads,read\" group:\"Organize\" help:\"Thread operations (get, modify)\"`", YAMLKey: "thread"},
			{GoName: "Get", GoType: "GmailGetCmd", Tag: "`cmd:\"\" name:\"get\" aliases:\"info,show\" group:\"Read\" help:\"Get a message (full|metadata|raw)\"`", YAMLKey: "get"},
			{GoName: "Attachment", GoType: "GmailAttachmentCmd", Tag: "`cmd:\"\" name:\"attachment\" group:\"Read\" help:\"Download a single attachment\"`", YAMLKey: "attachment"},
			{GoName: "URL", GoType: "GmailURLCmd", Tag: "`cmd:\"\" name:\"url\" group:\"Read\" help:\"Print Gmail web URLs for threads\"`", YAMLKey: "url"},
			{GoName: "History", GoType: "GmailHistoryCmd", Tag: "`cmd:\"\" name:\"history\" group:\"Read\" help:\"Gmail history\"`", YAMLKey: "history"},
			{GoName: "Labels", GoType: "GmailLabelsCmd", Tag: "`cmd:\"\" name:\"labels\" aliases:\"label\" group:\"Organize\" help:\"Label operations\"`", YAMLKey: "labels"},
			{GoName: "Batch", GoType: "GmailBatchCmd", Tag: "`cmd:\"\" name:\"batch\" group:\"Organize\" help:\"Batch operations\"`", YAMLKey: "batch"},
			{GoName: "Send", GoType: "GmailSendCmd", Tag: "`cmd:\"\" name:\"send\" group:\"Write\" help:\"Send an email\"`", YAMLKey: "send"},
			{GoName: "Track", GoType: "GmailTrackCmd", Tag: "`cmd:\"\" name:\"track\" group:\"Write\" help:\"Email open tracking\"`", YAMLKey: "track"},
			{GoName: "Drafts", GoType: "GmailDraftsCmd", Tag: "`cmd:\"\" name:\"drafts\" aliases:\"draft\" group:\"Write\" help:\"Draft operations\"`", YAMLKey: "drafts"},
			{GoName: "Settings", GoType: "GmailSettingsCmd", Tag: "`cmd:\"\" name:\"settings\" group:\"Admin\" help:\"Settings and admin\"`", YAMLKey: "settings"},
			{GoName: "Watch", GoType: "GmailWatchCmd", Tag: "`cmd:\"\" name:\"watch\" hidden:\"\" help:\"Manage Gmail watch\"`", YAMLKey: "watch"},
			{GoName: "AutoForward", GoType: "GmailAutoForwardCmd", Tag: "`cmd:\"\" name:\"autoforward\" hidden:\"\" help:\"Auto-forwarding settings\"`", YAMLKey: "autoforward"},
			{GoName: "Delegates", GoType: "GmailDelegatesCmd", Tag: "`cmd:\"\" name:\"delegates\" hidden:\"\" help:\"Delegate operations\"`", YAMLKey: "delegates"},
			{GoName: "Filters", GoType: "GmailFiltersCmd", Tag: "`cmd:\"\" name:\"filters\" hidden:\"\" help:\"Filter operations\"`", YAMLKey: "filters"},
			{GoName: "Forwarding", GoType: "GmailForwardingCmd", Tag: "`cmd:\"\" name:\"forwarding\" hidden:\"\" help:\"Forwarding addresses\"`", YAMLKey: "forwarding"},
			{GoName: "SendAs", GoType: "GmailSendAsCmd", Tag: "`cmd:\"\" name:\"sendas\" hidden:\"\" help:\"Send-as settings\"`", YAMLKey: "sendas"},
			{GoName: "Vacation", GoType: "GmailVacationCmd", Tag: "`cmd:\"\" name:\"vacation\" hidden:\"\" help:\"Vacation responder\"`", YAMLKey: "vacation"},
		},
	}
}

func gmailSettingsSpec() serviceSpec {
	return serviceSpec{
		YAMLKey:    "gmail.settings",
		StructName: "GmailSettingsCmd",
		File:       "gmail_settings_cmd_gen.go",
		Fields: []field{
			{GoName: "Filters", GoType: "GmailFiltersCmd", Tag: "`cmd:\"\" name:\"filters\" group:\"Organize\" help:\"Filter operations\"`", YAMLKey: "filters"},
			{GoName: "Delegates", GoType: "GmailDelegatesCmd", Tag: "`cmd:\"\" name:\"delegates\" group:\"Admin\" help:\"Delegate operations\"`", YAMLKey: "delegates"},
			{GoName: "Forwarding", GoType: "GmailForwardingCmd", Tag: "`cmd:\"\" name:\"forwarding\" group:\"Admin\" help:\"Forwarding addresses\"`", YAMLKey: "forwarding"},
			{GoName: "AutoForward", GoType: "GmailAutoForwardCmd", Tag: "`cmd:\"\" name:\"autoforward\" group:\"Admin\" help:\"Auto-forwarding settings\"`", YAMLKey: "autoforward"},
			{GoName: "SendAs", GoType: "GmailSendAsCmd", Tag: "`cmd:\"\" name:\"sendas\" group:\"Admin\" help:\"Send-as settings\"`", YAMLKey: "sendas"},
			{GoName: "Vacation", GoType: "GmailVacationCmd", Tag: "`cmd:\"\" name:\"vacation\" group:\"Admin\" help:\"Vacation responder\"`", YAMLKey: "vacation"},
			{GoName: "Watch", GoType: "GmailWatchCmd", Tag: "`cmd:\"\" name:\"watch\" group:\"Admin\" help:\"Manage Gmail watch\"`", YAMLKey: "watch"},
		},
	}
}

func gmailThreadSpec() serviceSpec {
	return serviceSpec{
		YAMLKey:    "gmail.thread",
		StructName: "GmailThreadCmd",
		File:       "gmail_thread_cmd_gen.go",
		Fields: []field{
			{GoName: "Get", GoType: "GmailThreadGetCmd", Tag: "`cmd:\"\" name:\"get\" aliases:\"info,show\" default:\"withargs\" help:\"Get a thread with all messages (optionally download attachments)\"`", YAMLKey: "get"},
			{GoName: "Modify", GoType: "GmailThreadModifyCmd", Tag: "`cmd:\"\" name:\"modify\" aliases:\"update,edit,set\" help:\"Modify labels on all messages in a thread\"`", YAMLKey: "modify"},
			{GoName: "Attachments", GoType: "GmailThreadAttachmentsCmd", Tag: "`cmd:\"\" name:\"attachments\" aliases:\"files\" help:\"List all attachments in a thread\"`", YAMLKey: "attachments"},
		},
	}
}

func gmailDraftsSpec() serviceSpec {
	return serviceSpec{
		YAMLKey:    "gmail.drafts",
		StructName: "GmailDraftsCmd",
		File:       "gmail_drafts_cmd_gen.go",
		Fields: []field{
			{GoName: "List", GoType: "GmailDraftsListCmd", Tag: "`cmd:\"\" name:\"list\" aliases:\"ls\" help:\"List drafts\"`", YAMLKey: "list"},
			{GoName: "Get", GoType: "GmailDraftsGetCmd", Tag: "`cmd:\"\" name:\"get\" aliases:\"info,show\" help:\"Get draft details\"`", YAMLKey: "get"},
			{GoName: "Delete", GoType: "GmailDraftsDeleteCmd", Tag: "`cmd:\"\" name:\"delete\" aliases:\"rm,del,remove\" help:\"Delete a draft\"`", YAMLKey: "delete"},
			{GoName: "Send", GoType: "GmailDraftsSendCmd", Tag: "`cmd:\"\" name:\"send\" aliases:\"post\" help:\"Send a draft\"`", YAMLKey: "send"},
			{GoName: "Create", GoType: "GmailDraftsCreateCmd", Tag: "`cmd:\"\" name:\"create\" aliases:\"add,new\" help:\"Create a draft\"`", YAMLKey: "create"},
			{GoName: "Update", GoType: "GmailDraftsUpdateCmd", Tag: "`cmd:\"\" name:\"update\" aliases:\"edit,set\" help:\"Update a draft\"`", YAMLKey: "update"},
		},
	}
}

func gmailLabelsSpec() serviceSpec {
	return serviceSpec{
		YAMLKey:    "gmail.labels",
		StructName: "GmailLabelsCmd",
		File:       "gmail_labels_cmd_gen.go",
		Fields: []field{
			{GoName: "List", GoType: "GmailLabelsListCmd", Tag: "`cmd:\"\" name:\"list\" aliases:\"ls\" help:\"List labels\"`", YAMLKey: "list"},
			{GoName: "Get", GoType: "GmailLabelsGetCmd", Tag: "`cmd:\"\" name:\"get\" aliases:\"info,show\" help:\"Get label details (including counts)\"`", YAMLKey: "get"},
			{GoName: "Create", GoType: "GmailLabelsCreateCmd", Tag: "`cmd:\"\" name:\"create\" aliases:\"add,new\" help:\"Create a new label\"`", YAMLKey: "create"},
			{GoName: "Modify", GoType: "GmailLabelsModifyCmd", Tag: "`cmd:\"\" name:\"modify\" aliases:\"update,edit,set\" help:\"Modify labels on threads\"`", YAMLKey: "modify"},
			{GoName: "Delete", GoType: "GmailLabelsDeleteCmd", Tag: "`cmd:\"\" name:\"delete\" aliases:\"rm,del\" help:\"Delete a label\"`", YAMLKey: "delete"},
		},
	}
}

func gmailBatchSpec() serviceSpec {
	return serviceSpec{
		YAMLKey:    "gmail.batch",
		StructName: "GmailBatchCmd",
		File:       "gmail_batch_cmd_gen.go",
		Fields: []field{
			{GoName: "Delete", GoType: "GmailBatchDeleteCmd", Tag: "`cmd:\"\" name:\"delete\" aliases:\"rm,del,remove\" help:\"Permanently delete multiple messages\"`", YAMLKey: "delete"},
			{GoName: "Modify", GoType: "GmailBatchModifyCmd", Tag: "`cmd:\"\" name:\"modify\" aliases:\"update,edit,set\" help:\"Modify labels on multiple messages\"`", YAMLKey: "modify"},
		},
	}
}

func calendarSpec() serviceSpec {
	return serviceSpec{
		YAMLKey:    "calendar",
		StructName: "CalendarCmd",
		File:       "calendar_cmd_gen.go",
		Fields: []field{
			{GoName: "Calendars", GoType: "CalendarCalendarsCmd", Tag: "`cmd:\"\" name:\"calendars\" help:\"List calendars\"`", YAMLKey: "calendars"},
			{GoName: "ACL", GoType: "CalendarAclCmd", Tag: "`cmd:\"\" name:\"acl\" aliases:\"permissions,perms\" help:\"List calendar ACL\"`", YAMLKey: "acl"},
			{GoName: "Events", GoType: "CalendarEventsCmd", Tag: "`cmd:\"\" name:\"events\" aliases:\"list,ls\" help:\"List events from a calendar or all calendars\"`", YAMLKey: "events"},
			{GoName: "Event", GoType: "CalendarEventCmd", Tag: "`cmd:\"\" name:\"event\" aliases:\"get,info,show\" help:\"Get event\"`", YAMLKey: "event"},
			{GoName: "Create", GoType: "CalendarCreateCmd", Tag: "`cmd:\"\" name:\"create\" aliases:\"add,new\" help:\"Create an event\"`", YAMLKey: "create"},
			{GoName: "Update", GoType: "CalendarUpdateCmd", Tag: "`cmd:\"\" name:\"update\" aliases:\"edit,set\" help:\"Update an event\"`", YAMLKey: "update"},
			{GoName: "Delete", GoType: "CalendarDeleteCmd", Tag: "`cmd:\"\" name:\"delete\" aliases:\"rm,del,remove\" help:\"Delete an event\"`", YAMLKey: "delete"},
			{GoName: "FreeBusy", GoType: "CalendarFreeBusyCmd", Tag: "`cmd:\"\" name:\"freebusy\" help:\"Get free/busy\"`", YAMLKey: "freebusy"},
			{GoName: "Respond", GoType: "CalendarRespondCmd", Tag: "`cmd:\"\" name:\"respond\" aliases:\"rsvp,reply\" help:\"Respond to an event invitation\"`", YAMLKey: "respond"},
			{GoName: "ProposeTime", GoType: "CalendarProposeTimeCmd", Tag: "`cmd:\"\" name:\"propose-time\" help:\"Generate URL to propose a new meeting time (browser-only feature)\"`", YAMLKey: "propose-time"},
			{GoName: "Colors", GoType: "CalendarColorsCmd", Tag: "`cmd:\"\" name:\"colors\" help:\"Show calendar colors\"`", YAMLKey: "colors"},
			{GoName: "Conflicts", GoType: "CalendarConflictsCmd", Tag: "`cmd:\"\" name:\"conflicts\" help:\"Find conflicts\"`", YAMLKey: "conflicts"},
			{GoName: "Search", GoType: "CalendarSearchCmd", Tag: "`cmd:\"\" name:\"search\" aliases:\"find,query\" help:\"Search events\"`", YAMLKey: "search"},
			{GoName: "Time", GoType: "CalendarTimeCmd", Tag: "`cmd:\"\" name:\"time\" help:\"Show server time\"`", YAMLKey: "time"},
			{GoName: "Users", GoType: "CalendarUsersCmd", Tag: "`cmd:\"\" name:\"users\" help:\"List workspace users (use their email as calendar ID)\"`", YAMLKey: "users"},
			{GoName: "Team", GoType: "CalendarTeamCmd", Tag: "`cmd:\"\" name:\"team\" help:\"Show events for all members of a Google Group\"`", YAMLKey: "team"},
			{GoName: "FocusTime", GoType: "CalendarFocusTimeCmd", Tag: "`cmd:\"\" name:\"focus-time\" aliases:\"focus\" help:\"Create a Focus Time block\"`", YAMLKey: "focus-time"},
			{GoName: "OOO", GoType: "CalendarOOOCmd", Tag: "`cmd:\"\" name:\"out-of-office\" aliases:\"ooo\" help:\"Create an Out of Office event\"`", YAMLKey: "out-of-office"},
			{GoName: "WorkingLocation", GoType: "CalendarWorkingLocationCmd", Tag: "`cmd:\"\" name:\"working-location\" aliases:\"wl\" help:\"Set working location (home/office/custom)\"`", YAMLKey: "working-location"},
		},
	}
}

func driveSpec() serviceSpec {
	return serviceSpec{
		YAMLKey:    "drive",
		StructName: "DriveCmd",
		File:       "drive_cmd_gen.go",
		Fields: []field{
			{GoName: "Ls", GoType: "DriveLsCmd", Tag: "`cmd:\"\" name:\"ls\" help:\"List files in a folder (default: root)\"`", YAMLKey: "ls"},
			{GoName: "Search", GoType: "DriveSearchCmd", Tag: "`cmd:\"\" name:\"search\" help:\"Full-text search across Drive\"`", YAMLKey: "search"},
			{GoName: "Get", GoType: "DriveGetCmd", Tag: "`cmd:\"\" name:\"get\" help:\"Get file metadata\"`", YAMLKey: "get"},
			{GoName: "Download", GoType: "DriveDownloadCmd", Tag: "`cmd:\"\" name:\"download\" help:\"Download a file (exports Google Docs formats)\"`", YAMLKey: "download"},
			{GoName: "Copy", GoType: "DriveCopyCmd", Tag: "`cmd:\"\" name:\"copy\" help:\"Copy a file\"`", YAMLKey: "copy"},
			{GoName: "Upload", GoType: "DriveUploadCmd", Tag: "`cmd:\"\" name:\"upload\" help:\"Upload a file\"`", YAMLKey: "upload"},
			{GoName: "Mkdir", GoType: "DriveMkdirCmd", Tag: "`cmd:\"\" name:\"mkdir\" help:\"Create a folder\"`", YAMLKey: "mkdir"},
			{GoName: "Delete", GoType: "DriveDeleteCmd", Tag: "`cmd:\"\" name:\"delete\" help:\"Move a file to trash (use --permanent to delete forever)\" aliases:\"rm,del\"`", YAMLKey: "delete"},
			{GoName: "Move", GoType: "DriveMoveCmd", Tag: "`cmd:\"\" name:\"move\" help:\"Move a file to a different folder\"`", YAMLKey: "move"},
			{GoName: "Rename", GoType: "DriveRenameCmd", Tag: "`cmd:\"\" name:\"rename\" help:\"Rename a file or folder\"`", YAMLKey: "rename"},
			{GoName: "Share", GoType: "DriveShareCmd", Tag: "`cmd:\"\" name:\"share\" help:\"Share a file or folder\"`", YAMLKey: "share"},
			{GoName: "Unshare", GoType: "DriveUnshareCmd", Tag: "`cmd:\"\" name:\"unshare\" help:\"Remove a permission from a file\"`", YAMLKey: "unshare"},
			{GoName: "Permissions", GoType: "DrivePermissionsCmd", Tag: "`cmd:\"\" name:\"permissions\" help:\"List permissions on a file\"`", YAMLKey: "permissions"},
			{GoName: "URL", GoType: "DriveURLCmd", Tag: "`cmd:\"\" name:\"url\" help:\"Print web URLs for files\"`", YAMLKey: "url"},
			{GoName: "Comments", GoType: "DriveCommentsCmd", Tag: "`cmd:\"\" name:\"comments\" help:\"Manage comments on files\"`", YAMLKey: "comments"},
			{GoName: "Drives", GoType: "DriveDrivesCmd", Tag: "`cmd:\"\" name:\"drives\" help:\"List shared drives (Team Drives)\"`", YAMLKey: "drives"},
		},
	}
}

func driveCommentsSpec() serviceSpec {
	return serviceSpec{
		YAMLKey:    "drive.comments",
		StructName: "DriveCommentsCmd",
		File:       "drive_comments_cmd_gen.go",
		Fields: []field{
			{GoName: "List", GoType: "DriveCommentsListCmd", Tag: "`cmd:\"\" name:\"list\" aliases:\"ls\" help:\"List comments on a file\"`", YAMLKey: "list"},
			{GoName: "Get", GoType: "DriveCommentsGetCmd", Tag: "`cmd:\"\" name:\"get\" aliases:\"info,show\" help:\"Get a comment by ID\"`", YAMLKey: "get"},
			{GoName: "Create", GoType: "DriveCommentsCreateCmd", Tag: "`cmd:\"\" name:\"create\" aliases:\"add,new\" help:\"Create a comment on a file\"`", YAMLKey: "create"},
			{GoName: "Update", GoType: "DriveCommentsUpdateCmd", Tag: "`cmd:\"\" name:\"update\" aliases:\"edit,set\" help:\"Update a comment\"`", YAMLKey: "update"},
			{GoName: "Delete", GoType: "DriveCommentsDeleteCmd", Tag: "`cmd:\"\" name:\"delete\" aliases:\"rm,del,remove\" help:\"Delete a comment\"`", YAMLKey: "delete"},
			{GoName: "Reply", GoType: "DriveCommentReplyCmd", Tag: "`cmd:\"\" name:\"reply\" aliases:\"respond\" help:\"Reply to a comment\"`", YAMLKey: "reply"},
		},
	}
}

func contactsSpec() serviceSpec {
	return serviceSpec{
		YAMLKey:    "contacts",
		StructName: "ContactsCmd",
		File:       "contacts_cmd_gen.go",
		Fields: []field{
			{GoName: "Search", GoType: "ContactsSearchCmd", Tag: "`cmd:\"\" name:\"search\" help:\"Search contacts by name/email/phone\"`", YAMLKey: "search"},
			{GoName: "List", GoType: "ContactsListCmd", Tag: "`cmd:\"\" name:\"list\" aliases:\"ls\" help:\"List contacts\"`", YAMLKey: "list"},
			{GoName: "Get", GoType: "ContactsGetCmd", Tag: "`cmd:\"\" name:\"get\" aliases:\"info,show\" help:\"Get a contact\"`", YAMLKey: "get"},
			{GoName: "Create", GoType: "ContactsCreateCmd", Tag: "`cmd:\"\" name:\"create\" aliases:\"add,new\" help:\"Create a contact\"`", YAMLKey: "create"},
			{GoName: "Update", GoType: "ContactsUpdateCmd", Tag: "`cmd:\"\" name:\"update\" aliases:\"edit,set\" help:\"Update a contact\"`", YAMLKey: "update"},
			{GoName: "Delete", GoType: "ContactsDeleteCmd", Tag: "`cmd:\"\" name:\"delete\" aliases:\"rm,del,remove\" help:\"Delete a contact\"`", YAMLKey: "delete"},
			{GoName: "Directory", GoType: "ContactsDirectoryCmd", Tag: "`cmd:\"\" name:\"directory\" help:\"Directory contacts\"`", YAMLKey: "directory"},
			{GoName: "Other", GoType: "ContactsOtherCmd", Tag: "`cmd:\"\" name:\"other\" help:\"Other contacts\"`", YAMLKey: "other"},
		},
	}
}

func contactsDirectorySpec() serviceSpec {
	return serviceSpec{
		YAMLKey:    "contacts.directory",
		StructName: "ContactsDirectoryCmd",
		File:       "contacts_directory_cmd_gen.go",
		Fields: []field{
			{GoName: "List", GoType: "ContactsDirectoryListCmd", Tag: "`cmd:\"\" name:\"list\" help:\"List people from the Workspace directory\"`", YAMLKey: "list"},
			{GoName: "Search", GoType: "ContactsDirectorySearchCmd", Tag: "`cmd:\"\" name:\"search\" help:\"Search people in the Workspace directory\"`", YAMLKey: "search"},
		},
	}
}

func contactsOtherSpec() serviceSpec {
	return serviceSpec{
		YAMLKey:    "contacts.other",
		StructName: "ContactsOtherCmd",
		File:       "contacts_other_cmd_gen.go",
		Fields: []field{
			{GoName: "List", GoType: "ContactsOtherListCmd", Tag: "`cmd:\"\" name:\"list\" help:\"List other contacts\"`", YAMLKey: "list"},
			{GoName: "Search", GoType: "ContactsOtherSearchCmd", Tag: "`cmd:\"\" name:\"search\" help:\"Search other contacts\"`", YAMLKey: "search"},
			{GoName: "Delete", GoType: "ContactsOtherDeleteCmd", Tag: "`cmd:\"\" name:\"delete\" help:\"Delete an other contact\"`", YAMLKey: "delete"},
		},
	}
}

func tasksSpec() serviceSpec {
	return serviceSpec{
		YAMLKey:    "tasks",
		StructName: "TasksCmd",
		File:       "tasks_cmd_gen.go",
		Fields: []field{
			{GoName: "Lists", GoType: "TasksListsCmd", Tag: "`cmd:\"\" name:\"lists\" help:\"List task lists\"`", YAMLKey: "lists"},
			{GoName: "List", GoType: "TasksListCmd", Tag: "`cmd:\"\" name:\"list\" aliases:\"ls\" help:\"List tasks\"`", YAMLKey: "list"},
			{GoName: "Get", GoType: "TasksGetCmd", Tag: "`cmd:\"\" name:\"get\" aliases:\"info,show\" help:\"Get a task\"`", YAMLKey: "get"},
			{GoName: "Add", GoType: "TasksAddCmd", Tag: "`cmd:\"\" name:\"add\" help:\"Add a task\" aliases:\"create\"`", YAMLKey: "add"},
			{GoName: "Update", GoType: "TasksUpdateCmd", Tag: "`cmd:\"\" name:\"update\" aliases:\"edit,set\" help:\"Update a task\"`", YAMLKey: "update"},
			{GoName: "Done", GoType: "TasksDoneCmd", Tag: "`cmd:\"\" name:\"done\" help:\"Mark task completed\" aliases:\"complete\"`", YAMLKey: "done"},
			{GoName: "Undo", GoType: "TasksUndoCmd", Tag: "`cmd:\"\" name:\"undo\" help:\"Mark task needs action\" aliases:\"uncomplete,undone\"`", YAMLKey: "undo"},
			{GoName: "Delete", GoType: "TasksDeleteCmd", Tag: "`cmd:\"\" name:\"delete\" aliases:\"rm,del,remove\" help:\"Delete a task\"`", YAMLKey: "delete"},
			{GoName: "Clear", GoType: "TasksClearCmd", Tag: "`cmd:\"\" name:\"clear\" help:\"Clear completed tasks\"`", YAMLKey: "clear"},
		},
	}
}

func tasksListsSpec() serviceSpec {
	return serviceSpec{
		YAMLKey:    "tasks.lists",
		StructName: "TasksListsCmd",
		File:       "tasks_lists_cmd_gen.go",
		Fields: []field{
			{GoName: "List", GoType: "TasksListsListCmd", Tag: "`cmd:\"\" default:\"withargs\" help:\"List task lists\"`", YAMLKey: "list"},
			{GoName: "Create", GoType: "TasksListsCreateCmd", Tag: "`cmd:\"\" name:\"create\" help:\"Create a task list\" aliases:\"add,new\"`", YAMLKey: "create"},
		},
	}
}

func docsSpec() serviceSpec {
	return serviceSpec{
		YAMLKey:    "docs",
		StructName: "DocsCmd",
		File:       "docs_cmd_gen.go",
		Fields: []field{
			{GoName: "Export", GoType: "DocsExportCmd", Tag: "`cmd:\"\" name:\"export\" aliases:\"download,dl\" help:\"Export a Google Doc (pdf|docx|txt)\"`", YAMLKey: "export"},
			{GoName: "Info", GoType: "DocsInfoCmd", Tag: "`cmd:\"\" name:\"info\" aliases:\"get,show\" help:\"Get Google Doc metadata\"`", YAMLKey: "info"},
			{GoName: "Create", GoType: "DocsCreateCmd", Tag: "`cmd:\"\" name:\"create\" aliases:\"add,new\" help:\"Create a Google Doc\"`", YAMLKey: "create"},
			{GoName: "Copy", GoType: "DocsCopyCmd", Tag: "`cmd:\"\" name:\"copy\" aliases:\"cp,duplicate\" help:\"Copy a Google Doc\"`", YAMLKey: "copy"},
			{GoName: "Cat", GoType: "DocsCatCmd", Tag: "`cmd:\"\" name:\"cat\" aliases:\"text,read\" help:\"Print a Google Doc as plain text\"`", YAMLKey: "cat"},
			{GoName: "Comments", GoType: "DocsCommentsCmd", Tag: "`cmd:\"\" name:\"comments\" help:\"Manage comments on a Google Doc\"`", YAMLKey: "comments"},
			{GoName: "ListTabs", GoType: "DocsListTabsCmd", Tag: "`cmd:\"\" name:\"list-tabs\" help:\"List all tabs in a Google Doc\"`", YAMLKey: "list-tabs"},
			{GoName: "Write", GoType: "DocsWriteCmd", Tag: "`cmd:\"\" name:\"write\" help:\"Write content to a Google Doc\"`", YAMLKey: "write"},
			{GoName: "Insert", GoType: "DocsInsertCmd", Tag: "`cmd:\"\" name:\"insert\" help:\"Insert text at a specific position\"`", YAMLKey: "insert"},
			{GoName: "Delete", GoType: "DocsDeleteCmd", Tag: "`cmd:\"\" name:\"delete\" help:\"Delete text range from document\"`", YAMLKey: "delete"},
			{GoName: "FindReplace", GoType: "DocsFindReplaceCmd", Tag: "`cmd:\"\" name:\"find-replace\" help:\"Find and replace text in document\"`", YAMLKey: "find-replace"},
			{GoName: "Update", GoType: "DocsUpdateCmd", Tag: "`cmd:\"\" name:\"update\" help:\"Update content in a Google Doc\"`", YAMLKey: "update"},
		},
	}
}

func docsCommentsSpec() serviceSpec {
	return serviceSpec{
		YAMLKey:    "docs.comments",
		StructName: "DocsCommentsCmd",
		File:       "docs_comments_cmd_gen.go",
		Fields: []field{
			{GoName: "List", GoType: "DocsCommentsListCmd", Tag: "`cmd:\"\" name:\"list\" aliases:\"ls\" help:\"List comments on a Google Doc\"`", YAMLKey: "list"},
			{GoName: "Get", GoType: "DocsCommentsGetCmd", Tag: "`cmd:\"\" name:\"get\" aliases:\"info,show\" help:\"Get a comment by ID\"`", YAMLKey: "get"},
			{GoName: "Add", GoType: "DocsCommentsAddCmd", Tag: "`cmd:\"\" name:\"add\" aliases:\"create,new\" help:\"Add a comment to a Google Doc\"`", YAMLKey: "add"},
			{GoName: "Reply", GoType: "DocsCommentsReplyCmd", Tag: "`cmd:\"\" name:\"reply\" aliases:\"respond\" help:\"Reply to a comment\"`", YAMLKey: "reply"},
			{GoName: "Resolve", GoType: "DocsCommentsResolveCmd", Tag: "`cmd:\"\" name:\"resolve\" help:\"Resolve a comment (mark as done)\"`", YAMLKey: "resolve"},
			{GoName: "Delete", GoType: "DocsCommentsDeleteCmd", Tag: "`cmd:\"\" name:\"delete\" aliases:\"rm,del,remove\" help:\"Delete a comment\"`", YAMLKey: "delete"},
		},
	}
}

func sheetsSpec() serviceSpec {
	return serviceSpec{
		YAMLKey:    "sheets",
		StructName: "SheetsCmd",
		File:       "sheets_cmd_gen.go",
		Fields: []field{
			{GoName: "Get", GoType: "SheetsGetCmd", Tag: "`cmd:\"\" name:\"get\" aliases:\"read,show\" help:\"Get values from a range\"`", YAMLKey: "get"},
			{GoName: "Update", GoType: "SheetsUpdateCmd", Tag: "`cmd:\"\" name:\"update\" aliases:\"edit,set\" help:\"Update values in a range\"`", YAMLKey: "update"},
			{GoName: "Append", GoType: "SheetsAppendCmd", Tag: "`cmd:\"\" name:\"append\" aliases:\"add\" help:\"Append values to a range\"`", YAMLKey: "append"},
			{GoName: "Insert", GoType: "SheetsInsertCmd", Tag: "`cmd:\"\" name:\"insert\" help:\"Insert empty rows or columns into a sheet\"`", YAMLKey: "insert"},
			{GoName: "Clear", GoType: "SheetsClearCmd", Tag: "`cmd:\"\" name:\"clear\" help:\"Clear values in a range\"`", YAMLKey: "clear"},
			{GoName: "Format", GoType: "SheetsFormatCmd", Tag: "`cmd:\"\" name:\"format\" help:\"Apply cell formatting to a range\"`", YAMLKey: "format"},
			{GoName: "Notes", GoType: "SheetsNotesCmd", Tag: "`cmd:\"\" name:\"notes\" help:\"Get cell notes from a range\"`", YAMLKey: "notes"},
			{GoName: "Metadata", GoType: "SheetsMetadataCmd", Tag: "`cmd:\"\" name:\"metadata\" aliases:\"info\" help:\"Get spreadsheet metadata\"`", YAMLKey: "metadata"},
			{GoName: "Create", GoType: "SheetsCreateCmd", Tag: "`cmd:\"\" name:\"create\" aliases:\"new\" help:\"Create a new spreadsheet\"`", YAMLKey: "create"},
			{GoName: "Copy", GoType: "SheetsCopyCmd", Tag: "`cmd:\"\" name:\"copy\" aliases:\"cp,duplicate\" help:\"Copy a Google Sheet\"`", YAMLKey: "copy"},
			{GoName: "Export", GoType: "SheetsExportCmd", Tag: "`cmd:\"\" name:\"export\" aliases:\"download,dl\" help:\"Export a Google Sheet (pdf|xlsx|csv) via Drive\"`", YAMLKey: "export"},
		},
	}
}

func slidesSpec() serviceSpec {
	return serviceSpec{
		YAMLKey:    "slides",
		StructName: "SlidesCmd",
		File:       "slides_cmd_gen.go",
		Fields: []field{
			{GoName: "Export", GoType: "SlidesExportCmd", Tag: "`cmd:\"\" name:\"export\" aliases:\"download,dl\" help:\"Export a Google Slides deck (pdf|pptx)\"`", YAMLKey: "export"},
			{GoName: "Info", GoType: "SlidesInfoCmd", Tag: "`cmd:\"\" name:\"info\" aliases:\"get,show\" help:\"Get Google Slides presentation metadata\"`", YAMLKey: "info"},
			{GoName: "Create", GoType: "SlidesCreateCmd", Tag: "`cmd:\"\" name:\"create\" aliases:\"add,new\" help:\"Create a Google Slides presentation\"`", YAMLKey: "create"},
			{GoName: "CreateFromMarkdown", GoType: "SlidesCreateFromMarkdownCmd", Tag: "`cmd:\"\" name:\"create-from-markdown\" help:\"Create a Google Slides presentation from markdown\"`", YAMLKey: "create-from-markdown"},
			{GoName: "Copy", GoType: "SlidesCopyCmd", Tag: "`cmd:\"\" name:\"copy\" aliases:\"cp,duplicate\" help:\"Copy a Google Slides presentation\"`", YAMLKey: "copy"},
			{GoName: "AddSlide", GoType: "SlidesAddSlideCmd", Tag: "`cmd:\"\" name:\"add-slide\" help:\"Add a slide with a full-bleed image and optional speaker notes\"`", YAMLKey: "add-slide"},
			{GoName: "ListSlides", GoType: "SlidesListSlidesCmd", Tag: "`cmd:\"\" name:\"list-slides\" help:\"List all slides with their object IDs\"`", YAMLKey: "list-slides"},
			{GoName: "DeleteSlide", GoType: "SlidesDeleteSlideCmd", Tag: "`cmd:\"\" name:\"delete-slide\" help:\"Delete a slide by object ID\"`", YAMLKey: "delete-slide"},
			{GoName: "ReadSlide", GoType: "SlidesReadSlideCmd", Tag: "`cmd:\"\" name:\"read-slide\" help:\"Read slide content: speaker notes, text elements, and images\"`", YAMLKey: "read-slide"},
			{GoName: "UpdateNotes", GoType: "SlidesUpdateNotesCmd", Tag: "`cmd:\"\" name:\"update-notes\" help:\"Update speaker notes on an existing slide\"`", YAMLKey: "update-notes"},
			{GoName: "ReplaceSlide", GoType: "SlidesReplaceSlideCmd", Tag: "`cmd:\"\" name:\"replace-slide\" help:\"Replace the image on an existing slide in-place\"`", YAMLKey: "replace-slide"},
		},
	}
}

func chatSpec() serviceSpec {
	return serviceSpec{
		YAMLKey:    "chat",
		StructName: "ChatCmd",
		File:       "chat_cmd_gen.go",
		Fields: []field{
			{GoName: "Spaces", GoType: "ChatSpacesCmd", Tag: "`cmd:\"\" name:\"spaces\" help:\"Chat spaces\"`", YAMLKey: "spaces"},
			{GoName: "Messages", GoType: "ChatMessagesCmd", Tag: "`cmd:\"\" name:\"messages\" help:\"Chat messages\"`", YAMLKey: "messages"},
			{GoName: "Threads", GoType: "ChatThreadsCmd", Tag: "`cmd:\"\" name:\"threads\" help:\"Chat threads\"`", YAMLKey: "threads"},
			{GoName: "DM", GoType: "ChatDMCmd", Tag: "`cmd:\"\" name:\"dm\" help:\"Direct messages\"`", YAMLKey: "dm"},
		},
	}
}

func chatSpacesSpec() serviceSpec {
	return serviceSpec{
		YAMLKey:    "chat.spaces",
		StructName: "ChatSpacesCmd",
		File:       "chat_spaces_cmd_gen.go",
		Fields: []field{
			{GoName: "List", GoType: "ChatSpacesListCmd", Tag: "`cmd:\"\" name:\"list\" aliases:\"ls\" help:\"List spaces\"`", YAMLKey: "list"},
			{GoName: "Find", GoType: "ChatSpacesFindCmd", Tag: "`cmd:\"\" name:\"find\" aliases:\"search,query\" help:\"Find spaces by display name\"`", YAMLKey: "find"},
			{GoName: "Create", GoType: "ChatSpacesCreateCmd", Tag: "`cmd:\"\" name:\"create\" aliases:\"add,new\" help:\"Create a space\"`", YAMLKey: "create"},
		},
	}
}

func chatMessagesSpec() serviceSpec {
	return serviceSpec{
		YAMLKey:    "chat.messages",
		StructName: "ChatMessagesCmd",
		File:       "chat_messages_cmd_gen.go",
		Fields: []field{
			{GoName: "List", GoType: "ChatMessagesListCmd", Tag: "`cmd:\"\" name:\"list\" aliases:\"ls\" help:\"List messages\"`", YAMLKey: "list"},
			{GoName: "Send", GoType: "ChatMessagesSendCmd", Tag: "`cmd:\"\" name:\"send\" aliases:\"create,post\" help:\"Send a message\"`", YAMLKey: "send"},
		},
	}
}

func chatDMSpec() serviceSpec {
	return serviceSpec{
		YAMLKey:    "chat.dm",
		StructName: "ChatDMCmd",
		File:       "chat_dm_cmd_gen.go",
		Fields: []field{
			{GoName: "Send", GoType: "ChatDMSendCmd", Tag: "`cmd:\"\" name:\"send\" aliases:\"create,post\" help:\"Send a direct message\"`", YAMLKey: "send"},
			{GoName: "Space", GoType: "ChatDMSpaceCmd", Tag: "`cmd:\"\" name:\"space\" aliases:\"find,setup\" help:\"Find or create a DM space\"`", YAMLKey: "space"},
		},
	}
}

func chatThreadsSpec() serviceSpec {
	return serviceSpec{
		YAMLKey:    "chat.threads",
		StructName: "ChatThreadsCmd",
		File:       "chat_threads_cmd_gen.go",
		Fields: []field{
			{GoName: "List", GoType: "ChatThreadsListCmd", Tag: "`cmd:\"\" name:\"list\" help:\"List threads in a space\"`", YAMLKey: "list"},
		},
	}
}

func formsSpec() serviceSpec {
	return serviceSpec{
		YAMLKey:    "forms",
		StructName: "FormsCmd",
		File:       "forms_cmd_gen.go",
		Fields: []field{
			{GoName: "Get", GoType: "FormsGetCmd", Tag: "`cmd:\"\" name:\"get\" aliases:\"info,show\" help:\"Get a form\"`", YAMLKey: "get"},
			{GoName: "Create", GoType: "FormsCreateCmd", Tag: "`cmd:\"\" name:\"create\" aliases:\"new\" help:\"Create a form\"`", YAMLKey: "create"},
			{GoName: "Responses", GoType: "FormsResponsesCmd", Tag: "`cmd:\"\" name:\"responses\" help:\"Form responses\"`", YAMLKey: "responses"},
		},
	}
}

func formsResponsesSpec() serviceSpec {
	return serviceSpec{
		YAMLKey:    "forms.responses",
		StructName: "FormsResponsesCmd",
		File:       "forms_responses_cmd_gen.go",
		Fields: []field{
			{GoName: "List", GoType: "FormsResponsesListCmd", Tag: "`cmd:\"\" name:\"list\" aliases:\"ls\" help:\"List form responses\"`", YAMLKey: "list"},
			{GoName: "Get", GoType: "FormsResponseGetCmd", Tag: "`cmd:\"\" name:\"get\" aliases:\"info,show\" help:\"Get a form response\"`", YAMLKey: "get"},
		},
	}
}

func appscriptSpec() serviceSpec {
	return serviceSpec{
		YAMLKey:    "appscript",
		StructName: "AppScriptCmd",
		File:       "appscript_cmd_gen.go",
		Fields: []field{
			{GoName: "Get", GoType: "AppScriptGetCmd", Tag: "`cmd:\"\" name:\"get\" aliases:\"info,show\" help:\"Get Apps Script project metadata\"`", YAMLKey: "get"},
			{GoName: "Content", GoType: "AppScriptContentCmd", Tag: "`cmd:\"\" name:\"content\" aliases:\"cat\" help:\"Get Apps Script project content\"`", YAMLKey: "content"},
			{GoName: "Run", GoType: "AppScriptRunCmd", Tag: "`cmd:\"\" name:\"run\" help:\"Run a deployed Apps Script function\"`", YAMLKey: "run"},
			{GoName: "Create", GoType: "AppScriptCreateCmd", Tag: "`cmd:\"\" name:\"create\" aliases:\"new\" help:\"Create an Apps Script project\"`", YAMLKey: "create"},
		},
	}
}

func classroomSpec() serviceSpec {
	return serviceSpec{
		YAMLKey:    "classroom",
		StructName: "ClassroomCmd",
		File:       "classroom_cmd_gen.go",
		Fields: []field{
			{GoName: "Courses", GoType: "ClassroomCoursesCmd", Tag: "`cmd:\"\" aliases:\"course\" help:\"Courses\"`", YAMLKey: "courses"},
			{GoName: "Students", GoType: "ClassroomStudentsCmd", Tag: "`cmd:\"\" aliases:\"student\" help:\"Course students\"`", YAMLKey: "students"},
			{GoName: "Teachers", GoType: "ClassroomTeachersCmd", Tag: "`cmd:\"\" aliases:\"teacher\" help:\"Course teachers\"`", YAMLKey: "teachers"},
			{GoName: "Roster", GoType: "ClassroomRosterCmd", Tag: "`cmd:\"\" aliases:\"members\" help:\"Course roster (students + teachers)\"`", YAMLKey: "roster"},
			{GoName: "Coursework", GoType: "ClassroomCourseworkCmd", Tag: "`cmd:\"\" name:\"coursework\" aliases:\"work\" help:\"Coursework\"`", YAMLKey: "coursework"},
			{GoName: "Materials", GoType: "ClassroomMaterialsCmd", Tag: "`cmd:\"\" name:\"materials\" aliases:\"material\" help:\"Coursework materials\"`", YAMLKey: "materials"},
			{GoName: "Submissions", GoType: "ClassroomSubmissionsCmd", Tag: "`cmd:\"\" aliases:\"submission\" help:\"Student submissions\"`", YAMLKey: "submissions"},
			{GoName: "Announcements", GoType: "ClassroomAnnouncementsCmd", Tag: "`cmd:\"\" aliases:\"announcement,ann\" help:\"Announcements\"`", YAMLKey: "announcements"},
			{GoName: "Topics", GoType: "ClassroomTopicsCmd", Tag: "`cmd:\"\" aliases:\"topic\" help:\"Topics\"`", YAMLKey: "topics"},
			{GoName: "Invitations", GoType: "ClassroomInvitationsCmd", Tag: "`cmd:\"\" aliases:\"invitation,invites\" help:\"Invitations\"`", YAMLKey: "invitations"},
			{GoName: "Guardians", GoType: "ClassroomGuardiansCmd", Tag: "`cmd:\"\" aliases:\"guardian\" help:\"Guardians\"`", YAMLKey: "guardians"},
			{GoName: "GuardianInvites", GoType: "ClassroomGuardianInvitesCmd", Tag: "`cmd:\"\" name:\"guardian-invitations\" aliases:\"guardian-invites\" help:\"Guardian invitations\"`", YAMLKey: "guardian-invitations"},
			{GoName: "Profile", GoType: "ClassroomProfileCmd", Tag: "`cmd:\"\" aliases:\"me\" help:\"User profiles\"`", YAMLKey: "profile"},
		},
	}
}

func peopleSpec() serviceSpec {
	return serviceSpec{
		YAMLKey:    "people",
		StructName: "PeopleCmd",
		File:       "people_cmd_gen.go",
		Fields: []field{
			{GoName: "Me", GoType: "PeopleMeCmd", Tag: "`cmd:\"\" name:\"me\" help:\"Show your profile (people/me)\"`", YAMLKey: "me"},
			{GoName: "Get", GoType: "PeopleGetCmd", Tag: "`cmd:\"\" name:\"get\" aliases:\"info,show\" help:\"Get a user profile by ID\"`", YAMLKey: "get"},
			{GoName: "Search", GoType: "PeopleSearchCmd", Tag: "`cmd:\"\" name:\"search\" aliases:\"find,query\" help:\"Search the Workspace directory\"`", YAMLKey: "search"},
			{GoName: "Relations", GoType: "PeopleRelationsCmd", Tag: "`cmd:\"\" name:\"relations\" help:\"Get user relations\"`", YAMLKey: "relations"},
		},
	}
}

func groupsSpec() serviceSpec {
	return serviceSpec{
		YAMLKey:    "groups",
		StructName: "GroupsCmd",
		File:       "groups_cmd_gen.go",
		Fields: []field{
			{GoName: "List", GoType: "GroupsListCmd", Tag: "`cmd:\"\" name:\"list\" aliases:\"ls\" help:\"List groups you belong to\"`", YAMLKey: "list"},
			{GoName: "Members", GoType: "GroupsMembersCmd", Tag: "`cmd:\"\" name:\"members\" help:\"List members of a group\"`", YAMLKey: "members"},
		},
	}
}

func keepSpec() serviceSpec {
	return serviceSpec{
		YAMLKey:    "keep",
		StructName: "KeepCmd",
		File:       "keep_cmd_gen.go",
		NonCmdPrefix: "\tServiceAccount string `name:\"service-account\" help:\"Path to service account JSON file\"`\n" +
			"\tImpersonate    string `name:\"impersonate\" help:\"Email to impersonate (required with service-account)\"`\n",
		Fields: []field{
			{GoName: "List", GoType: "KeepListCmd", Tag: "`cmd:\"\" default:\"withargs\" help:\"List notes\"`", YAMLKey: "list"},
			{GoName: "Get", GoType: "KeepGetCmd", Tag: "`cmd:\"\" name:\"get\" help:\"Get a note\"`", YAMLKey: "get"},
			{GoName: "Search", GoType: "KeepSearchCmd", Tag: "`cmd:\"\" name:\"search\" help:\"Search notes by text (client-side)\"`", YAMLKey: "search"},
			{GoName: "Attachment", GoType: "KeepAttachmentCmd", Tag: "`cmd:\"\" name:\"attachment\" help:\"Download an attachment\"`", YAMLKey: "attachment"},
		},
	}
}

func authSpec() serviceSpec {
	return serviceSpec{
		YAMLKey:    "auth",
		StructName: "AuthCmd",
		File:       "auth_cmd_gen.go",
		Fields: []field{
			{GoName: "Credentials", GoType: "AuthCredentialsCmd", Tag: "`cmd:\"\" name:\"credentials\" help:\"Manage OAuth client credentials\"`", YAMLKey: "credentials"},
			{GoName: "Add", GoType: "AuthAddCmd", Tag: "`cmd:\"\" name:\"add\" help:\"Authorize and store a refresh token\"`", YAMLKey: "add"},
			{GoName: "Services", GoType: "AuthServicesCmd", Tag: "`cmd:\"\" name:\"services\" help:\"List supported auth services and scopes\"`", YAMLKey: "services"},
			{GoName: "List", GoType: "AuthListCmd", Tag: "`cmd:\"\" name:\"list\" help:\"List stored accounts\"`", YAMLKey: "list"},
			{GoName: "Aliases", GoType: "AuthAliasCmd", Tag: "`cmd:\"\" name:\"alias\" help:\"Manage account aliases\"`", YAMLKey: "alias"},
			{GoName: "Status", GoType: "AuthStatusCmd", Tag: "`cmd:\"\" name:\"status\" help:\"Show auth configuration and keyring backend\"`", YAMLKey: "status"},
			{GoName: "Keyring", GoType: "AuthKeyringCmd", Tag: "`cmd:\"\" name:\"keyring\" help:\"Configure keyring backend\"`", YAMLKey: "keyring"},
			{GoName: "Remove", GoType: "AuthRemoveCmd", Tag: "`cmd:\"\" name:\"remove\" help:\"Remove a stored refresh token\"`", YAMLKey: "remove"},
			{GoName: "Tokens", GoType: "AuthTokensCmd", Tag: "`cmd:\"\" name:\"tokens\" help:\"Manage stored refresh tokens\"`", YAMLKey: "tokens"},
			{GoName: "Manage", GoType: "AuthManageCmd", Tag: "`cmd:\"\" name:\"manage\" help:\"Open accounts manager in browser\" aliases:\"login\"`", YAMLKey: "manage"},
			{GoName: "ServiceAcct", GoType: "AuthServiceAccountCmd", Tag: "`cmd:\"\" name:\"service-account\" help:\"Configure service account (Workspace only; domain-wide delegation)\"`", YAMLKey: "service-account"},
			{GoName: "Keep", GoType: "AuthKeepCmd", Tag: "`cmd:\"\" name:\"keep\" help:\"Configure service account for Google Keep (Workspace only)\"`", YAMLKey: "keep"},
		},
	}
}

func authCredentialsSpec() serviceSpec {
	return serviceSpec{
		YAMLKey:    "auth.credentials",
		StructName: "AuthCredentialsCmd",
		File:       "auth_credentials_cmd_gen.go",
		Fields: []field{
			{GoName: "Set", GoType: "AuthCredentialsSetCmd", Tag: "`cmd:\"\" default:\"withargs\" help:\"Store OAuth client credentials\"`", YAMLKey: "set"},
			{GoName: "List", GoType: "AuthCredentialsListCmd", Tag: "`cmd:\"\" name:\"list\" help:\"List stored OAuth client credentials\"`", YAMLKey: "list"},
		},
	}
}

func authTokensSpec() serviceSpec {
	return serviceSpec{
		YAMLKey:    "auth.tokens",
		StructName: "AuthTokensCmd",
		File:       "auth_tokens_cmd_gen.go",
		Fields: []field{
			{GoName: "List", GoType: "AuthTokensListCmd", Tag: "`cmd:\"\" name:\"list\" help:\"List stored tokens (by key only)\"`", YAMLKey: "list"},
			{GoName: "Delete", GoType: "AuthTokensDeleteCmd", Tag: "`cmd:\"\" name:\"delete\" help:\"Delete a stored refresh token\"`", YAMLKey: "delete"},
			{GoName: "Export", GoType: "AuthTokensExportCmd", Tag: "`cmd:\"\" name:\"export\" help:\"Export a refresh token to a file (contains secrets)\"`", YAMLKey: "export"},
			{GoName: "Import", GoType: "AuthTokensImportCmd", Tag: "`cmd:\"\" name:\"import\" help:\"Import a refresh token file into keyring (contains secrets)\"`", YAMLKey: "import"},
		},
	}
}

func authAliasSpec() serviceSpec {
	return serviceSpec{
		YAMLKey:    "auth.alias",
		StructName: "AuthAliasCmd",
		File:       "auth_alias_cmd_gen.go",
		Fields: []field{
			{GoName: "List", GoType: "AuthAliasListCmd", Tag: "`cmd:\"\" name:\"list\" help:\"List account aliases\"`", YAMLKey: "list"},
			{GoName: "Set", GoType: "AuthAliasSetCmd", Tag: "`cmd:\"\" name:\"set\" help:\"Set an account alias\"`", YAMLKey: "set"},
			{GoName: "Unset", GoType: "AuthAliasUnsetCmd", Tag: "`cmd:\"\" name:\"unset\" help:\"Remove an account alias\"`", YAMLKey: "unset"},
		},
	}
}

func authServiceAccountSpec() serviceSpec {
	return serviceSpec{
		YAMLKey:    "auth.service-account",
		StructName: "AuthServiceAccountCmd",
		File:       "auth_service_account_cmd_gen.go",
		Fields: []field{
			{GoName: "Set", GoType: "AuthServiceAccountSetCmd", Tag: "`cmd:\"\" name:\"set\" help:\"Store a service account key for impersonation\"`", YAMLKey: "set"},
			{GoName: "Unset", GoType: "AuthServiceAccountUnsetCmd", Tag: "`cmd:\"\" name:\"unset\" help:\"Remove stored service account key\"`", YAMLKey: "unset"},
			{GoName: "Status", GoType: "AuthServiceAccountStatusCmd", Tag: "`cmd:\"\" name:\"status\" help:\"Show stored service account key status\"`", YAMLKey: "status"},
		},
	}
}

func cliFieldDefs() []field {
	return []field{
		{GoName: "Send", GoType: "GmailSendCmd", Tag: "`cmd:\"\" name:\"send\" help:\"Send an email (alias for 'gmail send')\"`", YAMLKey: "send"},
		{GoName: "Ls", GoType: "DriveLsCmd", Tag: "`cmd:\"\" name:\"ls\" aliases:\"list\" help:\"List Drive files (alias for 'drive ls')\"`", YAMLKey: "ls"},
		{GoName: "Search", GoType: "DriveSearchCmd", Tag: "`cmd:\"\" name:\"search\" aliases:\"find\" help:\"Search Drive files (alias for 'drive search')\"`", YAMLKey: "search"},
		{GoName: "Open", GoType: "OpenCmd", Tag: "`cmd:\"\" name:\"open\" aliases:\"browse\" help:\"Print a best-effort web URL for a Google URL/ID (offline)\"`", YAMLKey: "open"},
		{GoName: "Download", GoType: "DriveDownloadCmd", Tag: "`cmd:\"\" name:\"download\" aliases:\"dl\" help:\"Download a Drive file (alias for 'drive download')\"`", YAMLKey: "download"},
		{GoName: "Upload", GoType: "DriveUploadCmd", Tag: "`cmd:\"\" name:\"upload\" aliases:\"up,put\" help:\"Upload a file to Drive (alias for 'drive upload')\"`", YAMLKey: "upload"},
		{GoName: "Login", GoType: "AuthAddCmd", Tag: "`cmd:\"\" name:\"login\" help:\"Authorize and store a refresh token (alias for 'auth add')\"`", YAMLKey: "login"},
		{GoName: "Logout", GoType: "AuthRemoveCmd", Tag: "`cmd:\"\" name:\"logout\" help:\"Remove a stored refresh token (alias for 'auth remove')\"`", YAMLKey: "logout"},
		{GoName: "Status", GoType: "AuthStatusCmd", Tag: "`cmd:\"\" name:\"status\" aliases:\"st\" help:\"Show auth/config status (alias for 'auth status')\"`", YAMLKey: "status"},
		{GoName: "Me", GoType: "PeopleMeCmd", Tag: "`cmd:\"\" name:\"me\" help:\"Show your profile (alias for 'people me')\"`", YAMLKey: "me"},
		{GoName: "Whoami", GoType: "PeopleMeCmd", Tag: "`cmd:\"\" name:\"whoami\" aliases:\"who-am-i\" help:\"Show your profile (alias for 'people me')\"`", YAMLKey: "whoami"},
	}
}

func cliServiceFields() []field {
	return []field{
		// Service commands: YAMLKey maps to profile section for filtering.
		{GoName: "Auth", GoType: "AuthCmd", Tag: "`cmd:\"\" help:\"Auth and credentials\"`", YAMLKey: "auth"},
		{GoName: "Groups", GoType: "GroupsCmd", Tag: "`cmd:\"\" aliases:\"group\" help:\"Google Groups\"`", YAMLKey: "groups"},
		{GoName: "Drive", GoType: "DriveCmd", Tag: "`cmd:\"\" aliases:\"drv\" help:\"Google Drive\"`", YAMLKey: "drive"},
		{GoName: "Docs", GoType: "DocsCmd", Tag: "`cmd:\"\" aliases:\"doc\" help:\"Google Docs (export via Drive)\"`", YAMLKey: "docs"},
		{GoName: "Slides", GoType: "SlidesCmd", Tag: "`cmd:\"\" aliases:\"slide\" help:\"Google Slides\"`", YAMLKey: "slides"},
		{GoName: "Calendar", GoType: "CalendarCmd", Tag: "`cmd:\"\" aliases:\"cal\" help:\"Google Calendar\"`", YAMLKey: "calendar"},
		{GoName: "Classroom", GoType: "ClassroomCmd", Tag: "`cmd:\"\" aliases:\"class\" help:\"Google Classroom\"`", YAMLKey: "classroom"},
		{GoName: "Gmail", GoType: "GmailCmd", Tag: "`cmd:\"\" aliases:\"mail,email\" help:\"Gmail\"`", YAMLKey: "gmail"},
		{GoName: "Chat", GoType: "ChatCmd", Tag: "`cmd:\"\" help:\"Google Chat\"`", YAMLKey: "chat"},
		{GoName: "Contacts", GoType: "ContactsCmd", Tag: "`cmd:\"\" aliases:\"contact\" help:\"Google Contacts\"`", YAMLKey: "contacts"},
		{GoName: "Tasks", GoType: "TasksCmd", Tag: "`cmd:\"\" aliases:\"task\" help:\"Google Tasks\"`", YAMLKey: "tasks"},
		{GoName: "People", GoType: "PeopleCmd", Tag: "`cmd:\"\" aliases:\"person\" help:\"Google People\"`", YAMLKey: "people"},
		{GoName: "Keep", GoType: "KeepCmd", Tag: "`cmd:\"\" help:\"Google Keep (Workspace only)\"`", YAMLKey: "keep"},
		{GoName: "Sheets", GoType: "SheetsCmd", Tag: "`cmd:\"\" aliases:\"sheet\" help:\"Google Sheets\"`", YAMLKey: "sheets"},
		{GoName: "Forms", GoType: "FormsCmd", Tag: "`cmd:\"\" aliases:\"form\" help:\"Google Forms\"`", YAMLKey: "forms"},
		{GoName: "AppScript", GoType: "AppScriptCmd", Tag: "`cmd:\"\" name:\"appscript\" aliases:\"script,apps-script\" help:\"Google Apps Script\"`", YAMLKey: "appscript"},
		// Utility commands: no YAMLKey, always included.
		{GoName: "Time", GoType: "TimeCmd", Tag: "`cmd:\"\" help:\"Local time utilities\"`"},
		{GoName: "Config", GoType: "ConfigCmd", Tag: "`cmd:\"\" help:\"Manage configuration\"`"},
		{GoName: "ExitCodes", GoType: "AgentExitCodesCmd", Tag: "`cmd:\"\" name:\"exit-codes\" aliases:\"exitcodes\" help:\"Print stable exit codes (alias for 'agent exit-codes')\"`"},
		{GoName: "Agent", GoType: "AgentCmd", Tag: "`cmd:\"\" help:\"Agent-friendly helpers\"`"},
		{GoName: "Schema", GoType: "SchemaCmd", Tag: "`cmd:\"\" help:\"Machine-readable command/flag schema\" aliases:\"help-json,helpjson\"`"},
		{GoName: "VersionCmd", GoType: "VersionCmd", Tag: "`cmd:\"\" name:\"version\" help:\"Print version\"`"},
		{GoName: "Completion", GoType: "CompletionCmd", Tag: "`cmd:\"\" help:\"Generate shell completion scripts\"`"},
		{GoName: "Complete", GoType: "CompletionInternalCmd", Tag: "`cmd:\"\" name:\"__complete\" hidden:\"\" help:\"Internal completion helper\"`"},
	}
}
