package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestUpdateProfilesWritePath(t *testing.T) {
	// Build a minimal expected key tree: gmail with search, send, drafts (nested).
	expected := newKeyNode()
	gmail := expected.addBranch("gmail")
	gmail.addLeaf("search")
	gmail.addLeaf("send")
	drafts := gmail.addBranch("drafts")
	drafts.addLeaf("create")
	drafts.addLeaf("send")
	expected.addBranch("calendar").addLeaf("events")

	// Create a profile with some keys missing.
	profileYAML := `# Test profile
gmail:
  search: true
  # send is missing
  drafts:
    create: true
    # drafts.send is missing
# calendar section entirely missing
`
	tmpDir := t.TempDir()
	profilePath := filepath.Join(tmpDir, "test.yaml")
	if err := os.WriteFile(profilePath, []byte(profileYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	// Run update (non-dry-run, default false).
	added, err := updateOneProfile(profilePath, expected, false)
	if err != nil {
		t.Fatalf("updateOneProfile: %v", err)
	}

	if added != 3 {
		t.Errorf("expected 3 keys added, got %d", added)
	}

	// Re-read and verify the output.
	data, err := os.ReadFile(profilePath)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)

	// Verify added keys are present.
	if !strings.Contains(content, "send") {
		t.Error("expected 'send' key to be added under gmail")
	}
	if !strings.Contains(content, "calendar") {
		t.Error("expected 'calendar' section to be added")
	}
	if !strings.Contains(content, "events") {
		t.Error("expected 'events' key under calendar")
	}

	// Verify the output is valid YAML that round-trips.
	var parsed map[string]any
	if err := yaml.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("output is not valid YAML: %v", err)
	}

	// Verify existing keys were preserved.
	gmail2, ok := parsed["gmail"].(map[string]any)
	if !ok {
		t.Fatal("gmail section missing or wrong type")
	}
	if gmail2["search"] != true {
		t.Error("existing key gmail.search should still be true")
	}

	// Verify new keys got the right default (false for non-full profiles).
	if gmail2["send"] != false {
		t.Errorf("new key gmail.send should default to false, got %v", gmail2["send"])
	}

	cal, ok := parsed["calendar"].(map[string]any)
	if !ok {
		t.Fatal("calendar section missing or wrong type")
	}
	if cal["events"] != false {
		t.Errorf("new key calendar.events should default to false, got %v", cal["events"])
	}
}

func TestUpdateProfilesMultiChildNested(t *testing.T) {
	// When the last sibling section has multiple children, deepLastLine
	// must walk to the deepest node. Otherwise new keys get inserted
	// inside the sibling section, producing invalid YAML.
	expected := newKeyNode()
	gmail := expected.addBranch("gmail")
	gmail.addLeaf("search")
	gmail.addLeaf("send")
	labels := gmail.addBranch("labels")
	labels.addLeaf("list")
	labels.addLeaf("get")
	labels.addLeaf("create")

	profileYAML := `gmail:
  search: true
  labels:
    list: true
    get: true
    create: true
`
	tmpDir := t.TempDir()
	profilePath := filepath.Join(tmpDir, "test.yaml")
	if err := os.WriteFile(profilePath, []byte(profileYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	added, err := updateOneProfile(profilePath, expected, false)
	if err != nil {
		t.Fatalf("updateOneProfile: %v", err)
	}
	if added != 1 {
		t.Errorf("expected 1 key added (gmail.send), got %d", added)
	}

	data, err := os.ReadFile(profilePath)
	if err != nil {
		t.Fatal(err)
	}

	// Must be valid YAML.
	var parsed map[string]any
	if err := yaml.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("output is not valid YAML: %v\n%s", err, data)
	}

	// gmail.send should exist at the gmail level, not inside labels.
	gmail2 := parsed["gmail"].(map[string]any)
	if gmail2["send"] != false {
		t.Errorf("gmail.send should be false, got %v", gmail2["send"])
	}

	// labels should still have all 3 children.
	labels2 := gmail2["labels"].(map[string]any)
	if len(labels2) != 3 {
		t.Errorf("labels should have 3 keys, got %d: %v", len(labels2), labels2)
	}
}

func TestUpdateProfilesFullDefaultsToTrue(t *testing.T) {
	expected := newKeyNode()
	expected.addBranch("gmail").addLeaf("send")

	profileYAML := "gmail:\n  search: true\n"
	tmpDir := t.TempDir()
	profilePath := filepath.Join(tmpDir, "full.yaml") // "full.yaml" triggers default=true
	if err := os.WriteFile(profilePath, []byte(profileYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	added, err := updateOneProfile(profilePath, expected, false)
	if err != nil {
		t.Fatalf("updateOneProfile: %v", err)
	}
	if added != 1 {
		t.Errorf("expected 1 key added, got %d", added)
	}

	data, err := os.ReadFile(profilePath)
	if err != nil {
		t.Fatal(err)
	}
	var parsed map[string]any
	if err := yaml.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("invalid YAML: %v", err)
	}
	gmail := parsed["gmail"].(map[string]any)
	if gmail["send"] != true {
		t.Errorf("full.yaml should default new keys to true, got %v", gmail["send"])
	}
}

func TestUpdateProfilesBoolShorthandPreserved(t *testing.T) {
	// When a section uses bool shorthand (e.g., "classroom: false"),
	// --update-profiles should leave it alone even if the expected tree
	// has children under that section.
	expected := newKeyNode()
	classroom := expected.addBranch("classroom")
	classroom.addLeaf("courses")
	classroom.addLeaf("roster")

	profileYAML := "classroom: false\n"
	tmpDir := t.TempDir()
	profilePath := filepath.Join(tmpDir, "test.yaml")
	if err := os.WriteFile(profilePath, []byte(profileYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	added, err := updateOneProfile(profilePath, expected, false)
	if err != nil {
		t.Fatalf("updateOneProfile: %v", err)
	}
	if added != 0 {
		t.Errorf("bool shorthand sections should be left alone, but %d keys were added", added)
	}

	// Verify the file is unchanged.
	data, err := os.ReadFile(profilePath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(data)) != "classroom: false" {
		t.Errorf("file should be unchanged, got:\n%s", data)
	}
}

func TestCollectMissingEntireSection(t *testing.T) {
	// When an entire service section is absent, collectMissing should
	// enumerate all its leaf keys with correct dotted paths.
	expected := newKeyNode()
	admin := expected.addBranch("admin")
	admin.addLeaf("users")
	admin.addLeaf("groups")
	expected.addBranch("gmail").addLeaf("search")

	// Profile has gmail but not admin.
	profileYAML := "gmail:\n  search: true\n"
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(profileYAML), &doc); err != nil {
		t.Fatal(err)
	}
	rootMap := doc.Content[0]

	var missing []string
	collectMissing(rootMap, expected, "", &missing)

	if len(missing) != 2 {
		t.Fatalf("expected 2 missing keys, got %d: %v", len(missing), missing)
	}

	found := map[string]bool{}
	for _, k := range missing {
		found[k] = true
	}
	if !found["admin.users"] {
		t.Error("expected admin.users in missing list")
	}
	if !found["admin.groups"] {
		t.Error("expected admin.groups in missing list")
	}
}

func TestDryRunNoSideEffects(t *testing.T) {
	expected := newKeyNode()
	expected.addBranch("gmail").addLeaf("send")

	profileYAML := "gmail:\n  search: true\n"
	tmpDir := t.TempDir()
	profilePath := filepath.Join(tmpDir, "test.yaml")
	if err := os.WriteFile(profilePath, []byte(profileYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	added, err := updateOneProfile(profilePath, expected, true) // dry-run
	if err != nil {
		t.Fatalf("updateOneProfile dry-run: %v", err)
	}
	if added != 1 {
		t.Errorf("dry-run should report 1 key would be added, got %d", added)
	}

	// File should be unchanged.
	data, err := os.ReadFile(profilePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != profileYAML {
		t.Errorf("dry-run should not modify file, got:\n%s", data)
	}
}

func TestUpdateProfilesComprehensive(t *testing.T) {
	// This test exercises all major code paths in a single realistic scenario:
	//   - Multiple existing sections with missing keys (sort by insertAfterLine)
	//   - Multi-child nested section as last sibling (deepLastLine recursion)
	//   - Missing leaf under existing section (basic insertion)
	//   - Missing nested section under existing parent (sectionHeader generation)
	//   - Entirely missing top-level section (appendToEnd)
	//   - Bool shorthand section left untouched
	//   - Comment preservation (text-line insertion, not yaml.Marshal)
	//   - Idempotency (second run adds nothing)
	//   - Round-trip YAML validity

	// Build expected key tree.
	expected := newKeyNode()

	gmail := expected.addBranch("gmail")
	gmail.addLeaf("search")
	gmail.addLeaf("send")
	gmail.addLeaf("archive")
	labels := gmail.addBranch("labels")
	labels.addLeaf("list")
	labels.addLeaf("get")
	labels.addLeaf("rename")

	calendar := expected.addBranch("calendar")
	calendar.addLeaf("events")
	calendar.addLeaf("create")

	drive := expected.addBranch("drive")
	drive.addLeaf("list")
	drive.addLeaf("upload")
	drive.addLeaf("delete")

	// classroom: bool shorthand, should not be expanded
	classroom := expected.addBranch("classroom")
	classroom.addLeaf("courses")
	classroom.addLeaf("roster")

	// tasks: entirely missing section
	tasks := expected.addBranch("tasks")
	tasks.addLeaf("list")
	tasks.addLeaf("create")

	profileYAML := `# Safety profile for comprehensive test
# Tests every insertion path

gmail:
  search: true
  # send and archive are missing
  labels:
    list: true
    get: true
    # rename is missing

calendar:
  events: true
  # create is missing

drive:
  list: true
  # upload and delete are missing

# classroom uses bool shorthand
classroom: false

# tasks section is entirely absent
`

	tmpDir := t.TempDir()
	profilePath := filepath.Join(tmpDir, "test.yaml")
	if err := os.WriteFile(profilePath, []byte(profileYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	// --- First run: add missing keys ---
	added, err := updateOneProfile(profilePath, expected, false)
	if err != nil {
		t.Fatalf("updateOneProfile: %v", err)
	}

	// Expected additions:
	// gmail.send, gmail.archive, gmail.labels.rename,
	// calendar.create, drive.upload, drive.delete,
	// tasks.list, tasks.create
	// classroom is bool shorthand = 0 additions
	if added != 8 {
		t.Errorf("expected 8 keys added, got %d", added)
	}

	data, err := os.ReadFile(profilePath)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)

	// 1. Must be valid YAML.
	var parsed map[string]any
	if err := yaml.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("output is not valid YAML: %v\n%s", err, content)
	}

	// 2. Verify structural correctness of each section.
	gmail2 := parsed["gmail"].(map[string]any)
	if gmail2["search"] != true {
		t.Error("gmail.search should be true (preserved)")
	}
	if gmail2["send"] != false {
		t.Errorf("gmail.send should be false (new), got %v", gmail2["send"])
	}
	if gmail2["archive"] != false {
		t.Errorf("gmail.archive should be false (new), got %v", gmail2["archive"])
	}

	labels2 := gmail2["labels"].(map[string]any)
	if labels2["list"] != true {
		t.Error("gmail.labels.list should be true (preserved)")
	}
	if labels2["get"] != true {
		t.Error("gmail.labels.get should be true (preserved)")
	}
	if labels2["rename"] != false {
		t.Errorf("gmail.labels.rename should be false (new), got %v", labels2["rename"])
	}

	cal := parsed["calendar"].(map[string]any)
	if cal["events"] != true {
		t.Error("calendar.events should be true (preserved)")
	}
	if cal["create"] != false {
		t.Errorf("calendar.create should be false (new), got %v", cal["create"])
	}

	drv := parsed["drive"].(map[string]any)
	if drv["list"] != true {
		t.Error("drive.list should be true (preserved)")
	}
	if drv["upload"] != false {
		t.Errorf("drive.upload should be false (new), got %v", drv["upload"])
	}
	if drv["delete"] != false {
		t.Errorf("drive.delete should be false (new), got %v", drv["delete"])
	}

	// 3. Classroom bool shorthand should be preserved exactly.
	if parsed["classroom"] != false {
		t.Errorf("classroom should remain bool false, got %v (%T)", parsed["classroom"], parsed["classroom"])
	}

	// 4. Tasks section should be fully generated.
	tsk, ok := parsed["tasks"].(map[string]any)
	if !ok {
		t.Fatal("tasks section missing or wrong type")
	}
	if tsk["list"] != false {
		t.Errorf("tasks.list should be false (new), got %v", tsk["list"])
	}
	if tsk["create"] != false {
		t.Errorf("tasks.create should be false (new), got %v", tsk["create"])
	}

	// 5. Comments should be preserved (text-line insertion, not yaml.Marshal).
	if !strings.Contains(content, "# Safety profile for comprehensive test") {
		t.Error("header comment should be preserved")
	}
	if !strings.Contains(content, "# send and archive are missing") {
		t.Error("inline comment should be preserved")
	}
	if !strings.Contains(content, "# classroom uses bool shorthand") {
		t.Error("classroom comment should be preserved")
	}

	// 6. New keys should have # NEW markers.
	if !strings.Contains(content, "send: false # NEW") {
		t.Error("new keys should have # NEW marker")
	}
	if !strings.Contains(content, "tasks: # NEW") {
		t.Error("new section headers should have # NEW marker")
	}

	// --- Second run: idempotency ---
	added2, err := updateOneProfile(profilePath, expected, false)
	if err != nil {
		t.Fatalf("second updateOneProfile: %v", err)
	}
	if added2 != 0 {
		t.Errorf("second run should add 0 keys (idempotent), got %d", added2)
	}

	// File should be byte-identical after second run (no modification).
	data2, err := os.ReadFile(profilePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data2) != content {
		t.Error("second run should not modify the file (idempotency)")
	}
}
