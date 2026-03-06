package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"strings"
)

// parsedStruct holds the AST-extracted information for one struct type.
type parsedStruct struct {
	Name       string
	Fields     []parsedField
	SourceFile string
	Imports    []string
	ExtraCode  string
}

// parsedField is a single field extracted from a Go struct via AST.
type parsedField struct {
	GoName string
	GoType string
	Tag    string // full tag including backticks
	IsCmd  bool   // has cmd:"" in the tag
}

// utilityTypes is the set of CLI field types that are always included
// (no YAML key, no filtering).
var utilityTypes = map[string]bool{
	"TimeCmd":               true,
	"ConfigCmd":             true,
	"AgentExitCodesCmd":     true,
	"AgentCmd":              true,
	"SchemaCmd":             true,
	"VersionCmd":            true,
	"CompletionCmd":         true,
	"CompletionInternalCmd": true,
}

// yamlKeyFromTag extracts the name:"" value from a struct tag.
// Falls back to strings.ToLower(goName) if no name tag is present.
func yamlKeyFromTag(rawTag, goName string) string {
	tag := reflect.StructTag(strings.Trim(rawTag, "`"))
	if v, ok := tag.Lookup("name"); ok && v != "" {
		return v
	}
	return strings.ToLower(goName)
}

// hasCmdTag checks whether a struct tag contains cmd:"".
func hasCmdTag(rawTag string) bool {
	tag := reflect.StructTag(strings.Trim(rawTag, "`"))
	_, ok := tag.Lookup("cmd")
	return ok
}

// extractStructFields walks an AST struct type and returns its fields,
// skipping embedded (anonymous) fields.
func extractStructFields(st *ast.StructType) []parsedField {
	var out []parsedField
	for _, f := range st.Fields.List {
		if len(f.Names) == 0 {
			continue // embedded/anonymous
		}
		name := f.Names[0].Name
		typeName := exprToString(f.Type)
		tag := ""
		if f.Tag != nil {
			tag = f.Tag.Value
		}
		out = append(out, parsedField{
			GoName: name,
			GoType: typeName,
			Tag:    tag,
			IsCmd:  tag != "" && hasCmdTag(tag),
		})
	}
	return out
}

// exprToString converts an ast.Expr to its string representation.
func exprToString(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.SelectorExpr:
		return exprToString(e.X) + "." + e.Sel.Name
	case *ast.StarExpr:
		return "*" + exprToString(e.X)
	default:
		fatal("unexpected AST node type %T in struct field type", expr)
		return "" // unreachable
	}
}

// extractImports returns import paths from a file, skipping kong.
func extractImports(file *ast.File) []string {
	var out []string
	for _, imp := range file.Imports {
		path := strings.Trim(imp.Path.Value, `"`)
		if path == "github.com/alecthomas/kong" {
			continue
		}
		out = append(out, path)
	}
	return out
}

// extractExtraCode pulls var/const/func declarations from the source bytes,
// excluding import blocks and type declarations.
func extractExtraCode(file *ast.File, fset *token.FileSet, src []byte) string {
	var parts []string
	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.GenDecl:
			if d.Tok == token.IMPORT || d.Tok == token.TYPE {
				continue
			}
			// var or const
			start := fset.Position(d.Pos()).Offset
			end := fset.Position(d.End()).Offset
			parts = append(parts, string(src[start:end]))
		case *ast.FuncDecl:
			start := fset.Position(d.Pos()).Offset
			end := fset.Position(d.End()).Offset
			parts = append(parts, string(src[start:end]))
		}
	}
	return strings.Join(parts, "\n\n")
}

// parseTypesFiles parses all *_types.go files in dir and returns a map
// from struct name to parsed struct info. Only structs named "CLI" or
// ending in "Cmd" are included.
func parseTypesFiles(dir string) (map[string]*parsedStruct, error) {
	pattern := filepath.Join(dir, "*_types.go")
	files, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("globbing types files: %w", err)
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no *_types.go files found in %s", dir)
	}

	structs := make(map[string]*parsedStruct)
	fset := token.NewFileSet()

	for _, path := range files {
		src, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", path, err)
		}

		f, err := parser.ParseFile(fset, path, src, parser.ParseComments)
		if err != nil {
			return nil, fmt.Errorf("parsing %s: %w", path, err)
		}

		base := filepath.Base(path)
		imports := extractImports(f)
		extraCode := extractExtraCode(f, fset, src)

		// Track whether we've assigned imports/extraCode to the first Cmd struct in this file.
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
				ps := &parsedStruct{
					Name:       name,
					Fields:     fields,
					SourceFile: base,
				}
				// Attach imports and extraCode only to first Cmd struct in the file.
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

// isParentCmd checks whether a struct type has any fields with cmd:"" tags,
// making it a parent command that contains subcommands.
func isParentCmd(structs map[string]*parsedStruct, typeName string) bool {
	ps, ok := structs[typeName]
	if !ok {
		return false
	}
	for _, f := range ps.Fields {
		if f.IsCmd {
			return true
		}
	}
	return false
}

// buildNonCmdPrefix renders non-cmd fields as literal Go source lines
// (e.g., for KeepCmd's ServiceAccount and Impersonate fields).
func buildNonCmdPrefix(fields []parsedField) string {
	var lines []string
	for _, f := range fields {
		if f.IsCmd {
			continue
		}
		lines = append(lines, fmt.Sprintf("\t%s %s %s", f.GoName, f.GoType, f.Tag))
	}
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n") + "\n"
}

// buildSpecsForStruct recursively builds serviceSpecs for a parent Cmd struct
// and any child parent Cmds it contains.
func buildSpecsForStruct(structs map[string]*parsedStruct, structName, yamlKey string, specs *[]serviceSpec) {
	ps, ok := structs[structName]
	if !ok {
		fatal("struct %s not found in types files (missing *_types.go entry?)", structName)
		return
	}

	spec := serviceSpec{
		YAMLKey:      yamlKey,
		StructName:   structName,
		File:         outputFileName(yamlKey),
		Imports:      ps.Imports,
		ExtraCode:    ps.ExtraCode,
		NonCmdPrefix: buildNonCmdPrefix(ps.Fields),
	}

	for _, f := range ps.Fields {
		if !f.IsCmd {
			continue
		}
		childKey := yamlKeyFromTag(f.Tag, f.GoName)
		if isParentCmd(structs, f.GoType) {
			// Recurse: child parent Cmd gets its own spec with dotted YAML key.
			buildSpecsForStruct(structs, f.GoType, yamlKey+"."+childKey, specs)
		}
		spec.Fields = append(spec.Fields, field{
			GoName:  f.GoName,
			GoType:  f.GoType,
			Tag:     f.Tag,
			YAMLKey: childKey,
		})
	}

	*specs = append(*specs, spec)
}

// buildServiceSpecs walks the CLI struct and builds all serviceSpecs by
// discovering parent Cmd types and recursing into their subcommands.
func buildServiceSpecs(structs map[string]*parsedStruct) ([]serviceSpec, error) {
	cli, ok := structs["CLI"]
	if !ok {
		return nil, fmt.Errorf("CLI struct not found in types files")
	}

	var specs []serviceSpec
	for _, f := range cli.Fields {
		if !f.IsCmd {
			continue
		}
		if utilityTypes[f.GoType] {
			continue // utilities have no YAML key, always included
		}
		if !isParentCmd(structs, f.GoType) {
			continue // aliases (leaf commands on CLI) handled by buildCLIFields
		}
		yamlKey := yamlKeyFromTag(f.Tag, f.GoName)
		buildSpecsForStruct(structs, f.GoType, yamlKey, &specs)
	}

	return specs, nil
}

// buildCLIFields categorizes CLI struct fields into aliases (leaf commands)
// and services/utilities for the CLI generation file.
func buildCLIFields(structs map[string]*parsedStruct) (aliases []field, services []field) {
	cli, ok := structs["CLI"]
	if !ok {
		fatal("CLI struct not found in types files")
		return
	}

	for _, f := range cli.Fields {
		if !f.IsCmd {
			continue
		}
		yamlKey := yamlKeyFromTag(f.Tag, f.GoName)

		if utilityTypes[f.GoType] {
			// Utility: no YAMLKey, always included.
			services = append(services, field{
				GoName: f.GoName,
				GoType: f.GoType,
				Tag:    f.Tag,
			})
		} else if isParentCmd(structs, f.GoType) {
			// Service: parent Cmd with subcommands.
			services = append(services, field{
				GoName:  f.GoName,
				GoType:  f.GoType,
				Tag:     f.Tag,
				YAMLKey: yamlKey,
			})
		} else {
			// Alias: leaf command on CLI (e.g., Send, Ls, Download).
			aliases = append(aliases, field{
				GoName:  f.GoName,
				GoType:  f.GoType,
				Tag:     f.Tag,
				YAMLKey: yamlKey,
			})
		}
	}

	return aliases, services
}

// outputFileName converts a dotted YAML key to an output filename.
// "auth.service-account" -> "auth_service_account_cmd_gen.go"
func outputFileName(yamlKey string) string {
	name := strings.ReplaceAll(yamlKey, ".", "_")
	name = strings.ReplaceAll(name, "-", "_")
	return name + "_cmd_gen.go"
}
