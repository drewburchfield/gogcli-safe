package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// parseSourceFiles parses all .go files in dir except *_types.go,
// *_cmd_gen.go, and *_test.go, returning parent command structs
// (CLI or *Cmd with at least one cmd:"" field).
func parseSourceFiles(dir string) (map[string]*parsedStruct, error) {
	allFiles, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		return nil, fmt.Errorf("globbing source files: %w", err)
	}

	structs := make(map[string]*parsedStruct)
	fset := token.NewFileSet()

	for _, path := range allFiles {
		base := filepath.Base(path)
		if strings.HasSuffix(base, "_types.go") ||
			strings.HasSuffix(base, "_cmd_gen.go") ||
			strings.HasSuffix(base, "_test.go") {
			continue
		}

		src, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", path, err)
		}

		f, err := parser.ParseFile(fset, path, src, parser.ParseComments)
		if err != nil {
			return nil, fmt.Errorf("parsing %s: %w", path, err)
		}

		imports := extractImports(f)
		extraCode := extractExtraCode(f, fset, src)

		firstCmdAssigned := false

		for _, decl := range f.Decls {
			gd, ok := decl.(*ast.GenDecl)
			if !ok || gd.Tok != token.TYPE {
				continue
			}
			for _, spec := range gd.Specs {
				ts, ok := spec.(*ast.TypeSpec)
				if !ok {
					continue
				}
				st, ok := ts.Type.(*ast.StructType)
				if !ok {
					continue
				}
				name := ts.Name.Name
				if name != "CLI" && !strings.HasSuffix(name, "Cmd") {
					continue
				}
				fields := extractStructFields(st)
				// Only parent command structs (those with cmd:"" fields).
				hasCmd := false
				for _, field := range fields {
					if field.IsCmd {
						hasCmd = true
						break
					}
				}
				if !hasCmd {
					continue
				}

				ps := &parsedStruct{
					Name:       name,
					Fields:     fields,
					SourceFile: base,
				}
				if !firstCmdAssigned {
					ps.Imports = imports
					ps.ExtraCode = extraCode
					firstCmdAssigned = true
				}
				structs[name] = ps
			}
		}
	}

	return structs, nil
}

// verifyTypes checks for two classes of problems:
//
//  1. Duplicate: a parent command struct exists in BOTH a types file and a
//     source file (would cause a compile error).
//  2. Missing service: a CLI struct field references a parent command type
//     that is not a utility and has no types file entry. This catches new
//     services added upstream without a corresponding types file.
//
// Sub-parent structs (like ClassroomCoursesCmd, GmailFiltersCmd) that are
// intentionally kept in source files are not flagged. The generator treats
// them as atomic units, so they don't need types files.
func verifyTypes(dir string) ([]string, error) {
	typesStructs, err := parseTypesFiles(dir)
	if err != nil {
		return nil, err
	}

	sourceStructs, err := parseSourceFiles(dir)
	if err != nil {
		return nil, err
	}

	var violations []string

	// Check 1: no struct should appear in both types and source files.
	for name, sp := range sourceStructs {
		if tp, ok := typesStructs[name]; ok {
			violations = append(violations, fmt.Sprintf(
				"DUPLICATE: %s found in both %s and %s", name, sp.SourceFile, tp.SourceFile))
		}
	}

	// Check 2: CLI struct fields that reference parent command types
	// should have types file entries (unless they're utilities).
	cli, hasCLI := typesStructs["CLI"]
	if hasCLI {
		for _, f := range cli.Fields {
			if !f.IsCmd || utilityTypes[f.GoType] {
				continue
			}
			if _, inTypes := typesStructs[f.GoType]; inTypes {
				continue // already has a types file entry
			}
			if sp, ok := sourceStructs[f.GoType]; ok {
				typesFile := strings.TrimSuffix(sp.SourceFile, ".go") + "_types.go"
				violations = append(violations, fmt.Sprintf(
					"MISSING: %s (CLI service) in %s needs migration to %s",
					f.GoType, sp.SourceFile, typesFile))
			}
		}
	}

	sort.Strings(violations)
	return violations, nil
}

// syncTypes extracts parent command structs from source files into
// *_types.go files with //go:build !safety_profile tags. It only
// migrates structs that are referenced by existing types-file structs
// (or that would be top-level service entries on CLI).
func syncTypes(dir string) error {
	typesStructs, err := parseTypesFiles(dir)
	if err != nil {
		return err
	}

	sourceStructs, err := parseSourceFiles(dir)
	if err != nil {
		return err
	}

	// Find CLI-level service types that need migration to types files.
	needsMigration := make(map[string]*parsedStruct)

	if cli, ok := typesStructs["CLI"]; ok {
		for _, f := range cli.Fields {
			if !f.IsCmd || utilityTypes[f.GoType] {
				continue
			}
			if sp, ok := sourceStructs[f.GoType]; ok {
				if _, inTypes := typesStructs[f.GoType]; !inTypes {
					needsMigration[f.GoType] = sp
				}
			}
		}
	}

	if len(needsMigration) == 0 {
		fmt.Println("All parent command structs are already in *_types.go files. Nothing to sync.")
		return nil
	}

	// Group by source file for combined output.
	byFile := make(map[string][]*parsedStruct)
	for _, ps := range needsMigration {
		byFile[ps.SourceFile] = append(byFile[ps.SourceFile], ps)
	}

	// Sort files for deterministic output.
	files := make([]string, 0, len(byFile))
	for f := range byFile {
		files = append(files, f)
	}
	sort.Strings(files)

	for _, sourceFile := range files {
		psList := byFile[sourceFile]
		typesFile := strings.TrimSuffix(sourceFile, ".go") + "_types.go"
		typesPath := filepath.Join(dir, typesFile)

		// If types file already exists, skip to avoid overwriting manual edits.
		if _, statErr := os.Stat(typesPath); statErr == nil {
			var names []string
			for _, ps := range psList {
				names = append(names, ps.Name)
			}
			fmt.Printf("SKIP: %s already exists (contains: %s)\n", typesFile, strings.Join(names, ", "))
			fmt.Printf("  Manually verify %s matches %s\n", typesFile, sourceFile)
			continue
		}

		var buf bytes.Buffer
		buf.WriteString("//go:build !safety_profile\n\npackage cmd\n\n")

		for i, ps := range psList {
			if i > 0 {
				buf.WriteString("\n")
			}
			fmt.Fprintf(&buf, "type %s struct {\n", ps.Name)
			for _, f := range ps.Fields {
				if f.Tag != "" {
					fmt.Fprintf(&buf, "\t%s %s %s\n", f.GoName, f.GoType, f.Tag)
				} else {
					fmt.Fprintf(&buf, "\t%s %s\n", f.GoName, f.GoType)
				}
			}
			buf.WriteString("}\n")
		}

		formatted, fmtErr := format.Source(buf.Bytes())
		if fmtErr != nil {
			return fmt.Errorf("formatting %s: %w", typesFile, fmtErr)
		}

		if err := os.WriteFile(typesPath, formatted, 0o644); err != nil {
			return fmt.Errorf("writing %s: %w", typesFile, err)
		}

		var names []string
		for _, ps := range psList {
			names = append(names, ps.Name)
		}
		fmt.Printf("Created %s (%s)\n", typesFile, strings.Join(names, ", "))
		fmt.Printf("  ACTION REQUIRED: Remove %s from %s\n", strings.Join(names, ", "), sourceFile)
	}

	return nil
}
