package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JustSebNL/timekeeper/internal/api"
	"github.com/JustSebNL/timekeeper/internal/instanceguard"
	"github.com/JustSebNL/timekeeper/internal/model"
	"github.com/JustSebNL/timekeeper/internal/store"
	"github.com/xuri/excelize/v2"
)

func createOfflineBatchSource(t *testing.T) (string, string, string) {
	t.Helper()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "source.db")
	database, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	project, err := database.CreateProject(ctx, model.CreateProjectInput{Name: "Offline batch source"})
	if err != nil {
		database.Close()
		t.Fatal(err)
	}
	category, err := database.CreateCategory(ctx, project.ProjectID, model.CreateCategoryInput{Name: "Batch target"})
	if err != nil {
		database.Close()
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	return path, project.ProjectID, category.CategoryID
}

func writeOfflineBatch(t *testing.T, path, categoryID string) string {
	t.Helper()
	batch := model.ExecutionBatchInput{
		Version: "timekeeper-execution-batch/v1",
		Tasks: []model.ExecutionBatchTask{{
			CategoryID: categoryID,
			Name:       "Offline batch parent",
			Subtasks: []model.ExecutionBatchSubtask{{
				Name:    "Offline batch child",
				Sprints: []model.CreateSprintInput{{Name: "Offline batch sprint", EstimatedDurationSeconds: 120}},
			}},
		}},
	}
	payload, err := json.Marshal(batch)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestOfflineBatchCheckOnlyCopiesDatabaseAndCreatesNoObjects(t *testing.T) {
	ctx := context.Background()
	source, projectID, categoryID := createOfflineBatchSource(t)
	batchPath := writeOfflineBatch(t, filepath.Join(t.TempDir(), "batch.json"), categoryID)
	output := filepath.Join(t.TempDir(), "offline-check.db")

	report, err := runOfflineBatchCopy(ctx, source, output, projectID, batchPath, "", true)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Validation.Valid || report.Validation.Counts != (model.BatchCounts{Tasks: 1, Subtasks: 1, Sprints: 1, TotalObjects: 3}) {
		t.Fatalf("offline smoke result=%+v", report.Validation)
	}
	if report.Created.Counts.TotalObjects != 0 {
		t.Fatalf("check-only created objects: %+v", report.Created.Counts)
	}

	for label, path := range map[string]string{"source": source, "output": output} {
		database, err := store.Open(path)
		if err != nil {
			t.Fatalf("open %s database: %v", label, err)
		}
		tasks, err := database.ListTasks(ctx, projectID)
		queue, queueErr := database.ListRegistrationQueue(ctx, projectID, "", 10)
		_ = database.Close()
		if err != nil || len(tasks) != 0 {
			t.Fatalf("%s tasks after check-only=%d err=%v", label, len(tasks), err)
		}
		if queueErr != nil || len(queue) != 0 {
			t.Fatalf("%s registration queue after check-only=%+v err=%v", label, queue, queueErr)
		}
	}
}

func TestOfflineBatchApplyMutatesOnlyTheOutputCopy(t *testing.T) {
	ctx := context.Background()
	source, projectID, categoryID := createOfflineBatchSource(t)
	batchPath := writeOfflineBatch(t, filepath.Join(t.TempDir(), "batch.json"), categoryID)
	output := filepath.Join(t.TempDir(), "offline-apply.db")
	sourceBefore, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}

	report, err := runOfflineBatchCopy(ctx, source, output, projectID, batchPath, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Validation.Valid || report.Created.Counts != (model.BatchCounts{Tasks: 1, Subtasks: 1, Sprints: 1, TotalObjects: 3}) {
		t.Fatalf("offline apply result validation=%+v created=%+v", report.Validation, report.Created.Counts)
	}

	sourceAfter, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(sourceBefore, sourceAfter) {
		t.Fatal("offline batch altered source database bytes")
	}

	sourceDB, err := store.Open(source)
	if err != nil {
		t.Fatal(err)
	}
	sourceTasks, sourceErr := sourceDB.ListTasks(ctx, projectID)
	_ = sourceDB.Close()
	if sourceErr != nil || len(sourceTasks) != 0 {
		t.Fatalf("source DB changed: tasks=%d err=%v", len(sourceTasks), sourceErr)
	}

	outputDB, err := store.Open(output)
	if err != nil {
		t.Fatal(err)
	}
	outputTasks, err := outputDB.ListTasks(ctx, projectID)
	if err != nil || len(outputTasks) != 1 || outputTasks[0].Name != "Offline batch parent" {
		_ = outputDB.Close()
		t.Fatalf("output tasks=%+v err=%v", outputTasks, err)
	}
	subtasks, err := outputDB.ListSubtasks(ctx, outputTasks[0].TaskID)
	if err != nil || len(subtasks) != 1 {
		_ = outputDB.Close()
		t.Fatalf("output subtasks=%d err=%v", len(subtasks), err)
	}
	sprints, err := outputDB.ListSubtaskSprints(ctx, subtasks[0].SubtaskID)
	_ = outputDB.Close()
	if err != nil || len(sprints) != 1 {
		t.Fatalf("output sprints=%d err=%v", len(sprints), err)
	}
}

func TestOfflineBatchUsesCoreAPIRoutesAndCreatesNormalTasks(t *testing.T) {
	ctx := context.Background()
	source, projectID, categoryID := createOfflineBatchSource(t)
	batchPath := writeOfflineBatch(t, filepath.Join(t.TempDir(), "batch.json"), categoryID)
	output := filepath.Join(t.TempDir(), "offline-api-copy.db")
	var requests []string
	apiFactory := func(database *store.Store) http.Handler {
		core := api.New(database)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requests = append(requests, r.Method+" "+r.URL.Path)
			core.ServeHTTP(w, r)
		})
	}

	report, err := runOfflineBatchCopyWithAPI(ctx, source, output, projectID, batchPath, "", false, apiFactory)
	if err != nil {
		t.Fatal(err)
	}
	wantRequests := []string{
		"POST /api/v1/projects/" + projectID + "/execution-batches/validate",
		"POST /api/v1/projects/" + projectID + "/execution-batches",
	}
	if len(requests) != len(wantRequests) {
		t.Fatalf("Core API request sequence=%v", requests)
	}
	for index, want := range wantRequests {
		if requests[index] != want {
			t.Fatalf("Core API request[%d]=%q, want %q; all=%v", index, requests[index], want, requests)
		}
	}
	if len(report.Created.Tasks) != 1 {
		t.Fatalf("created API Task records=%d", len(report.Created.Tasks))
	}

	outputDB, err := store.Open(output)
	if err != nil {
		t.Fatal(err)
	}
	defer outputDB.Close()
	readback := httptest.NewRecorder()
	api.New(outputDB).ServeHTTP(readback, httptest.NewRequest(http.MethodGet, "/api/v1/tasks/"+report.Created.Tasks[0].TaskID, nil))
	if readback.Code != http.StatusOK {
		t.Fatalf("ordinary Task GET status=%d body=%s", readback.Code, readback.Body.String())
	}
	var response struct {
		Task model.Task `json:"task"`
	}
	if err := json.Unmarshal(readback.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Task.TaskID != report.Created.Tasks[0].TaskID || response.Task.Name != "Offline batch parent" {
		t.Fatalf("ordinary Task API readback=%+v", response.Task)
	}
	queue, err := outputDB.ListRegistrationQueue(ctx, projectID, "", 10)
	if err != nil || len(queue) != 1 {
		t.Fatalf("output registration queue=%+v err=%v", queue, err)
	}
	if queue[0].Kind != model.RegistrationQueueKindExecutionBatch || queue[0].Status != model.RegistrationQueueStatusRegistered {
		t.Fatalf("output registration receipt summary=%+v", queue[0])
	}
	receipt, err := outputDB.GetRegistrationQueueItem(ctx, projectID, queue[0].QueueID)
	if err != nil || receipt.Result == nil || receipt.Result.Counts.TotalObjects != 3 {
		t.Fatalf("output full registration receipt=%+v err=%v", receipt, err)
	}
}

func TestOfflineBatchLegacyCSVSmokeAndApply(t *testing.T) {
	ctx := context.Background()
	source, projectID, categoryID := createOfflineBatchSource(t)
	batchPath := filepath.Join(t.TempDir(), "tasks.csv")
	data := fmt.Sprintf("name,category_id,estimate,priority\nOffline CSV task,%s,30m,High\n", categoryID)
	if err := os.WriteFile(batchPath, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "offline-csv.db")

	report, err := runOfflineBatchCopy(ctx, source, output, projectID, batchPath, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Validation.Valid || report.Validation.Counts != (model.BatchCounts{Tasks: 1, TotalObjects: 1}) {
		t.Fatalf("CSV Core smoke result=%+v", report.Validation)
	}
	if len(report.LegacyCreated) != 1 || report.LegacyCreated[0].Name != "Offline CSV task" {
		t.Fatalf("CSV apply result=%+v", report.LegacyCreated)
	}
	outputDB, err := store.Open(output)
	if err != nil {
		t.Fatal(err)
	}
	tasks, err := outputDB.ListTasks(ctx, projectID)
	_ = outputDB.Close()
	if err != nil || len(tasks) != 1 || tasks[0].Name != "Offline CSV task" {
		t.Fatalf("CSV readback tasks=%+v err=%v", tasks, err)
	}
}

func TestOfflineBatchRejectsMissingProjectBeforeCreatingBatchObjects(t *testing.T) {
	ctx := context.Background()
	source, projectID, categoryID := createOfflineBatchSource(t)
	batchPath := writeOfflineBatch(t, filepath.Join(t.TempDir(), "batch.json"), categoryID)
	output := filepath.Join(t.TempDir(), "offline-invalid-project.db")

	report, err := runOfflineBatchCopy(ctx, source, output, "P-999999", batchPath, "", false)
	if err == nil || report.Validation.Valid {
		t.Fatalf("missing Project should fail offline preflight: report=%+v err=%v", report, err)
	}
	if len(report.Validation.Issues) == 0 || report.Validation.Issues[0].Code != "project_not_found" {
		t.Fatalf("missing Project issue=%+v", report.Validation.Issues)
	}
	copyDB, openErr := store.Open(output)
	if openErr != nil {
		t.Fatal(openErr)
	}
	tasks, listErr := copyDB.ListTasks(ctx, projectID)
	_ = copyDB.Close()
	if listErr != nil || len(tasks) != 0 {
		t.Fatalf("invalid Project wrote objects: tasks=%d err=%v", len(tasks), listErr)
	}
}

func TestOfflineBatchRefusesLockedSource(t *testing.T) {
	ctx := context.Background()
	source, projectID, categoryID := createOfflineBatchSource(t)
	batchPath := writeOfflineBatch(t, filepath.Join(t.TempDir(), "batch.json"), categoryID)
	output := filepath.Join(t.TempDir(), "locked-copy.db")
	lock, err := instanceguard.Acquire(source)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()

	if _, err := runOfflineBatchCopy(ctx, source, output, projectID, batchPath, "", true); err == nil {
		t.Fatal("offline batch must refuse a source database locked by Core")
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatalf("locked-source rejection created output copy: stat err=%v", err)
	}
}

func TestOfflineBatchRefusesSidecarsAndExistingOutput(t *testing.T) {
	ctx := context.Background()
	source, projectID, categoryID := createOfflineBatchSource(t)
	batchPath := writeOfflineBatch(t, filepath.Join(t.TempDir(), "batch.json"), categoryID)

	sidecar := source + "-wal"
	if err := os.WriteFile(sidecar, []byte("uncheckpointed data"), 0o600); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "sidecar-copy.db")
	if _, err := runOfflineBatchCopy(ctx, source, output, projectID, batchPath, "", true); err == nil {
		t.Fatal("offline batch must refuse a source with WAL/SHM sidecars")
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatalf("sidecar rejection created output copy: stat err=%v", err)
	}
	if err := os.Remove(sidecar); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(output, []byte("occupied"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := runOfflineBatchCopy(ctx, source, output, projectID, batchPath, "", true); err == nil {
		t.Fatal("offline batch must refuse to overwrite an existing output database")
	}
}

func TestOfflineBatchSupportedFormatsSmokeAndApply(t *testing.T) {
	ctx := context.Background()
	source, projectID, categoryID := createOfflineBatchSource(t)
	sourceBefore, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name     string
		filename string
		nested   bool
	}{
		{name: "versioned-json", filename: "batch.json", nested: true},
		{name: "legacy-json", filename: "tasks.json"},
		{name: "versioned-yaml", filename: "batch.yaml", nested: true},
		{name: "legacy-yaml", filename: "tasks.yml"},
		{name: "csv", filename: "tasks.csv"},
		{name: "tsv", filename: "tasks.tsv"},
		{name: "xlsx", filename: "tasks.xlsx"},
		{name: "sql", filename: "tasks.sql"},
		{name: "zip", filename: "tasks.zip"},
		{name: "tar", filename: "tasks.tar"},
		{name: "tar-gz", filename: "tasks.tar.gz"},
		{name: "tgz", filename: "tasks.tgz"},
		{name: "gzip-csv", filename: "tasks.csv.gz"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			taskName := "Offline " + tc.name + " task"
			batchPath := writeOfflineFormatFixture(t, tc.filename, categoryID, taskName, tc.nested)
			output := filepath.Join(t.TempDir(), "copy.db")
			report, err := runOfflineBatchCopy(ctx, source, output, projectID, batchPath, "", false)
			if err != nil {
				t.Fatal(err)
			}
			expected := model.BatchCounts{Tasks: 1, TotalObjects: 1}
			if tc.nested {
				expected = model.BatchCounts{Tasks: 1, Subtasks: 1, Sprints: 1, TotalObjects: 3}
			}
			if !report.Validation.Valid || report.Validation.Counts != expected || report.Created.Counts != expected {
				t.Fatalf("offline result validation=%+v created=%+v expected=%+v", report.Validation, report.Created.Counts, expected)
			}
			outputDB, err := store.Open(output)
			if err != nil {
				t.Fatal(err)
			}
			tasks, err := outputDB.ListTasks(ctx, projectID)
			if err != nil || len(tasks) != 1 || tasks[0].Name != taskName {
				_ = outputDB.Close()
				t.Fatalf("offline readback tasks=%+v err=%v", tasks, err)
			}
			if tc.nested {
				subtasks, err := outputDB.ListSubtasks(ctx, tasks[0].TaskID)
				if err != nil || len(subtasks) != 1 {
					_ = outputDB.Close()
					t.Fatalf("nested readback subtasks=%d err=%v", len(subtasks), err)
				}
				sprints, err := outputDB.ListSubtaskSprints(ctx, subtasks[0].SubtaskID)
				if err != nil || len(sprints) != 1 {
					_ = outputDB.Close()
					t.Fatalf("nested readback sprints=%d err=%v", len(sprints), err)
				}
			}
			_ = outputDB.Close()
		})
	}

	sourceAfter, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(sourceBefore, sourceAfter) {
		t.Fatal("per-format offline batch smoke changed source DB bytes")
	}
}

func writeOfflineFormatFixture(t *testing.T, filename, categoryID, taskName string, nested bool) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), filename)
	csv := fmt.Sprintf("name,category_id,estimated_duration_seconds\n%s,%s,60\n", taskName, categoryID)
	if nested {
		input := model.ExecutionBatchInput{
			Version: "timekeeper-execution-batch/v1",
			Tasks: []model.ExecutionBatchTask{{
				CategoryID: categoryID,
				Name:       taskName,
				Subtasks: []model.ExecutionBatchSubtask{{
					Name:    taskName + " child",
					Sprints: []model.CreateSprintInput{{Name: taskName + " sprint", EstimatedDurationSeconds: 60}},
				}},
			}},
		}
		if filepath.Ext(filename) == ".json" {
			payload, err := json.Marshal(input)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, payload, 0o600); err != nil {
				t.Fatal(err)
			}
			return path
		}
		yaml := fmt.Sprintf("version: timekeeper-execution-batch/v1\ntasks:\n  - category_id: %s\n    name: %s\n    subtasks:\n      - name: %s child\n        sprints:\n          - name: %s sprint\n            estimated_duration_seconds: 60\n            buffer_pct: 0\n", categoryID, taskName, taskName, taskName)
		if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}

	switch filepath.Ext(filename) {
	case ".json":
		payload, err := json.Marshal(map[string]any{"tasks": []map[string]any{{"name": taskName, "category_id": categoryID, "estimated_duration_seconds": 60}}})
		if err != nil {
			t.Fatal(err)
		}
		writeOfflineFixtureBytes(t, path, payload)
	case ".yaml", ".yml":
		writeOfflineFixtureBytes(t, path, []byte(fmt.Sprintf("tasks:\n  - name: %s\n    category_id: %s\n    estimated_duration_seconds: 60\n", taskName, categoryID)))
	case ".csv":
		writeOfflineFixtureBytes(t, path, []byte(csv))
	case ".tsv":
		writeOfflineFixtureBytes(t, path, []byte(fmt.Sprintf("name	category_id	estimated_duration_seconds\n%s	%s	60\n", taskName, categoryID)))
	case ".xlsx":
		file := excelize.NewFile()
		sheet := file.GetSheetName(0)
		_ = file.SetCellValue(sheet, "A1", "name")
		_ = file.SetCellValue(sheet, "B1", "category_id")
		_ = file.SetCellValue(sheet, "C1", "estimated_duration_seconds")
		_ = file.SetCellValue(sheet, "A2", taskName)
		_ = file.SetCellValue(sheet, "B2", categoryID)
		_ = file.SetCellValue(sheet, "C2", 60)
		var payload bytes.Buffer
		if err := file.Write(&payload); err != nil {
			t.Fatal(err)
		}
		_ = file.Close()
		writeOfflineFixtureBytes(t, path, payload.Bytes())
	case ".sql":
		writeOfflineFixtureBytes(t, path, []byte(fmt.Sprintf("INSERT INTO tasks (name, category_id, estimated_duration_seconds) VALUES ('%s', '%s', 60);", taskName, categoryID)))
	case ".zip":
		var payload bytes.Buffer
		writer := zip.NewWriter(&payload)
		member, err := writer.Create("tasks.csv")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := member.Write([]byte(csv)); err != nil {
			t.Fatal(err)
		}
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}
		writeOfflineFixtureBytes(t, path, payload.Bytes())
	case ".tar", ".gz", ".tgz":
		if filepath.Ext(filename) == ".gz" && !strings.HasSuffix(filename, ".tar.gz") {
			file, err := os.Create(path)
			if err != nil {
				t.Fatal(err)
			}
			writer := gzip.NewWriter(file)
			if _, err := writer.Write([]byte(csv)); err != nil {
				t.Fatal(err)
			}
			if err := writer.Close(); err != nil {
				file.Close()
				t.Fatal(err)
			}
			if err := file.Close(); err != nil {
				t.Fatal(err)
			}
			return path
		}
		file, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		var destination io.Writer = file
		var gzipWriter *gzip.Writer
		if strings.HasSuffix(filename, ".gz") || strings.HasSuffix(filename, ".tgz") {
			gzipWriter = gzip.NewWriter(file)
			destination = gzipWriter
		}
		tarWriter := tar.NewWriter(destination)
		if err := tarWriter.WriteHeader(&tar.Header{Name: "tasks.csv", Mode: 0o600, Size: int64(len(csv))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tarWriter.Write([]byte(csv)); err != nil {
			t.Fatal(err)
		}
		if err := tarWriter.Close(); err != nil {
			t.Fatal(err)
		}
		if gzipWriter != nil {
			if err := gzipWriter.Close(); err != nil {
				t.Fatal(err)
			}
		}
		if err := file.Close(); err != nil {
			t.Fatal(err)
		}
	default:
		t.Fatalf("fixture extension not handled: %s", filename)
	}
	return path
}

func writeOfflineFixtureBytes(t *testing.T, path string, payload []byte) {
	t.Helper()
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		t.Fatal(err)
	}
}
