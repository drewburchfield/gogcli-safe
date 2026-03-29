package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIsEnabled(t *testing.T) {
	warnings = nil
	warningSet = map[string]bool{}
	defer func() { warnings = nil; warningSet = map[string]bool{} }()

	config := map[string]any{
		"send":   true,
		"delete": false,
		"drafts": map[string]any{
			"create": true,
			"send":   false,
		},
		"settings": map[string]any{
			"filters":    false,
			"forwarding": false,
		},
	}

	tests := []struct {
		key  string
		want bool
	}{
		{"send", true},
		{"delete", false},
		{"drafts", true},    // map with at least one true leaf
		{"settings", false}, // map with all false leaves
		{"missing", false},  // fail-closed: not in config
	}
	for _, tt := range tests {
		got := isEnabledCtx(config, tt.key, "")
		if got != tt.want {
			t.Errorf("isEnabled(config, %q) = %v, want %v", tt.key, got, tt.want)
		}
	}

	// nil config = fail-closed
	if isEnabledCtx(nil, "anything", "") {
		t.Error("isEnabledCtx(nil, ...) should return false")
	}
}

func TestFilterFields(t *testing.T) {
	warnings = nil
	warningSet = map[string]bool{}
	defer func() { warnings = nil; warningSet = map[string]bool{} }()

	fields := []field{
		{GoName: "Send", YAMLKey: "send"},
		{GoName: "Search", YAMLKey: "search"},
		{GoName: "Delete", YAMLKey: "delete"},
	}
	config := map[string]any{
		"send":   false,
		"search": true,
		// "delete" absent = fail-closed (excluded)
	}

	got := filterFields(fields, config, "test")
	if len(got) != 1 || got[0].GoName != "Search" {
		t.Errorf("filterFields: got %v, want [Search]", got)
	}

	// nil config = fail-closed (all excluded)
	got = filterFields(fields, nil, "test")
	if len(got) != 0 {
		t.Errorf("filterFields(nil): got %d fields, want 0", len(got))
	}
}

func TestIsServiceDisabled(t *testing.T) {
	profile := map[string]any{
		"classroom": false,
		"calendar":  true,
		"gmail": map[string]any{
			"send":   true,
			"thread": map[string]any{"get": true},
		},
	}

	tests := []struct {
		key  string
		want bool // true = disabled
	}{
		{"classroom", true},     // explicitly false
		{"calendar", false},     // explicitly true
		{"gmail", false},        // map (not disabled)
		{"gmail.thread", false}, // nested map
		{"nonexistent", true},   // missing = disabled (fail-closed)
	}
	for _, tt := range tests {
		got := isServiceDisabled(profile, tt.key)
		if got != tt.want {
			t.Errorf("isServiceDisabled(profile, %q) = %v, want %v", tt.key, got, tt.want)
		}
	}
}

func TestResolveEnabledFields_BoolShorthand(t *testing.T) {
	profile := map[string]any{"calendar": true}
	fields := []field{
		{GoName: "Events", YAMLKey: "events"},
		{GoName: "Create", YAMLKey: "create"},
	}

	got := resolveEnabledFields(fields, nil, profile, "calendar")
	if len(got) != len(fields) {
		t.Errorf("service: true should include all %d fields, got %d", len(fields), len(got))
	}
}

func TestResolveEnabledFields_NestedBoolShorthand(t *testing.T) {
	// gmail: true should enable all fields in nested parent commands like gmail.settings
	profile := map[string]any{"gmail": true}
	fields := []field{
		{GoName: "Filters", YAMLKey: "filters"},
		{GoName: "Forwarding", YAMLKey: "forwarding"},
	}

	got := resolveEnabledFields(fields, nil, profile, "gmail.settings")
	if len(got) != len(fields) {
		t.Errorf("gmail: true should enable all gmail.settings fields, got %d of %d", len(got), len(fields))
	}
}

func TestValidateYAMLKeys(t *testing.T) {
	warnings = nil
	warningSet = map[string]bool{}
	defer func() { warnings = nil; warningSet = map[string]bool{} }()

	known := map[string]bool{
		"gmail":        true,
		"gmail.send":   true,
		"gmail.search": true,
		"aliases":      true,
		"aliases.ls":   true,
	}

	profile := map[string]any{
		"gmail": map[string]any{
			"send":   true,
			"search": true,
			"typo":   true, // unrecognized
		},
		"bogus": true, // unrecognized top-level
	}

	validateYAMLKeys(profile, known, "")

	if len(warnings) != 2 {
		t.Errorf("expected 2 warnings, got %d: %v", len(warnings), warnings)
	}

	var foundTypo, foundBogus bool
	for _, w := range warnings {
		if strings.Contains(w, "gmail.typo") {
			foundTypo = true
		}
		if strings.Contains(w, "bogus") {
			foundBogus = true
		}
	}
	if !foundTypo {
		t.Error("expected warning about gmail.typo")
	}
	if !foundBogus {
		t.Error("expected warning about bogus")
	}

}

func TestBuildEmptyStruct(t *testing.T) {
	spec := serviceSpec{
		StructName: "ClassroomCmd",
		File:       "classroom_cmd_gen.go",
	}

	got := buildEmptyStruct(spec)
	if !strings.Contains(got, "type ClassroomCmd struct{}") {
		t.Errorf("expected empty struct, got:\n%s", got)
	}
	if !strings.Contains(got, "//go:build safety_profile") {
		t.Error("expected safety_profile build tag")
	}
}

func TestBuildEmptyStructWithNonCmdPrefix(t *testing.T) {
	spec := serviceSpec{
		StructName:   "KeepCmd",
		File:         "keep_cmd_gen.go",
		NonCmdPrefix: "\tServiceAccount string `help:\"SA email\"`\n",
	}

	got := buildEmptyStruct(spec)
	if !strings.Contains(got, "type KeepCmd struct") {
		t.Errorf("expected KeepCmd struct, got:\n%s", got)
	}
	if !strings.Contains(got, "ServiceAccount") {
		t.Error("expected NonCmdPrefix fields preserved in empty struct")
	}
}

func TestMapHasEnabledLeaf(t *testing.T) {
	tests := []struct {
		name string
		m    map[string]any
		want bool
	}{
		{"all false", map[string]any{"a": false, "b": false}, false},
		{"one true", map[string]any{"a": false, "b": true}, true},
		{"nested true", map[string]any{"a": map[string]any{"b": true}}, true},
		{"nested all false", map[string]any{"a": map[string]any{"b": false}}, false},
		{"empty map", map[string]any{}, false},
	}
	for _, tt := range tests {
		got := mapHasEnabledLeaf(tt.m)
		if got != tt.want {
			t.Errorf("mapHasEnabledLeaf(%s) = %v, want %v", tt.name, got, tt.want)
		}
	}
}

// TestGeneratedOutputExcludesDisabledCommands verifies that disabled commands
// are physically absent from generated service files, not just not compiled.
func TestGeneratedOutputExcludesDisabledCommands(t *testing.T) {
	warnings = nil
	warningSet = map[string]bool{}
	defer func() { warnings = nil; warningSet = map[string]bool{} }()

	spec := serviceSpec{
		StructName: "GmailCmd",
		File:       "test_gmail_cmd_gen.go",
		Fields: []field{
			{GoName: "Search", GoType: "GmailSearchCmd", Tag: "`cmd:\"\" name:\"search\"`", YAMLKey: "search"},
			{GoName: "Send", GoType: "GmailSendCmd", Tag: "`cmd:\"\" name:\"send\"`", YAMLKey: "send"},
			{GoName: "Drafts", GoType: "GmailDraftsCmd", Tag: "`cmd:\"\" name:\"drafts\"`", YAMLKey: "drafts"},
		},
	}

	profile := map[string]any{
		"gmail": map[string]any{
			"search": true,
			"send":   false,
			"drafts": true,
		},
	}

	tmpDir := t.TempDir()
	svcConfig := resolveDottedSection(profile, "gmail")
	enabled := resolveEnabledFields(spec.Fields, svcConfig, profile, "gmail")
	if err := writeServiceFileFromFields(tmpDir, spec, enabled); err != nil {
		t.Fatalf("writeServiceFileFromFields: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(tmpDir, spec.File))
	if err != nil {
		t.Fatalf("reading generated file: %v", err)
	}
	src := string(content)

	// Search and Drafts should be present
	if !strings.Contains(src, "GmailSearchCmd") {
		t.Error("expected GmailSearchCmd in generated output (search: true)")
	}
	if !strings.Contains(src, "GmailDraftsCmd") {
		t.Error("expected GmailDraftsCmd in generated output (drafts: true)")
	}

	// Send should be ABSENT
	if strings.Contains(src, "GmailSendCmd") {
		t.Error("GmailSendCmd should NOT be in generated output (send: false)")
	}
}

// TestGeneratedCLIFileExcludesDisabledAliases verifies that disabled aliases
// (like Send) are absent from the generated CLI struct.
func TestGeneratedCLIFileExcludesDisabledAliases(t *testing.T) {
	warnings = nil
	warningSet = map[string]bool{}
	defer func() { warnings = nil; warningSet = map[string]bool{} }()

	aliases := []field{
		{GoName: "Send", GoType: "GmailSendCmd", Tag: "`cmd:\"\" name:\"send\"`", YAMLKey: "send"},
		{GoName: "Ls", GoType: "DriveLsCmd", Tag: "`cmd:\"\" name:\"ls\"`", YAMLKey: "ls"},
	}
	services := []field{
		{GoName: "Gmail", GoType: "GmailCmd", Tag: "`cmd:\"\" help:\"Gmail\"`", YAMLKey: "gmail"},
		{GoName: "Config", GoType: "ConfigCmd", Tag: "`cmd:\"\" help:\"Config\"`", YAMLKey: ""},
	}

	profile := map[string]any{
		"aliases": map[string]any{
			"send": false,
			"ls":   true,
		},
		"gmail": map[string]any{
			"search": true,
		},
	}

	tmpDir := t.TempDir()
	if err := generateCLIFile(tmpDir, profile, aliases, services); err != nil {
		t.Fatalf("generateCLIFile: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(tmpDir, "cli_cmd_gen.go"))
	if err != nil {
		t.Fatalf("reading generated file: %v", err)
	}
	src := string(content)

	// Ls alias should be present
	if !strings.Contains(src, "DriveLsCmd") {
		t.Error("expected DriveLsCmd in generated output (aliases.ls: true)")
	}

	// Send alias should be ABSENT
	if strings.Contains(src, "GmailSendCmd") {
		t.Error("GmailSendCmd should NOT be in generated CLI output (aliases.send: false)")
	}

	// Gmail service should be present (has enabled leaf)
	if !strings.Contains(src, "GmailCmd") {
		t.Error("expected GmailCmd in generated output (gmail has enabled commands)")
	}

	// Config (utility, no YAMLKey) should always be included
	if !strings.Contains(src, "ConfigCmd") {
		t.Error("expected ConfigCmd in generated output (utility command always included)")
	}
}

// TestWarningDeduplication verifies that identical warnings are not repeated.
func TestWarningDeduplication(t *testing.T) {
	warnings = nil
	warningSet = map[string]bool{}
	defer func() { warnings = nil; warningSet = map[string]bool{} }()

	warn("key %q not in profile", "send")
	warn("key %q not in profile", "send")   // duplicate
	warn("key %q not in profile", "delete") // different

	if len(warnings) != 2 {
		t.Errorf("expected 2 deduplicated warnings, got %d: %v", len(warnings), warnings)
	}
}
