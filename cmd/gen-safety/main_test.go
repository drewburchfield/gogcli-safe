package main

import "testing"

func TestIsEnabled(t *testing.T) {
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
		got := isEnabled(config, tt.key)
		if got != tt.want {
			t.Errorf("isEnabled(config, %q) = %v, want %v", tt.key, got, tt.want)
		}
	}

	// nil config = fail-closed
	if isEnabled(nil, "anything") {
		t.Error("isEnabled(nil, ...) should return false")
	}
}

func TestFilterFields(t *testing.T) {
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

	got := filterFields(fields, config)
	if len(got) != 1 || got[0].GoName != "Search" {
		t.Errorf("filterFields: got %v, want [Search]", got)
	}

	// nil config = fail-closed (all excluded)
	got = filterFields(fields, nil)
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
		{"classroom", true},    // explicitly false
		{"calendar", false},    // explicitly true
		{"gmail", false},       // map (not disabled)
		{"gmail.thread", false}, // nested map
		{"nonexistent", true},  // missing = disabled (fail-closed)
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
