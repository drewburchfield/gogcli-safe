package cmd

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/steipete/gogcli/internal/backup"
)

func TestBackupInitDryRunDoesNotWriteConfigOrRepo(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "backup.json")
	repoPath := filepath.Join(dir, "repo")

	var stdout bytes.Buffer
	origStdout := os.Stdout
	readPipe, writePipe, pipeErr := os.Pipe()
	if pipeErr != nil {
		t.Fatalf("pipe: %v", pipeErr)
	}
	os.Stdout = writePipe
	t.Cleanup(func() {
		os.Stdout = origStdout
	})
	err := (&BackupInitCmd{
		backupFlags: backupFlags{
			Config: configPath,
			Repo:   repoPath,
			NoPush: true,
		},
	}).Run(newCmdJSONOutputContext(t, &stdout, nil), &RootFlags{DryRun: true, NoInput: true})
	_ = writePipe.Close()
	os.Stdout = origStdout
	if _, copyErr := io.Copy(&stdout, readPipe); copyErr != nil {
		t.Fatalf("read stdout: %v", copyErr)
	}

	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != 0 {
		t.Fatalf("expected dry-run exit 0, got %#v", err)
	}
	if _, statErr := os.Stat(configPath); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("dry-run wrote config: %v", statErr)
	}
	if _, statErr := os.Stat(repoPath); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("dry-run created repo: %v", statErr)
	}

	var payload struct {
		DryRun  bool           `json:"dry_run"`
		Op      string         `json:"op"`
		Request map[string]any `json:"request"`
	}
	if decodeErr := json.Unmarshal(stdout.Bytes(), &payload); decodeErr != nil {
		t.Fatalf("decode dry-run output: %v\n%s", decodeErr, stdout.String())
	}
	if !payload.DryRun || payload.Op != "backup.init" {
		t.Fatalf("unexpected dry-run payload: %#v", payload)
	}
	if payload.Request["repo"] != repoPath || payload.Request["push"] != false {
		t.Fatalf("unexpected request: %#v", payload.Request)
	}
}

func TestBackupExportReportsManifestSemanticCounts(t *testing.T) {
	repo, config, recipients := newBackupConfigForCmdTest(t)
	shard, err := backup.NewJSONLShard("contacts", "people", "acct", "data/contacts/acct/people/part-0001.jsonl.gz.age", []map[string]string{
		{"source": "connections"},
		{"source": "other"},
	})
	if err != nil {
		t.Fatalf("NewJSONLShard: %v", err)
	}
	if _, pushErr := backup.PushSnapshot(t.Context(), backup.Snapshot{
		Services: []string{"contacts"},
		Accounts: []string{"acct"},
		Counts: map[string]int{
			"contacts.connections": 1,
			"contacts.other":       1,
			"contacts.people":      99,
		},
		Shards: []backup.PlainShard{shard},
	}, backup.Options{ConfigPath: config, Recipients: recipients, Push: false}); pushErr != nil {
		t.Fatalf("PushSnapshot: %v", pushErr)
	}

	var stdout bytes.Buffer
	err = (&BackupExportCmd{
		backupReadFlags: backupReadFlags{Config: config, Repo: repo, NoPull: true},
		Out:             filepath.Join(t.TempDir(), "export"),
	}).Run(newCmdOutputContext(t, &stdout, io.Discard))
	if err != nil {
		t.Fatalf("BackupExportCmd.Run: %v", err)
	}
	for _, want := range []string{
		"count.contacts.connections\t1",
		"count.contacts.other\t1",
		"count.contacts.people\t2",
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("export output missing %q:\n%s", want, stdout.String())
		}
	}
}

func TestBackupExportReportsManifestCountsForSemanticCollisions(t *testing.T) {
	repo, config, recipients := newBackupConfigForCmdTest(t)
	shard, err := backup.NewJSONLShard("drive", "contents", "acct", "data/drive/acct/contents/part-0001.jsonl.gz.age", []driveBackupContent{
		{FileID: "ok", Name: "ok", ExportName: "ok.txt", DataBase64: base64.StdEncoding.EncodeToString([]byte("ok"))},
		{FileID: "skipped", Name: "skipped", ExportName: "skipped.txt", Skipped: true},
		{FileID: "error", Name: "error", ExportName: "error.txt", Error: "export failed"},
	})
	if err != nil {
		t.Fatalf("NewJSONLShard: %v", err)
	}
	if _, pushErr := backup.PushSnapshot(t.Context(), backup.Snapshot{
		Services: []string{"drive"},
		Accounts: []string{"acct"},
		Counts: map[string]int{
			"drive.contents":         1,
			"drive.contents.skipped": 1,
			"drive.contents.errors":  1,
		},
		Shards: []backup.PlainShard{shard},
	}, backup.Options{ConfigPath: config, Recipients: recipients, Push: false}); pushErr != nil {
		t.Fatalf("PushSnapshot: %v", pushErr)
	}

	var stdout bytes.Buffer
	err = (&BackupExportCmd{
		backupReadFlags: backupReadFlags{Config: config, Repo: repo, NoPull: true},
		Out:             filepath.Join(t.TempDir(), "export"),
	}).Run(newCmdOutputContext(t, &stdout, io.Discard))
	if err != nil {
		t.Fatalf("BackupExportCmd.Run: %v", err)
	}
	for _, want := range []string{
		"count.drive.contents\t1",
		"count.drive.contents.errors\t1",
		"count.drive.contents.skipped\t1",
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("export output missing %q:\n%s", want, stdout.String())
		}
	}
}
