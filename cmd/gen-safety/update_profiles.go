package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// profileDefault determines the default value for missing keys based on the
// profile filename. full.yaml defaults to true; all others default to false.
func profileDefault(profilePath string) bool {
	base := filepath.Base(profilePath)
	return base == "full.yaml"
}

// updateProfiles scans the given profile files for missing command keys.
// In normal mode it adds missing keys with profile-appropriate defaults,
// appending them to the appropriate sections while preserving existing
// formatting and comments. When dryRun is true it reports what would be
// added without modifying any files.
func updateProfiles(outputDir string, profilePaths []string, dryRun bool) error {
	structs, err := parseTypesFiles(outputDir)
	if err != nil {
		return fmt.Errorf("parsing types: %w", err)
	}

	specs, err := buildServiceSpecs(structs)
	if err != nil {
		return fmt.Errorf("building specs: %w", err)
	}

	aliases, _ := buildCLIFields(structs)

	// Build the expected YAML structure as a tree of keys.
	expected := buildExpectedKeys(specs, aliases)

	totalAdded := 0
	for _, profilePath := range profilePaths {
		added, err := updateOneProfile(profilePath, expected, dryRun)
		if err != nil {
			return fmt.Errorf("updating %s: %w", profilePath, err)
		}
		totalAdded += added
	}

	if totalAdded == 0 {
		fmt.Println("All profiles are up to date.")
	}
	return nil
}

// keyNode represents a node in the expected YAML key tree.
type keyNode struct {
	children map[string]*keyNode // nil for leaf keys
	order    []string            // insertion order
}

func newKeyNode() *keyNode {
	return &keyNode{children: make(map[string]*keyNode)}
}

func (n *keyNode) addLeaf(key string) {
	if _, ok := n.children[key]; !ok {
		n.children[key] = nil // leaf
		n.order = append(n.order, key)
	}
}

func (n *keyNode) addBranch(key string) *keyNode {
	if child, ok := n.children[key]; ok && child != nil {
		return child
	}
	child := newKeyNode()
	if _, exists := n.children[key]; !exists {
		n.order = append(n.order, key)
	}
	n.children[key] = child
	return child
}

// buildExpectedKeys builds the expected YAML key tree from specs.
func buildExpectedKeys(specs []serviceSpec, aliases []field) *keyNode {
	root := newKeyNode()

	// Service specs: each has a dotted YAML key and leaf fields.
	for _, spec := range specs {
		parts := strings.Split(spec.YAMLKey, ".")
		node := root
		for _, part := range parts {
			node = node.addBranch(part)
		}
		for _, f := range spec.Fields {
			node.addLeaf(f.YAMLKey)
		}
	}

	// Aliases section
	aliasNode := root.addBranch("aliases")
	for _, f := range aliases {
		aliasNode.addLeaf(f.YAMLKey)
	}

	// Utility commands (see utilityTypes in discover.go) are always included
	// and their YAML keys have no effect. Don't require them in profiles.

	return root
}

// updateOneProfile updates a single profile file, adding missing keys.
// Uses yaml.v3 for parsing/detection but appends new keys as text lines
// to preserve original formatting exactly.
func updateOneProfile(profilePath string, expected *keyNode, dryRun bool) (int, error) {
	data, err := os.ReadFile(profilePath)
	if err != nil {
		return 0, err
	}

	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return 0, fmt.Errorf("parsing YAML: %w", err)
	}

	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		return 0, fmt.Errorf("unexpected YAML structure in %s", profilePath)
	}

	rootMap := doc.Content[0]
	if rootMap.Kind != yaml.MappingNode {
		return 0, fmt.Errorf("expected mapping at root of %s", profilePath)
	}

	defaultVal := profileDefault(profilePath)
	base := filepath.Base(profilePath)

	// Collect missing keys grouped by their parent section.
	groups := collectMissingGrouped(rootMap, expected)

	if len(groups) == 0 {
		return 0, nil
	}

	totalMissing := 0
	for _, g := range groups {
		totalMissing += len(g.keys)
	}

	if dryRun {
		fmt.Printf("%s: %d key(s) would be added (dry-run, default: %v)\n", base, totalMissing, defaultVal)
		for _, g := range groups {
			for _, k := range g.keys {
				fmt.Printf("  + %s\n", k.fullPath)
			}
		}
		return totalMissing, nil
	}

	// Build the text lines to append and insert them into the original file.
	lines := strings.Split(string(data), "\n")
	added := 0

	// Sort groups so insertions don't shift later positions:
	// - Non-append groups first (sorted by insertAfterLine descending)
	// - For same insertAfterLine, shallower indent first so deeper
	//   content is inserted last and ends up above shallower content
	// - Append-to-end groups last (processed in order)
	sort.Slice(groups, func(i, j int) bool {
		if groups[i].appendToEnd != groups[j].appendToEnd {
			return !groups[i].appendToEnd
		}
		if groups[i].insertAfterLine != groups[j].insertAfterLine {
			return groups[i].insertAfterLine > groups[j].insertAfterLine
		}
		return groups[i].indent < groups[j].indent
	})

	for _, g := range groups {
		var newLines []string
		if g.sectionHeader != "" {
			newLines = append(newLines, g.sectionHeader+" # NEW")
		}
		for _, k := range g.keys {
			newLines = append(newLines, formatYAMLLine(k.key, defaultVal, g.indent))
			added++
		}
		if g.appendToEnd {
			// New top-level section: append at end of file.
			lines = append(lines, "")
			lines = append(lines, newLines...)
		} else {
			// Insert after the last key in this section.
			insertAt := g.insertAfterLine
			if insertAt < 0 || insertAt > len(lines) {
				return 0, fmt.Errorf("BUG: insertion point %d out of range (file has %d lines) for %s",
					insertAt, len(lines), base)
			}
			after := make([]string, len(lines[insertAt:]))
			copy(after, lines[insertAt:])
			lines = append(lines[:insertAt], newLines...)
			lines = append(lines, after...)
		}
	}

	out := strings.Join(lines, "\n")

	// Sanity-check: the output must still be valid YAML.
	var check yaml.Node
	if err := yaml.Unmarshal([]byte(out), &check); err != nil {
		return 0, fmt.Errorf("BUG: generated output is invalid YAML for %s: %w", base, err)
	}

	// Safe write: write to temp file then rename (atomic on same filesystem).
	tmpPath := profilePath + ".tmp"
	if err := os.WriteFile(tmpPath, []byte(out), 0o644); err != nil {
		return 0, fmt.Errorf("writing temp file for %s: %w", profilePath, err)
	}
	if err := os.Rename(tmpPath, profilePath); err != nil {
		os.Remove(tmpPath)
		return 0, fmt.Errorf("renaming %s: %w", profilePath, err)
	}

	fmt.Printf("%s: added %d missing key(s) (default: %v)\n", base, added, defaultVal)
	return added, nil
}

// missingKey represents a single key to add.
type missingKey struct {
	key      string // bare key name (e.g., "send")
	fullPath string // dotted path for display (e.g., "gmail.send")
}

// missingGroup represents a batch of keys to add to the same YAML section.
type missingGroup struct {
	sectionHeader   string // optional pre-formatted section header (e.g., "calendar:" or "  subsection:")
	keys            []missingKey
	indent          int  // indentation level for keys (number of spaces)
	insertAfterLine int  // 1-indexed line number (from yaml.Node.Line); used as slice index to insert after
	appendToEnd     bool // if true, append to end of file (new top-level key or section)
}

// collectMissingGrouped walks the expected tree and groups missing keys by
// their parent section, including the line number where they should be inserted.
func collectMissingGrouped(rootMap *yaml.Node, expected *keyNode) []missingGroup {
	var groups []missingGroup

	for _, key := range expected.order {
		child := expected.children[key]
		idx := findYAMLKey(rootMap, key)

		if idx < 0 {
			// Entire top-level section missing.
			if child == nil {
				groups = append(groups, missingGroup{
					keys:        []missingKey{{key: key, fullPath: key}},
					indent:      0,
					appendToEnd: true,
				})
			} else {
				var keys []missingKey
				collectAllMissingKeys(child, key, &keys)
				if len(keys) > 0 {
					groups = append(groups, missingGroup{
						sectionHeader: key + ":",
						keys:          keys,
						indent:        2,
						appendToEnd:   true,
					})
				}
			}
		} else if child != nil {
			valIdx := idx + 1
			if valIdx < len(rootMap.Content) {
				valNode := rootMap.Content[valIdx]
				if valNode.Kind == yaml.MappingNode {
					collectMissingInSection(valNode, child, key, 2, &groups)
				}
				// Bool shorthand (e.g., "classroom: false"): the user opted to
				// enable/disable the entire section, so don't expand into keys.
			}
		}
	}
	return groups
}

// collectMissingInSection recursively finds missing keys within an existing section.
func collectMissingInSection(mapNode *yaml.Node, expected *keyNode, prefix string, indent int, groups *[]missingGroup) {
	var missing []missingKey
	lastLine := mapNode.Line // fallback for empty mapping nodes

	// Find the last line of this section for insertion point.
	if len(mapNode.Content) > 0 {
		last := mapNode.Content[len(mapNode.Content)-1]
		lastLine = deepLastLine(last)
	}

	for _, key := range expected.order {
		child := expected.children[key]
		idx := findYAMLKey(mapNode, key)
		fullKey := prefix + "." + key

		if idx < 0 {
			if child == nil {
				missing = append(missing, missingKey{key: key, fullPath: fullKey})
			} else {
				// Missing nested section: emit as separate group with section header.
				var nestedKeys []missingKey
				collectAllMissingKeys(child, fullKey, &nestedKeys)
				if len(nestedKeys) > 0 {
					*groups = append(*groups, missingGroup{
						sectionHeader:   strings.Repeat(" ", indent) + key + ":",
						keys:            nestedKeys,
						indent:          indent + 2,
						insertAfterLine: lastLine,
					})
				}
			}
		} else if child != nil {
			valIdx := idx + 1
			if valIdx < len(mapNode.Content) && mapNode.Content[valIdx].Kind == yaml.MappingNode {
				collectMissingInSection(mapNode.Content[valIdx], child, fullKey, indent+2, groups)
			}
		}
	}

	if len(missing) > 0 {
		*groups = append(*groups, missingGroup{
			keys:            missing,
			indent:          indent,
			insertAfterLine: lastLine, // insert after the last key in this section
		})
	}
}

func collectAllMissingKeys(n *keyNode, prefix string, out *[]missingKey) {
	for _, key := range n.order {
		child := n.children[key]
		fullKey := prefix + "." + key
		if child == nil {
			*out = append(*out, missingKey{key: key, fullPath: fullKey})
		} else {
			collectAllMissingKeys(child, fullKey, out)
		}
	}
}

// formatYAMLLine formats a single "key: value # NEW" YAML line at the given
// indentation. The "# NEW" marker helps users identify machine-added entries.
func formatYAMLLine(key string, defaultVal bool, indent int) string {
	valStr := "false"
	if defaultVal {
		valStr = "true"
	}
	return fmt.Sprintf("%s%s: %s # NEW", strings.Repeat(" ", indent), key, valStr)
}

// deepLastLine returns the last line number occupied by a YAML node,
// recursing into MappingNode children. yaml.v3 sets MappingNode.Line to
// the first child's line, so we must walk to the deepest last child.
func deepLastLine(node *yaml.Node) int {
	if node.Kind == yaml.MappingNode && len(node.Content) > 0 {
		return deepLastLine(node.Content[len(node.Content)-1])
	}
	return node.Line
}

// findYAMLKey returns the index of the key node in a MappingNode's Content,
// or -1 if not found. MappingNode.Content alternates key, value, key, value...
func findYAMLKey(mapNode *yaml.Node, key string) int {
	for i := 0; i < len(mapNode.Content)-1; i += 2 {
		if mapNode.Content[i].Value == key {
			return i
		}
	}
	return -1
}

// collectMissing walks the expected key tree and collects dotted key paths
// that are absent from the YAML mapping node. Used by tests.
func collectMissing(mapNode *yaml.Node, expected *keyNode, prefix string, out *[]string) {
	if expected == nil || mapNode.Kind != yaml.MappingNode {
		return
	}
	for _, key := range expected.order {
		child := expected.children[key]
		fullKey := key
		if prefix != "" {
			fullKey = prefix + "." + key
		}
		idx := findYAMLKey(mapNode, key)

		if idx < 0 {
			if child == nil {
				*out = append(*out, fullKey)
			} else {
				var mk []missingKey
				collectAllMissingKeys(child, fullKey, &mk)
				for _, m := range mk {
					*out = append(*out, m.fullPath)
				}
			}
		} else if child != nil {
			valIdx := idx + 1
			if valIdx < len(mapNode.Content) && mapNode.Content[valIdx].Kind == yaml.MappingNode {
				collectMissing(mapNode.Content[valIdx], child, fullKey, out)
			}
		}
	}
}
