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
//	go run ./cmd/gen-safety --strict safety-profiles/agent-safe.yaml
//	go run ./cmd/gen-safety --verify          # check types files are in sync
//	go run ./cmd/gen-safety --sync            # extract structs to types files
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

// serviceSpec defines a service or nested parent command and how to generate its struct.
type serviceSpec struct {
	YAMLKey      string  // key in safety-profile.yaml
	StructName   string  // Go struct name (e.g. "GmailCmd")
	File         string  // output file (e.g. "gmail_cmd_gen.go")
	Fields       []field // all possible subcommand fields
	Imports      []string
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
	profilePath := "safety-profile.example.yaml"
	strict := false
	mode := "generate" // default mode
	for _, arg := range os.Args[1:] {
		switch arg {
		case "--strict":
			strict = true
		case "--verify":
			mode = "verify"
		case "--sync":
			mode = "sync"
		default:
			if strings.HasPrefix(arg, "-") {
				fatal("unknown flag: %s", arg)
			}
			profilePath = arg
		}
	}

	outputDir := "internal/cmd"

	// Handle --verify and --sync before profile-based generation.
	if mode == "verify" || mode == "sync" {
		if profilePath != "safety-profile.example.yaml" {
			fatal("--%s does not accept a profile path", mode)
		}
		if strict {
			fatal("--%s cannot be combined with --strict", mode)
		}
	}

	if mode == "verify" {
		violations, err := verifyTypes(outputDir)
		if err != nil {
			fatal("verify: %v", err)
		}
		if len(violations) == 0 {
			fmt.Println("OK: all parent command structs are in *_types.go files")
			return
		}
		fmt.Fprintf(os.Stderr, "gen-safety --verify: %d struct(s) need migration:\n", len(violations))
		for _, v := range violations {
			fmt.Fprintf(os.Stderr, "  - %s\n", v)
		}
		os.Exit(1)
	}

	if mode == "sync" {
		if err := syncTypes(outputDir); err != nil {
			fatal("sync: %v", err)
		}
		return
	}

	data, err := os.ReadFile(profilePath)
	if err != nil {
		fatal("reading profile: %v", err)
	}

	var profile map[string]any
	if err := yaml.Unmarshal(data, &profile); err != nil {
		fatal("parsing YAML: %v", err)
	}

	if len(profile) == 0 {
		fatal("profile is empty or null - all services would be silently disabled. Check your YAML file.")
	}

	structs, err := parseTypesFiles(outputDir)
	if err != nil {
		fatal("parsing types: %v", err)
	}

	specs, err := buildServiceSpecs(structs)
	if err != nil {
		fatal("building specs: %v", err)
	}

	aliases, services := buildCLIFields(structs)

	// Validate YAML keys against known specs to catch typos.
	knownKeys := buildKnownKeys(specs, aliases, services)
	validateYAMLKeys(profile, knownKeys, "")

	for _, spec := range specs {
		if err := generateServiceFile(outputDir, spec, profile); err != nil {
			fatal("generating %s: %v", spec.File, err)
		}
	}

	if err := generateCLIFile(outputDir, profile, aliases, services); err != nil {
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

func generateCLIFile(dir string, profile map[string]any, cliAliases []field, cliServices []field) error {
	aliasConfig := resolveDottedSection(profile, "aliases")

	var buf bytes.Buffer
	buf.WriteString(genHeader)

	buf.WriteString("import \"github.com/alecthomas/kong\"\n\n")

	buf.WriteString("type CLI struct {\n")
	buf.WriteString("\tRootFlags `embed:\"\"`\n\n")
	buf.WriteString("\tVersion kong.VersionFlag `help:\"Print version and exit\"`\n\n")

	// Action-first aliases
	buf.WriteString("\t// Action-first desire paths (agent-friendly shortcuts).\n")
	for _, f := range cliAliases {
		if isEnabled(aliasConfig, f.YAMLKey) {
			writeStructField(&buf, f)
		}
	}
	buf.WriteString("\n")

	// Service commands: include services that have at least one enabled command.
	// Fields without a YAMLKey (utility commands) are always included.
	for _, f := range cliServices {
		if f.YAMLKey != "" {
			if isServiceDisabled(profile, f.YAMLKey) {
				continue
			}
			// Also skip if service map is present but all leaves are false
			// (e.g., gmail: { send: false, drafts: { create: false } }).
			svcConfig := resolveDottedSection(profile, f.YAMLKey)
			if svcConfig != nil && !mapHasEnabledLeaf(svcConfig) {
				continue
			}
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
	if spec.NonCmdPrefix != "" {
		fmt.Fprintf(&buf, "type %s struct {\n%s}\n", spec.StructName, spec.NonCmdPrefix)
	} else {
		fmt.Fprintf(&buf, "type %s struct{}\n", spec.StructName)
	}
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
	for _, part := range parts {
		if current == nil {
			return false
		}
		v, ok := current[part]
		if !ok {
			return false
		}
		if b, ok := v.(bool); ok {
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
	for k, v := range m {
		switch val := v.(type) {
		case bool:
			if val {
				return true
			}
		case map[string]any:
			if mapHasEnabledLeaf(val) {
				return true
			}
		default:
			fatal("invalid value for key %q: got %T (%v), expected bool or map", k, v, v)
		}
	}
	return false
}

// buildKnownKeys constructs a set of all valid YAML key paths from the specs.
// The services list is used to derive tolerated YAML keys for utility commands
// (those with empty YAMLKey), keeping this in sync with the utilityTypes map
// in discover.go automatically.
func buildKnownKeys(specs []serviceSpec, aliases []field, services []field) map[string]bool {
	known := make(map[string]bool)
	// Top-level sections recognized by the generator
	known["aliases"] = true
	// Utility commands: always included, YAML keys tolerated but ignored.
	// Derived from utilityTypes via buildCLIFields (empty YAMLKey = utility).
	for _, f := range services {
		if f.YAMLKey == "" {
			key := yamlKeyFromTag(f.Tag, f.GoName)
			known[key] = true
		}
	}
	for _, spec := range specs {
		addSpecKeys(known, spec.YAMLKey, spec.Fields)
	}
	// Alias sub-keys
	for _, f := range aliases {
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
			warn("unexpected type %T at key %q in profile, treating as disabled", v, part)
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
