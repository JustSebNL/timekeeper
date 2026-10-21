package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/JustSebNL/timekeeper/internal/api"
	"github.com/JustSebNL/timekeeper/internal/cli"
	"github.com/JustSebNL/timekeeper/internal/instanceguard"
	"github.com/JustSebNL/timekeeper/internal/model"
	"github.com/JustSebNL/timekeeper/internal/store"
)

type offlineBatchReport struct {
	SourceDB            string
	CopyDB              string
	SourceSHA256        string
	CopyBeforeBatchHash string
	Validation          model.BatchValidationResult
	Created             model.ExecutionBatchResult
	LegacyCreated       []model.Task
	CheckOnly           bool
}

type offlineBatchAPIFactory func(*store.Store) http.Handler

// runOfflineBatchCopy uses the same Core API routes as the online CLI but
// dispatches them in-process, without starting an HTTP listener.
func runOfflineBatchCopy(ctx context.Context, sourceDB, outputDB, projectID, batchFile, format string, checkOnly bool) (offlineBatchReport, error) {
	return runOfflineBatchCopyWithAPI(ctx, sourceDB, outputDB, projectID, batchFile, format, checkOnly, api.New)
}

func runOfflineBatchCopyWithAPI(ctx context.Context, sourceDB, outputDB, projectID, batchFile, format string, checkOnly bool, apiFactory offlineBatchAPIFactory) (offlineBatchReport, error) {
	report := offlineBatchReport{CheckOnly: checkOnly}
	if strings.TrimSpace(projectID) == "" || strings.TrimSpace(batchFile) == "" || strings.TrimSpace(outputDB) == "" {
		return report, errors.New("offline batch requires an explicit Project ID, batch file, and new output database path")
	}
	input, legacyTasks, err := cli.ParseExecutionBatch(batchFile, format)
	if err != nil {
		return report, fmt.Errorf("parse offline batch: %w", err)
	}
	isNested := strings.TrimSpace(input.Version) != ""
	if isNested {
		local := model.ValidateExecutionBatchDocument(input)
		if !local.Valid {
			report.Validation = local
			return report, fmt.Errorf("batch document validation failed: %s", formatBatchIssues(local.Issues))
		}
	}

	sourcePath, err := canonicalExistingPath(sourceDB)
	if err != nil {
		return report, fmt.Errorf("resolve source database: %w", err)
	}
	outputPath, err := canonicalNewPath(outputDB)
	if err != nil {
		return report, fmt.Errorf("resolve output database: %w", err)
	}
	if sameFilesystemPath(sourcePath, outputPath) {
		return report, errors.New("offline batch output must be a different file from the source database")
	}
	report.SourceDB, report.CopyDB = sourcePath, outputPath
	if err := requireNoSQLiteSidecars(sourcePath); err != nil {
		return report, err
	}
	if err := requireNoSQLiteSidecars(outputPath); err != nil {
		return report, err
	}
	if _, err := os.Lstat(outputPath); err == nil {
		return report, fmt.Errorf("offline batch output already exists; refusing to overwrite: %s", outputPath)
	} else if !errors.Is(err, os.ErrNotExist) {
		return report, fmt.Errorf("inspect offline batch output: %w", err)
	}

	lock, err := instanceguard.Acquire(sourcePath)
	if err != nil {
		return report, fmt.Errorf("offline batch requires an unlocked, stopped Core database: %w", err)
	}
	defer lock.Close()

	// Validate the hash-matched copy, not the source. Opening a WAL-mode
	// source read-only can create SQLite sidecars beside the input file.
	sourceHash, err := hashDatabaseFile(sourcePath)
	if err != nil {
		return report, fmt.Errorf("hash source database: %w", err)
	}
	report.SourceSHA256 = sourceHash
	if err := copyDatabaseExclusive(sourcePath, outputPath); err != nil {
		return report, fmt.Errorf("create isolated database copy: %w", err)
	}
	copyHash, err := hashDatabaseFile(outputPath)
	if err != nil {
		_ = os.Remove(outputPath)
		return report, fmt.Errorf("hash isolated database copy: %w", err)
	}
	report.CopyBeforeBatchHash = copyHash
	sourceHashAfterCopy, err := hashDatabaseFile(sourcePath)
	if err != nil {
		_ = os.Remove(outputPath)
		return report, fmt.Errorf("recheck source database hash: %w", err)
	}
	if sourceHashAfterCopy != sourceHash || copyHash != sourceHash {
		_ = os.Remove(outputPath)
		return report, errors.New("source database changed during copy or output bytes differ; discarded the untrusted copy")
	}
	if err := store.CheckDatabaseFileIntegrity(ctx, outputPath); err != nil {
		_ = os.Remove(outputPath)
		return report, fmt.Errorf("isolated copy failed read-only integrity check: %w", err)
	}

	database, err := store.Open(outputPath)
	if err != nil {
		return report, fmt.Errorf("open isolated Core database copy: %w", err)
	}
	defer func() {
		if database != nil {
			_ = database.Close()
		}
	}()

	handler := apiFactory(database)
	validationPath, applyPath, body, err := offlineBatchAPIPayload(projectID, input, legacyTasks, isNested)
	if err != nil {
		return report, err
	}
	report.Validation, err = validateOfflineBatchThroughAPI(ctx, handler, validationPath, body)
	if err != nil {
		return report, fmt.Errorf("Core API batch smoke validation failed: %w", err)
	}
	if !report.Validation.Valid {
		return report, fmt.Errorf("Core API batch smoke validation failed: %s", formatBatchIssues(report.Validation.Issues))
	}
	if !checkOnly {
		if isNested {
			created, validation, applyErr := applyExecutionBatchThroughAPI(ctx, handler, applyPath, body)
			if validation != nil {
				report.Validation = *validation
			}
			if applyErr != nil {
				return report, fmt.Errorf("apply execution batch through Core API to isolated copy: %w", applyErr)
			}
			report.Created = created
		} else {
			report.LegacyCreated, err = applyLegacyTaskBatchThroughAPI(ctx, handler, applyPath, body)
			if err != nil {
				return report, fmt.Errorf("apply legacy task batch through Core API to isolated copy: %w", err)
			}
			report.Created.Counts = report.Validation.Counts
			report.Created.Tasks = report.LegacyCreated
		}
	}
	if err := database.Close(); err != nil {
		return report, fmt.Errorf("close isolated Core database copy: %w", err)
	}
	database = nil
	if err := requireNoSQLiteSidecars(outputPath); err != nil {
		return report, fmt.Errorf("isolated output did not close to a standalone SQLite file: %w", err)
	}
	if err := store.CheckDatabaseFileIntegrity(ctx, outputPath); err != nil {
		return report, fmt.Errorf("isolated output database failed post-run integrity check: %w", err)
	}
	sourceHashAfterRun, err := hashDatabaseFile(sourcePath)
	if err != nil {
		return report, fmt.Errorf("verify source database remained unchanged: %w", err)
	}
	if sourceHashAfterRun != sourceHash {
		return report, errors.New("source database bytes changed during offline batch; output copy was retained for inspection")
	}
	return report, nil
}

func offlineBatchAPIPayload(projectID string, input model.ExecutionBatchInput, legacyTasks []model.CreateTaskInput, nested bool) (string, string, []byte, error) {
	projectPath := "/api/v1/projects/" + url.PathEscape(projectID)
	if nested {
		body, err := json.Marshal(input)
		return projectPath + "/execution-batches/validate", projectPath + "/execution-batches", body, err
	}
	body, err := json.Marshal(model.CreateTasksBatchInput{Tasks: legacyTasks})
	return projectPath + "/tasks/batch/validate", projectPath + "/tasks/batch", body, err
}

func validateOfflineBatchThroughAPI(ctx context.Context, handler http.Handler, endpoint string, body []byte) (model.BatchValidationResult, error) {
	recorder := invokeCoreAPIInProcess(ctx, handler, http.MethodPost, endpoint, body)
	if recorder.Code != http.StatusOK {
		return model.BatchValidationResult{}, fmt.Errorf("validation API returned HTTP %d: %s", recorder.Code, strings.TrimSpace(recorder.Body.String()))
	}
	var validation model.BatchValidationResult
	if err := json.Unmarshal(recorder.Body.Bytes(), &validation); err != nil {
		return model.BatchValidationResult{}, fmt.Errorf("decode validation API response: %w", err)
	}
	return validation, nil
}

func applyExecutionBatchThroughAPI(ctx context.Context, handler http.Handler, endpoint string, body []byte) (model.ExecutionBatchResult, *model.BatchValidationResult, error) {
	recorder := invokeCoreAPIInProcess(ctx, handler, http.MethodPost, endpoint, body)
	if recorder.Code == http.StatusUnprocessableEntity {
		var validation model.BatchValidationResult
		if err := json.Unmarshal(recorder.Body.Bytes(), &validation); err != nil {
			return model.ExecutionBatchResult{}, nil, fmt.Errorf("apply-time validation returned HTTP 422 with invalid JSON: %w", err)
		}
		return model.ExecutionBatchResult{}, &validation, fmt.Errorf("apply-time Core API revalidation failed: %s", formatBatchIssues(validation.Issues))
	}
	if recorder.Code != http.StatusCreated {
		return model.ExecutionBatchResult{}, nil, fmt.Errorf("apply API returned HTTP %d: %s", recorder.Code, strings.TrimSpace(recorder.Body.String()))
	}
	var created model.ExecutionBatchResult
	if err := json.Unmarshal(recorder.Body.Bytes(), &created); err != nil {
		return model.ExecutionBatchResult{}, nil, fmt.Errorf("decode apply API response: %w", err)
	}
	validation := model.BatchValidationResult{Valid: true, Counts: created.Counts}
	return created, &validation, nil
}

func applyLegacyTaskBatchThroughAPI(ctx context.Context, handler http.Handler, endpoint string, body []byte) ([]model.Task, error) {
	recorder := invokeCoreAPIInProcess(ctx, handler, http.MethodPost, endpoint, body)
	if recorder.Code != http.StatusCreated {
		return nil, fmt.Errorf("legacy apply API returned HTTP %d: %s", recorder.Code, strings.TrimSpace(recorder.Body.String()))
	}
	var created struct {
		Tasks []model.Task `json:"tasks"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &created); err != nil {
		return nil, fmt.Errorf("decode legacy apply API response: %w", err)
	}
	return created.Tasks, nil
}

func invokeCoreAPIInProcess(ctx context.Context, handler http.Handler, method, endpoint string, body []byte) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, endpoint, bytes.NewReader(body)).WithContext(ctx)
	request.RemoteAddr = "127.0.0.1:0" // Indicates a local in-process dispatch; no socket exists.
	if len(body) != 0 {
		request.Header.Set("Content-Type", "application/json")
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

func canonicalExistingPath(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", errors.New("provide an explicit source database path")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("source database must be a regular standalone file")
	}
	return filepath.Clean(resolved), nil
}

func canonicalNewPath(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", errors.New("provide a new output database path")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(absolute))
	if err != nil {
		return "", fmt.Errorf("output parent directory must already exist: %w", err)
	}
	info, err := os.Stat(parent)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", errors.New("output parent path is not a directory")
	}
	return filepath.Join(parent, filepath.Base(absolute)), nil
}

func sameFilesystemPath(left, right string) bool {
	left, right = filepath.Clean(left), filepath.Clean(right)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(left, right)
	}
	return left == right
}

func requireNoSQLiteSidecars(databasePath string) error {
	for _, suffix := range []string{"-wal", "-shm", "-journal"} {
		path := databasePath + suffix
		if _, err := os.Lstat(path); err == nil {
			return fmt.Errorf("refusing standalone copy because SQLite sidecar exists: %s", path)
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("inspect SQLite sidecar %s: %w", path, err)
		}
	}
	return nil
}

func copyDatabaseExclusive(sourcePath, outputPath string) (err error) {
	source, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer source.Close()
	output, err := os.OpenFile(outputPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := output.Close(); err == nil && closeErr != nil {
			err = closeErr
		}
		if err != nil {
			_ = os.Remove(outputPath)
		}
	}()
	if _, err = io.Copy(output, source); err != nil {
		return err
	}
	if err = output.Sync(); err != nil {
		return err
	}
	return nil
}

func hashDatabaseFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}

func formatBatchIssues(issues []model.ValidationIssue) string {
	if len(issues) == 0 {
		return "no diagnostic details returned"
	}
	parts := make([]string, 0, len(issues))
	for _, issue := range issues {
		parts = append(parts, fmt.Sprintf("%s (%s): %s", issue.Code, issue.Field, issue.Message))
	}
	return strings.Join(parts, "; ")
}
