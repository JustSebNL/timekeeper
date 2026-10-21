package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeExecutionBatchFixture(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestParseExecutionBatchJSONKeepsPlanningDataNonExecutable(t *testing.T) {
	path := writeExecutionBatchFixture(t, "batch.json", `{"version":"timekeeper-execution-batch/v1","tasks":[{"category_id":"C-10","name":"Build view","estimated_duration_seconds":3600,"sprints":[{"name":"Implement","estimated_duration_seconds":1800}],"subtasks":[{"name":"Verify view","estimated_duration_seconds":900,"sprints":[{"name":"Run checks","estimated_duration_seconds":600}]}]}],"extensions":[{"task_id":"T-20","sprints":[{"name":"Follow-up","estimated_duration_seconds":300}]}],"planning":{"executable_task_metadata":[{"task_index":0,"key":"build_view","phase":1,"acceptance":["Accessible"]}],"deferred_items":[{"key":"future_provider","condition":"Await provider contract","acceptance":["Provider failure is isolated"]}],"release_gates":[],"application_constraints":["Do not deploy"]}}`)

	input, legacyTasks, err := ParseExecutionBatch(path, "")
	if err != nil {
		t.Fatalf("ParseExecutionBatch(JSON): %v", err)
	}
	if len(legacyTasks) != 0 {
		t.Fatalf("nested batch returned %d legacy Tasks", len(legacyTasks))
	}
	if input.Version != "timekeeper-execution-batch/v1" || len(input.Tasks) != 1 || len(input.Extensions) != 1 {
		t.Fatalf("parsed nested batch has wrong version or executable entries: %#v", input)
	}
	if input.Planning == nil || len(input.Planning.DeferredItems) != 1 {
		t.Fatalf("planning metadata was not preserved: %#v", input.Planning)
	}
	result := ValidateExecutionBatchDocument(input)
	if !result.Valid || result.Counts.Tasks != 1 || result.Counts.Subtasks != 1 || result.Counts.Sprints != 3 {
		t.Fatalf("planning metadata changed executable counts or valid batch failed: %#v", result)
	}
}

func TestParseExecutionBatchYAML(t *testing.T) {
	path := writeExecutionBatchFixture(t, "batch.yaml", "version: timekeeper-execution-batch/v1\ntasks: [{category_id: C-10, name: 'Build view', estimated_duration_seconds: 3600, subtasks: [{name: 'Verify view', estimated_duration_seconds: 900}]}]\nextensions: []\n")
	input, _, err := ParseExecutionBatch(path, "")
	if err != nil {
		t.Fatalf("ParseExecutionBatch(YAML): %v", err)
	}
	if input.Version != "timekeeper-execution-batch/v1" || len(input.Tasks) != 1 || input.Tasks[0].Subtasks[0].Name != "Verify view" {
		t.Fatalf("YAML fields were not decoded: %#v", input)
	}
}

func TestParseExecutionBatchRejectsUnknownFieldsWithPathAndFix(t *testing.T) {
	tests := []struct {
		name, content, wantPath string
	}{
		{"executable object", `{"version":"timekeeper-execution-batch/v1","tasks":[{"category_id":"C-10","name":"Build","unexpected":true}]}`, "tasks[0].unexpected"},
		{"planning object", `{"version":"timekeeper-execution-batch/v1","tasks":[{"category_id":"C-10","name":"Build"}],"planning":{"deferred_items":[],"unknown_note":"not executable"}}`, "planning.unknown_note"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeExecutionBatchFixture(t, "batch.json", tt.content)
			_, _, err := ParseExecutionBatch(path, "json")
			if err == nil || !strings.Contains(err.Error(), tt.wantPath) || !strings.Contains(strings.ToLower(err.Error()), "remove") {
				t.Fatalf("error must identify %q and explain the correction; got %v", tt.wantPath, err)
			}
		})
	}
}

func TestParseExecutionBatchRejectsUnversionedNestedDocument(t *testing.T) {
	path := writeExecutionBatchFixture(t, "batch.json", `{"tasks":[{"category_id":"C-10","name":"Build","subtasks":[{"name":"Verify"}]}],"extensions":[]}`)
	_, _, err := ParseExecutionBatch(path, "json")
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "version") || !strings.Contains(strings.ToLower(err.Error()), "set") {
		t.Fatalf("nested data without a version must be rejected with a correction; got %v", err)
	}
}

func TestParseExecutionBatchRejectsUnknownYAMLFieldWithPath(t *testing.T) {
	path := writeExecutionBatchFixture(t, "batch.yaml", "version: timekeeper-execution-batch/v1\ntasks:\n  - category_id: C-10\n    name: Build\n    unexpected: true\n")
	_, _, err := ParseExecutionBatch(path, "yaml")
	if err == nil || !strings.Contains(err.Error(), "tasks[0].unexpected") || !strings.Contains(strings.ToLower(err.Error()), "remove") {
		t.Fatalf("YAML unknown-field error must identify its path and correction; got %v", err)
	}
}

func TestParseExecutionBatchRejectsDuplicateYAMLKeysWithLocation(t *testing.T) {
	path := writeExecutionBatchFixture(t, "batch.yaml", "version: timekeeper-execution-batch/v1\ntasks: []\ntasks: []\n")
	_, _, err := ParseExecutionBatch(path, "yaml")
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "line") || !strings.Contains(strings.ToLower(err.Error()), "duplicate") {
		t.Fatalf("duplicate YAML keys need a location and repair guidance; got %v", err)
	}
}

func TestParseExecutionBatchReportsMalformedYAMLLocation(t *testing.T) {
	path := writeExecutionBatchFixture(t, "batch.yaml", "version: timekeeper-execution-batch/v1\ntasks: [\n  - name: broken\n")
	_, _, err := ParseExecutionBatch(path, "yaml")
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "line") || !strings.Contains(strings.ToLower(err.Error()), "fix") {
		t.Fatalf("malformed YAML error must include a source location and repair hint; got %v", err)
	}
}

func TestParseExecutionBatchRejectsMultipleYAMLDocuments(t *testing.T) {
	path := writeExecutionBatchFixture(t, "batch.yaml", "version: timekeeper-execution-batch/v1\ntasks: []\n---\nversion: timekeeper-execution-batch/v1\ntasks: []\n")
	if _, _, err := ParseExecutionBatch(path, "yaml"); err == nil || !strings.Contains(strings.ToLower(err.Error()), "one document") {
		t.Fatalf("multiple YAML documents must be rejected: %v", err)
	}
}

func TestValidateExecutionBatchDocumentReportsActionableIndependentIssuesInOrder(t *testing.T) {
	path := writeExecutionBatchFixture(t, "batch.json", `{"version":"timekeeper-execution-batch/v1","tasks":[{"category_id":"C-10","name":"Build view","estimated_duration_seconds":-1,"sprints":[{"name":"Implement","estimated_duration_seconds":0,"buffer_pct":10.5}]}],"extensions":[{"task_id":"T-20","sprints":[{"name":"Follow-up","estimated_duration_seconds":300}]},{"task_id":"T-20","subtasks":[{"name":"Second extension"}]}]}`)
	input, _, err := ParseExecutionBatch(path, "json")
	if err != nil {
		t.Fatalf("ParseExecutionBatch: %v", err)
	}
	result := ValidateExecutionBatchDocument(input)
	if result.Valid || len(result.Issues) != 4 {
		t.Fatalf("expected four independent issues, got %#v", result)
	}
	want := []struct{ path, field string }{
		{"tasks[0]", "estimated_duration_seconds"},
		{"tasks[0].sprints[0]", "estimated_duration_seconds"},
		{"tasks[0].sprints[0]", "buffer_pct"},
		{"extensions[1]", "task_id"},
	}
	for i, issue := range result.Issues {
		if issue.Path != want[i].path || issue.Field != want[i].field || issue.Code == "" || issue.Message == "" || issue.Suggestion == "" {
			t.Errorf("issue %d must have stable location, reason, and repair suggestion; got %#v", i, issue)
		}
	}
}

func TestValidateExecutionBatchDocumentRejectsEmptyUnsupportedAndOversizedBatches(t *testing.T) {
	path := writeExecutionBatchFixture(t, "batch.json", `{"version":"timekeeper-execution-batch/v1","tasks":[{"category_id":"C-10","name":"Build","estimated_duration_seconds":60}]}`)
	input, _, err := ParseExecutionBatch(path, "json")
	if err != nil {
		t.Fatalf("ParseExecutionBatch: %v", err)
	}

	input.Version = "timekeeper-execution-batch/v9"
	unsupported := ValidateExecutionBatchDocument(input)
	if unsupported.Valid || len(unsupported.Issues) == 0 || unsupported.Issues[0].Field != "version" || unsupported.Issues[0].Suggestion == "" {
		t.Fatalf("unsupported version needs a repair issue: %#v", unsupported)
	}

	input.Version = "timekeeper-execution-batch/v1"
	validTasks := input.Tasks
	input.Tasks = nil
	empty := ValidateExecutionBatchDocument(input)
	if empty.Valid || len(empty.Issues) == 0 {
		t.Fatalf("empty batch must fail validation: %#v", empty)
	}

	input.Tasks = append(input.Tasks, validTasks[0])
	for len(input.Tasks) < 101 {
		input.Tasks = append(input.Tasks, validTasks[0])
	}
	oversized := ValidateExecutionBatchDocument(input)
	if oversized.Valid || len(oversized.Issues) == 0 || oversized.Issues[0].Code != "task_limit" {
		t.Fatalf("more than 100 Tasks must fail with a limit repair issue: %#v", oversized)
	}
}

func TestValidateExecutionBatchDocumentRejectsMoreThanFourDirectSprints(t *testing.T) {
	path := writeExecutionBatchFixture(t, "batch.json", `{"version":"timekeeper-execution-batch/v1","tasks":[{"category_id":"C-10","name":"Build","sprints":[{"name":"One","estimated_duration_seconds":60}]}]}`)
	input, _, err := ParseExecutionBatch(path, "json")
	if err != nil {
		t.Fatalf("ParseExecutionBatch: %v", err)
	}
	for len(input.Tasks[0].Sprints) < 5 {
		input.Tasks[0].Sprints = append(input.Tasks[0].Sprints, input.Tasks[0].Sprints[0])
	}
	result := ValidateExecutionBatchDocument(input)
	if result.Valid || len(result.Issues) != 1 || result.Issues[0].Path != "tasks[0]" || result.Issues[0].Field != "sprints" || result.Issues[0].Code != "sprint_limit" {
		t.Fatalf("Task with more than four direct Sprints must fail with a repair hint: %#v", result)
	}
}

func TestValidateExecutionBatchDocumentRejectsMoreThanFiveHundredObjects(t *testing.T) {
	path := writeExecutionBatchFixture(t, "batch.json", `{"version":"timekeeper-execution-batch/v1","tasks":[{"category_id":"C-10","name":"Build","subtasks":[{"name":"First"}]}]}`)
	input, _, err := ParseExecutionBatch(path, "json")
	if err != nil {
		t.Fatalf("ParseExecutionBatch: %v", err)
	}
	first := input.Tasks[0].Subtasks[0]
	for len(input.Tasks[0].Subtasks) < 500 {
		input.Tasks[0].Subtasks = append(input.Tasks[0].Subtasks, first)
	}
	result := ValidateExecutionBatchDocument(input)
	if result.Valid || result.Counts.TotalObjects != 501 || len(result.Issues) != 1 || result.Issues[0].Code != "object_limit" {
		t.Fatalf("more than 500 executable objects must fail with a repair hint: %#v", result)
	}
}

func TestParseExecutionBatchPreservesLegacyTaskOnlyJSON(t *testing.T) {
	path := writeExecutionBatchFixture(t, "legacy.json", `{"tasks":[{"category_id":"C-10","name":"Legacy import","estimated_duration_seconds":60}]}`)
	input, tasks, err := ParseExecutionBatch(path, "")
	if err != nil {
		t.Fatalf("ParseExecutionBatch(legacy): %v", err)
	}
	if input.Version != "" || len(tasks) != 1 || tasks[0].Name != "Legacy import" {
		t.Fatalf("legacy task import changed: input=%#v tasks=%#v", input, tasks)
	}
}

func TestParseExecutionBatchPreservesLegacyCSVFromStdin(t *testing.T) {
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	previousStdin := os.Stdin
	os.Stdin = read
	defer func() {
		os.Stdin = previousStdin
		read.Close()
	}()
	if _, err := write.WriteString("name,category_id\nLegacy stdin,C-10\n"); err != nil {
		t.Fatal(err)
	}
	if err := write.Close(); err != nil {
		t.Fatal(err)
	}

	input, tasks, err := ParseExecutionBatch("-", "csv")
	if err != nil {
		t.Fatalf("ParseExecutionBatch(stdin CSV): %v", err)
	}
	if input.Version != "" || len(tasks) != 1 || tasks[0].Name != "Legacy stdin" {
		t.Fatalf("legacy stdin import changed: input=%#v tasks=%#v", input, tasks)
	}
}

func TestExtendedWebUIMigrationManifestUsesSupportedSafeBatchContract(t *testing.T) {
	path := filepath.Join("..", "..", "docs", "Extended-Web-UI-Migration-Objects.json")
	input, legacy, err := ParseExecutionBatch(path, "json")
	if err != nil {
		t.Fatalf("parse migration batch manifest: %v", err)
	}
	if len(legacy) != 0 || input.Version != executionBatchVersion || len(input.Tasks) != 23 || len(input.Extensions) != 7 {
		t.Fatalf("manifest was not converted to the supported executable contract: version=%q tasks=%d extensions=%d legacy=%d", input.Version, len(input.Tasks), len(input.Extensions), len(legacy))
	}
	result := ValidateExecutionBatchDocument(input)
	if !result.Valid || result.Counts.Tasks != 23 || result.Counts.TotalObjects > 500 {
		t.Fatalf("migration manifest fails the importer smoke test: counts=%#v issues=%#v", result.Counts, result.Issues)
	}
	if input.Planning == nil || len(input.Planning.ExecutableTaskMetadata) != 23 || len(input.Planning.DeferredItems) != 3 || len(input.Planning.ReleaseGates) != 3 {
		t.Fatalf("migration planning/deferred metadata was lost or became executable: %#v", input.Planning)
	}
	for index, metadata := range input.Planning.ExecutableTaskMetadata {
		if metadata.TaskIndex != index {
			t.Errorf("planning task mapping %d points to task index %d", index, metadata.TaskIndex)
		}
	}
	for _, task := range input.Tasks {
		if strings.Contains(task.Name, "Entra") || strings.Contains(task.Name, "LDAP") || strings.Contains(task.Name, "Teams") {
			t.Errorf("conditional provider work must remain planning-only, found executable Task %q", task.Name)
		}
	}
}
