package cli

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseCSVRejectsMalformedBatchFormat(t *testing.T) {
	tests := []struct {
		name, input string
	}{
		{"unknown header", "name,extra\nTask,ignored\n"},
		{"duplicate header", "name, Name\nTask,duplicate\n"},
		{"missing name header", "category_id\nC-10001\n"},
		{"ragged row", "name,category_id\nTask\n"},
		{"invalid estimate", "name,estimate\nTask,not-a-duration\n"},
		{"invalid seconds", "name,estimated_duration_seconds\nTask,12seconds\n"},
		{"negative seconds", "name,estimated_duration_seconds\nTask,-1\n"},
		{"ambiguous estimate fields", "name,estimate,estimated_duration_seconds\nTask,1h,3600\n"},
		{"invalid auto close", "name,auto_close\nTask,sometimes\n"},
		{"invalid percentage metadata", "name,completion_pct\nTask,NaN\n"},
		{"invalid timestamp metadata", "name,created_at\nTask,yesterday\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := parseCSV(strings.NewReader(tt.input), ','); err == nil {
				t.Fatalf("expected malformed batch format to be rejected")
			}
		})
	}
}

func TestParseCSVAcceptsDocumentedFieldsAndExportMetadata(t *testing.T) {
	input := "task_id,category_id,name,description,goal,status,priority,estimate,completion_pct,auto_close,created_at\nT-10001,C-10002,Import me,desc,goal,Completed,High,1h30m,100,true,2026-10-12T10:00:00Z\n"
	tasks, err := parseCSV(strings.NewReader(input), ',')
	if err != nil {
		t.Fatalf("parse export-compatible CSV: %v", err)
	}
	if len(tasks) != 1 || tasks[0].Name != "Import me" || tasks[0].CategoryID != "C-10002" || tasks[0].EstimatedDurationSeconds != 5400 || tasks[0].AutoClose == nil || !*tasks[0].AutoClose {
		t.Fatalf("unexpected imported task: %#v", tasks)
	}
}

func TestLegacyJSONRejectsUnknownFieldsAndInvalidValues(t *testing.T) {
	cases := []struct{ name, body string }{
		{"unknown field", `{"tasks":[{"name":"Task","unexpected":true}]}`},
		{"invalid duration type", `{"tasks":[{"name":"Task","estimated_duration_seconds":"60"}]}`},
		{"invalid estimate", `{"tasks":[{"name":"Task","estimate":"not-a-duration"}]}`},
		{"ambiguous estimates", `{"tasks":[{"name":"Task","estimate":"1h","estimated_duration_seconds":3600}]}`},
		{"invalid boolean", `{"tasks":[{"name":"Task","auto_close":"sometimes"}]}`},
		{"null estimate", `{"tasks":[{"name":"Task","estimate":null}]}`},
		{"null seconds", `{"tasks":[{"name":"Task","estimated_duration_seconds":null}]}`},
		{"duplicate key", `{"tasks":[{"name":"First","name":"Second"}]}`},
		{"trailing document", `{"tasks":[{"name":"Task"}]} {"tasks":[]}`},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if _, err := parseJSONOrYAML([]byte(test.body), true); err == nil {
				t.Fatalf("expected malformed legacy JSON to be rejected")
			}
		})
	}
}

func TestLegacyJSONAcceptsKnownExportMetadata(t *testing.T) {
	body := `{"tasks":[{"task_id":"T-1","project_id":"P-1","item_address":"1.1","name":"Task","status":"Completed","priority":"High","estimated_duration_seconds":60,"auto_close":true,"created_at":"2026-10-12T10:00:00Z"}]}`
	tasks, err := parseJSONOrYAML([]byte(body), true)
	if err != nil {
		t.Fatalf("parse exported task JSON: %v", err)
	}
	if len(tasks) != 1 || tasks[0].Name != "Task" || tasks[0].EstimatedDurationSeconds != 60 || tasks[0].AutoClose == nil || !*tasks[0].AutoClose {
		t.Fatalf("unexpected imported task: %#v", tasks)
	}
}

func TestSQLBatchRejectsWrongTableAndUnsupportedStatement(t *testing.T) {
	for _, body := range []string{
		"INSERT INTO projects (name) VALUES ('Not a task');",
		"DELETE FROM tasks;",
		"INSERT INTO tasks (name, surprise) VALUES ('Task', 'ignored');",
		"INSERT INTO tasks (name, estimated_duration_seconds) VALUES ('Task', '30');",
	} {
		if _, err := parseSQL([]byte(body)); err == nil {
			t.Fatalf("expected unsupported SQL batch to be rejected: %s", body)
		}
	}
}

func TestLegacyYAMLRejectsUnknownFieldsDuplicateKeysAndInvalidEstimates(t *testing.T) {
	cases := []struct{ name, body string }{
		{"unknown field", "tasks:\n  - name: Task\n    surprise: ignored\n"},
		{"duplicate key", "tasks:\n  - name: First\n    name: Second\n"},
		{"invalid estimate", "tasks:\n  - name: Task\n    estimate: not-a-duration\n"},
		{"multiple documents", "tasks:\n  - name: First\n---\ntasks:\n  - name: Second\n"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if _, err := parseJSONOrYAML([]byte(test.body), false); err == nil {
				t.Fatalf("expected malformed legacy YAML to be rejected")
			}
		})
	}
}

func TestNestedJSONRejectsDuplicateKeys(t *testing.T) {
	path := writeExecutionBatchFixture(t, "duplicate.json", `{"version":"timekeeper-execution-batch/v1","tasks":[{"name":"First","name":"Second"}]}`)
	if _, _, err := ParseExecutionBatch(path, "json"); err == nil || !strings.Contains(strings.ToLower(err.Error()), "duplicate") {
		t.Fatalf("duplicate JSON keys must be rejected with guidance; got %v", err)
	}
}

func TestSQLBatchAcceptsExportedEscapedStrings(t *testing.T) {
	input := "INSERT INTO tasks (task_id, category_id, name, description, goal, status, priority, estimated_duration_seconds, auto_close) VALUES ('T-1', 'C-1', 'O''Brien, API', '', '', 'Open', 'Normal', 3600, 1);"
	tasks, err := parseSQL([]byte(input))
	if err != nil {
		t.Fatalf("parse SQL task export: %v", err)
	}
	if len(tasks) != 1 || tasks[0].Name != "O'Brien, API" || tasks[0].EstimatedDurationSeconds != 3600 || tasks[0].AutoClose == nil || !*tasks[0].AutoClose {
		t.Fatalf("unexpected SQL-imported task: %#v", tasks)
	}
}

func TestLegacyXLSIsExplicitlyUnsupported(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.xls")
	if err := os.WriteFile(path, []byte("not an xlsx workbook"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ImportTasks(path, ""); err == nil || !strings.Contains(strings.ToLower(err.Error()), "unsupported format") {
		t.Fatalf(".xls should fail clearly instead of being parsed as a supported format: %v", err)
	}
}

func TestArchiveBatchRejectsUnsupportedFiles(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	file, err := zw.Create("notes.md")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = file.Write([]byte("not a supported task batch"))
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := parseZIP(buf.Bytes()); err == nil {
		t.Fatal("expected unsupported archive member to be rejected")
	}
}
