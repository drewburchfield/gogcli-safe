package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestYamlKeyFromTag(t *testing.T) {
	tests := []struct {
		tag    string
		goName string
		want   string
	}{
		{"`cmd:\"\" name:\"send\" help:\"Send an email\"`", "Send", "send"},
		{"`cmd:\"\" name:\"service-account\" help:\"Configure SA\"`", "ServiceAcct", "service-account"},
		{"`cmd:\"\" aliases:\"course\" help:\"Courses\"`", "Courses", "courses"},
		{"`cmd:\"\" name:\"out-of-office\" aliases:\"ooo\" help:\"OOO\"`", "OOO", "out-of-office"},
		{"", "FooBar", "foobar"},
	}
	for _, tt := range tests {
		got := yamlKeyFromTag(tt.tag, tt.goName)
		if got != tt.want {
			t.Errorf("yamlKeyFromTag(%q, %q) = %q, want %q", tt.tag, tt.goName, got, tt.want)
		}
	}
}

func TestHasCmdTag(t *testing.T) {
	if !hasCmdTag("`cmd:\"\" name:\"send\" help:\"Send\"`") {
		t.Error("expected true for cmd tag")
	}
	if hasCmdTag("`name:\"service-account\" help:\"Path\"`") {
		t.Error("expected false for non-cmd tag")
	}
	if hasCmdTag("") {
		t.Error("expected false for empty tag")
	}
}

func TestOutputFileName(t *testing.T) {
	tests := []struct {
		key  string
		want string
	}{
		{"gmail", "gmail_cmd_gen.go"},
		{"gmail.settings", "gmail_settings_cmd_gen.go"},
		{"auth.service-account", "auth_service_account_cmd_gen.go"},
		{"contacts.directory", "contacts_directory_cmd_gen.go"},
	}
	for _, tt := range tests {
		got := outputFileName(tt.key)
		if got != tt.want {
			t.Errorf("outputFileName(%q) = %q, want %q", tt.key, got, tt.want)
		}
	}
}

// TestParseTypesFiles verifies that the AST parser can read the actual
// upstream *_types.go files and produce sensible results.
func TestParseTypesFiles(t *testing.T) {
	dir := filepath.Join("..", "..", "internal", "cmd")
	if _, err := os.Stat(filepath.Join(dir, "root_types.go")); err != nil {
		t.Skipf("types files not found at %s: %v", dir, err)
	}

	structs, err := parseTypesFiles(dir)
	if err != nil {
		t.Fatalf("parseTypesFiles: %v", err)
	}

	// CLI struct must be found.
	cli, ok := structs["CLI"]
	if !ok {
		t.Fatal("CLI struct not found")
	}
	if len(cli.Fields) == 0 {
		t.Fatal("CLI struct has no fields")
	}

	// GmailCmd must exist with expected fields.
	gmail, ok := structs["GmailCmd"]
	if !ok {
		t.Fatal("GmailCmd not found")
	}
	var hasSearch, hasSend bool
	for _, f := range gmail.Fields {
		if f.GoName == "Search" {
			hasSearch = true
		}
		if f.GoName == "Send" {
			hasSend = true
		}
	}
	if !hasSearch || !hasSend {
		t.Errorf("GmailCmd missing expected fields: hasSearch=%v, hasSend=%v", hasSearch, hasSend)
	}
}

// TestMultiStructFile verifies that files with multiple Cmd structs
// (e.g., auth_types.go) are all discovered.
func TestMultiStructFile(t *testing.T) {
	dir := filepath.Join("..", "..", "internal", "cmd")
	if _, err := os.Stat(filepath.Join(dir, "auth_types.go")); err != nil {
		t.Skipf("types files not found: %v", err)
	}

	structs, err := parseTypesFiles(dir)
	if err != nil {
		t.Fatalf("parseTypesFiles: %v", err)
	}

	// auth_types.go has AuthCmd, AuthCredentialsCmd, AuthTokensCmd.
	for _, name := range []string{"AuthCmd", "AuthCredentialsCmd", "AuthTokensCmd"} {
		if _, ok := structs[name]; !ok {
			t.Errorf("struct %s not found (expected from auth_types.go)", name)
		}
	}

	// forms_types.go has FormsCmd, FormsResponsesCmd.
	for _, name := range []string{"FormsCmd", "FormsResponsesCmd"} {
		if _, ok := structs[name]; !ok {
			t.Errorf("struct %s not found (expected from forms_types.go)", name)
		}
	}
}

// TestKeepNonCmdPrefix verifies that KeepCmd's non-cmd fields
// (ServiceAccount, Impersonate) are captured as NonCmdPrefix.
func TestKeepNonCmdPrefix(t *testing.T) {
	dir := filepath.Join("..", "..", "internal", "cmd")
	if _, err := os.Stat(filepath.Join(dir, "keep_types.go")); err != nil {
		t.Skipf("types files not found: %v", err)
	}

	structs, err := parseTypesFiles(dir)
	if err != nil {
		t.Fatalf("parseTypesFiles: %v", err)
	}

	keep, ok := structs["KeepCmd"]
	if !ok {
		t.Fatal("KeepCmd not found")
	}

	prefix := buildNonCmdPrefix(keep.Fields)
	if !strings.Contains(prefix, "ServiceAccount") {
		t.Errorf("NonCmdPrefix should contain ServiceAccount, got: %q", prefix)
	}
	if !strings.Contains(prefix, "Impersonate") {
		t.Errorf("NonCmdPrefix should contain Impersonate, got: %q", prefix)
	}
}

// TestBuildServiceSpecs verifies the full pipeline produces specs
// for known services.
func TestBuildServiceSpecs(t *testing.T) {
	dir := filepath.Join("..", "..", "internal", "cmd")
	if _, err := os.Stat(filepath.Join(dir, "root_types.go")); err != nil {
		t.Skipf("types files not found: %v", err)
	}

	structs, err := parseTypesFiles(dir)
	if err != nil {
		t.Fatalf("parseTypesFiles: %v", err)
	}

	specs, err := buildServiceSpecs(structs)
	if err != nil {
		t.Fatalf("buildServiceSpecs: %v", err)
	}

	if len(specs) == 0 {
		t.Fatal("no specs generated")
	}

	// Build a map for easier lookup.
	specMap := make(map[string]serviceSpec)
	for _, s := range specs {
		specMap[s.YAMLKey] = s
	}

	// Verify a selection of expected specs.
	expected := []struct {
		key        string
		structName string
		minFields  int
	}{
		{"gmail", "GmailCmd", 10},
		{"gmail.settings", "GmailSettingsCmd", 5},
		{"gmail.thread", "GmailThreadCmd", 2},
		{"calendar", "CalendarCmd", 15},
		{"drive", "DriveCmd", 10},
		{"drive.comments", "DriveCommentsCmd", 4},
		{"auth", "AuthCmd", 10},
		{"auth.service-account", "AuthServiceAccountCmd", 2},
		{"keep", "KeepCmd", 3},
		{"classroom", "ClassroomCmd", 10},
	}

	for _, e := range expected {
		spec, ok := specMap[e.key]
		if !ok {
			t.Errorf("missing spec for key %q", e.key)
			continue
		}
		if spec.StructName != e.structName {
			t.Errorf("spec %q: struct name = %q, want %q", e.key, spec.StructName, e.structName)
		}
		if len(spec.Fields) < e.minFields {
			t.Errorf("spec %q: got %d fields, want at least %d", e.key, len(spec.Fields), e.minFields)
		}
	}
}

// TestBuildCLIFields verifies that CLI fields are categorized correctly.
func TestBuildCLIFields(t *testing.T) {
	dir := filepath.Join("..", "..", "internal", "cmd")
	if _, err := os.Stat(filepath.Join(dir, "root_types.go")); err != nil {
		t.Skipf("types files not found: %v", err)
	}

	structs, err := parseTypesFiles(dir)
	if err != nil {
		t.Fatalf("parseTypesFiles: %v", err)
	}

	aliases, services := buildCLIFields(structs)

	if len(aliases) == 0 {
		t.Error("expected at least one alias")
	}
	if len(services) == 0 {
		t.Error("expected at least one service")
	}

	// Aliases should include Send (GmailSendCmd is a leaf, not a parent).
	var hasSendAlias bool
	for _, a := range aliases {
		if a.GoName == "Send" {
			hasSendAlias = true
		}
	}
	if !hasSendAlias {
		t.Error("Send should be classified as an alias")
	}

	// Services should include Gmail (parent Cmd) and Time (utility).
	var hasGmail, hasTime bool
	for _, s := range services {
		if s.GoName == "Gmail" {
			hasGmail = true
			if s.YAMLKey == "" {
				t.Error("Gmail service should have a YAMLKey")
			}
		}
		if s.GoName == "Time" {
			hasTime = true
			if s.YAMLKey != "" {
				t.Error("Time utility should have empty YAMLKey")
			}
		}
	}
	if !hasGmail {
		t.Error("Gmail should be classified as a service")
	}
	if !hasTime {
		t.Error("Time should be classified as a utility (in services list)")
	}
}

// TestGmailWatchSkipped verifies that gmail_watch_types.go (no Cmd structs)
// does not contribute any structs to the map.
func TestGmailWatchSkipped(t *testing.T) {
	dir := filepath.Join("..", "..", "internal", "cmd")
	if _, err := os.Stat(filepath.Join(dir, "gmail_watch_types.go")); err != nil {
		t.Skipf("gmail_watch_types.go not found: %v", err)
	}

	structs, err := parseTypesFiles(dir)
	if err != nil {
		t.Fatalf("parseTypesFiles: %v", err)
	}

	// None of the types in gmail_watch_types.go end in "Cmd".
	for name, ps := range structs {
		if ps.SourceFile == "gmail_watch_types.go" {
			t.Errorf("unexpected struct %q from gmail_watch_types.go", name)
		}
	}
}

// TestClassroomFieldsNoNameTag verifies that fields without a name:"" tag
// (like Classroom's Courses, Students, etc.) get lowercase GoName as YAML key.
func TestClassroomFieldsNoNameTag(t *testing.T) {
	dir := filepath.Join("..", "..", "internal", "cmd")
	structs, err := parseTypesFiles(dir)
	if err != nil {
		t.Fatalf("parseTypesFiles: %v", err)
	}

	specs, err := buildServiceSpecs(structs)
	if err != nil {
		t.Fatalf("buildServiceSpecs: %v", err)
	}

	var classroomSpec *serviceSpec
	for i := range specs {
		if specs[i].YAMLKey == "classroom" {
			classroomSpec = &specs[i]
			break
		}
	}
	if classroomSpec == nil {
		t.Fatal("classroom spec not found")
	}

	// Courses has no name:"" tag, so YAML key should be "courses" (lowercased).
	var found bool
	for _, f := range classroomSpec.Fields {
		if f.GoName == "Courses" {
			found = true
			if f.YAMLKey != "courses" {
				t.Errorf("Courses YAML key = %q, want %q", f.YAMLKey, "courses")
			}
		}
	}
	if !found {
		t.Error("Courses field not found in classroom spec")
	}
}

// TestEndToEndSafeBuild runs the full pipeline: generate from a profile
// and compile with -tags safety_profile. This catches regressions where
// generated code does not compile. Tests both full.yaml (all enabled) and
// readonly.yaml (most disabled) to exercise both code paths.
func TestEndToEndSafeBuild(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping end-to-end build test in short mode")
	}

	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolving repo root: %v", err)
	}

	// Clean up generated files after all subtests complete to avoid
	// leaving side effects in the source tree.
	t.Cleanup(func() {
		files, _ := filepath.Glob(filepath.Join(repoRoot, "internal", "cmd", "*_cmd_gen.go"))
		for _, f := range files {
			os.Remove(f)
		}
	})

	profiles := []string{"full.yaml", "readonly.yaml", "agent-safe.yaml"}
	for _, profile := range profiles {
		t.Run(profile, func(t *testing.T) {
			profilePath := filepath.Join(repoRoot, "safety-profiles", profile)
			if _, err := os.Stat(profilePath); err != nil {
				t.Skipf("%s not found: %v", profile, err)
			}

			// Step 1: Run the generator with --strict.
			gen := exec.Command("go", "run", "./cmd/gen-safety", "--strict", profilePath)
			gen.Dir = repoRoot
			out, err := gen.CombinedOutput()
			if err != nil {
				t.Fatalf("gen-safety --strict %s failed:\n%s\n%v", profile, out, err)
			}

			// Step 2: Build with -tags safety_profile.
			build := exec.Command("go", "build", "-tags", "safety_profile", "./cmd/gog/")
			build.Dir = repoRoot
			out, err = build.CombinedOutput()
			if err != nil {
				t.Fatalf("go build -tags safety_profile (%s) failed:\n%s\n%v", profile, out, err)
			}
		})
	}
}
