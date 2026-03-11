package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestVerifyTypesClean runs --verify against the current codebase.
// If all parent command structs are properly in *_types.go files,
// this should pass with zero violations.
func TestVerifyTypesClean(t *testing.T) {
	dir := filepath.Join("..", "..", "internal", "cmd")
	if _, err := os.Stat(filepath.Join(dir, "root_types.go")); err != nil {
		t.Skipf("types files not found: %v", err)
	}

	violations, err := verifyTypes(dir)
	if err != nil {
		t.Fatalf("verifyTypes: %v", err)
	}

	for _, v := range violations {
		t.Errorf("unmigrated struct: %s", v)
	}
}

// TestParseSourceFilesExcludesTypes verifies that parseSourceFiles
// does not pick up structs from *_types.go files.
func TestParseSourceFilesExcludesTypes(t *testing.T) {
	dir := filepath.Join("..", "..", "internal", "cmd")
	if _, err := os.Stat(filepath.Join(dir, "root_types.go")); err != nil {
		t.Skipf("types files not found: %v", err)
	}

	sourceStructs, err := parseSourceFiles(dir)
	if err != nil {
		t.Fatalf("parseSourceFiles: %v", err)
	}

	// GmailCmd should be in gmail_types.go, not found in source files.
	if ps, ok := sourceStructs["GmailCmd"]; ok {
		t.Errorf("GmailCmd found in source file %s (should only be in types file)", ps.SourceFile)
	}

	// CLI should be in root_types.go, not found in source files.
	if ps, ok := sourceStructs["CLI"]; ok {
		t.Errorf("CLI found in source file %s (should only be in types file)", ps.SourceFile)
	}
}

// TestSyncTypesNoop verifies that --sync finds nothing to migrate
// when all CLI-level service structs are already in types files.
func TestSyncTypesNoop(t *testing.T) {
	dir := filepath.Join("..", "..", "internal", "cmd")
	if _, err := os.Stat(filepath.Join(dir, "root_types.go")); err != nil {
		t.Skipf("types files not found: %v", err)
	}

	typesStructs, err := parseTypesFiles(dir)
	if err != nil {
		t.Fatalf("parseTypesFiles: %v", err)
	}
	sourceStructs, err := parseSourceFiles(dir)
	if err != nil {
		t.Fatalf("parseSourceFiles: %v", err)
	}

	// Every CLI-level service field type should be in types, not source only.
	cli, ok := typesStructs["CLI"]
	if !ok {
		t.Fatal("CLI not found in types files")
	}
	for _, f := range cli.Fields {
		if !f.IsCmd || utilityTypes[f.GoType] {
			continue
		}
		if _, inTypes := typesStructs[f.GoType]; inTypes {
			continue
		}
		if _, inSource := sourceStructs[f.GoType]; inSource {
			t.Errorf("CLI service %s (%s) is in source but not types files", f.GoName, f.GoType)
		}
	}
}

// TestVerifyEndToEnd runs gen-safety --verify as a subprocess to test
// the CLI integration.
func TestVerifyEndToEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping end-to-end test in short mode")
	}

	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolving repo root: %v", err)
	}

	cmd := exec.Command("go", "run", "./cmd/gen-safety", "--verify")
	cmd.Dir = repoRoot
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("gen-safety --verify failed:\n%s\n%v", out, err)
	}
}
