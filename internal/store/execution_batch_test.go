package store_test

import (
	"context"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/JustSebNL/timekeeper/internal/model"
	"github.com/JustSebNL/timekeeper/internal/store"
)

func newExecutionBatchStoreFixture(t *testing.T) (*store.Store, context.Context, model.Project, model.Category, model.Task) {
	t.Helper()
	ctx := context.Background()
	database, err := store.Open(filepath.Join(t.TempDir(), "execution-batch.db"))
	if err != nil {
		t.Fatalf("open Store: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	project, err := database.CreateProject(ctx, model.CreateProjectInput{Name: "Execution batch test"})
	if err != nil {
		t.Fatalf("create Project: %v", err)
	}
	category, err := database.CreateCategory(ctx, project.ProjectID, model.CreateCategoryInput{Name: "Batch imports"})
	if err != nil {
		t.Fatalf("create Category: %v", err)
	}
	task, err := database.CreateTask(ctx, project.ProjectID, model.CreateTaskInput{CategoryID: category.CategoryID, Name: "Existing owner"})
	if err != nil {
		t.Fatalf("create Task: %v", err)
	}
	return database, ctx, project, category, task
}

func TestValidateExecutionBatchDoesNotWriteOrAllocateIDs(t *testing.T) {
	database, ctx, project, category, existingTask := newExecutionBatchStoreFixture(t)
	beforeTasks, err := database.ListTasks(ctx, project.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	beforeEvents, err := database.ListEventsAfterID(ctx, project.ProjectID, 0, 1000)
	if err != nil {
		t.Fatal(err)
	}
	input := model.ExecutionBatchInput{
		Version: "timekeeper-execution-batch/v1",
		Tasks:   []model.ExecutionBatchTask{{CategoryID: category.CategoryID, Name: "Validated only"}},
	}
	result, err := database.ValidateExecutionBatch(ctx, project.ProjectID, input)
	if err != nil {
		t.Fatalf("ValidateExecutionBatch: %v", err)
	}
	if !result.Valid || result.Counts.Tasks != 1 || result.Counts.TotalObjects != 1 {
		t.Fatalf("valid document should pass with one executable Task: %#v", result)
	}
	afterTasks, err := database.ListTasks(ctx, project.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	afterEvents, err := database.ListEventsAfterID(ctx, project.ProjectID, 0, 1000)
	if err != nil {
		t.Fatal(err)
	}
	if len(afterTasks) != len(beforeTasks) || len(afterEvents) != len(beforeEvents) {
		t.Fatalf("validation wrote rows/events: tasks %d->%d events %d->%d", len(beforeTasks), len(afterTasks), len(beforeEvents), len(afterEvents))
	}
	created, err := database.CreateTask(ctx, project.ProjectID, model.CreateTaskInput{CategoryID: category.CategoryID, Name: "Next real Task"})
	if err != nil {
		t.Fatalf("create Task after validation: %v", err)
	}
	previousNumber, err := strconv.ParseInt(strings.TrimPrefix(existingTask.TaskID, "T-"), 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	createdNumber, err := strconv.ParseInt(strings.TrimPrefix(created.TaskID, "T-"), 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	if createdNumber != previousNumber+1 {
		t.Fatalf("read-only validation advanced the global item ID: previous=%d created=%d", previousNumber, createdNumber)
	}
}

func TestValidateExecutionBatchReportsCategoryOutsideTargetProject(t *testing.T) {
	database, ctx, project, _, _ := newExecutionBatchStoreFixture(t)
	foreignProject, err := database.CreateProject(ctx, model.CreateProjectInput{Name: "Foreign batch project"})
	if err != nil {
		t.Fatal(err)
	}
	foreignCategory, err := database.CreateCategory(ctx, foreignProject.ProjectID, model.CreateCategoryInput{Name: "Foreign category"})
	if err != nil {
		t.Fatal(err)
	}
	result, err := database.ValidateExecutionBatch(ctx, project.ProjectID, model.ExecutionBatchInput{
		Version: "timekeeper-execution-batch/v1",
		Tasks:   []model.ExecutionBatchTask{{CategoryID: foreignCategory.CategoryID, Name: "Wrong Project"}},
	})
	if err != nil {
		t.Fatalf("ValidateExecutionBatch: %v", err)
	}
	if result.Valid || len(result.Issues) != 1 || result.Issues[0].Path != "tasks[0]" || result.Issues[0].Field != "category_id" || result.Issues[0].Suggestion == "" {
		t.Fatalf("foreign Category must yield a repair issue at tasks[0].category_id: %#v", result)
	}
}

func TestValidateExecutionBatchRejectsClosedExtensionOwner(t *testing.T) {
	database, ctx, project, _, task := newExecutionBatchStoreFixture(t)
	if _, err := database.UpdateTaskStatus(ctx, task.TaskID, model.UpdateTaskStatusInput{Status: "Completed"}); err != nil {
		t.Fatalf("complete existing Task: %v", err)
	}
	result, err := database.ValidateExecutionBatch(ctx, project.ProjectID, model.ExecutionBatchInput{
		Version:    "timekeeper-execution-batch/v1",
		Extensions: []model.ExecutionBatchExtension{{TaskID: task.TaskID, Sprints: []model.CreateSprintInput{{Name: "New work", EstimatedDurationSeconds: 600}}}},
	})
	if err != nil {
		t.Fatalf("ValidateExecutionBatch: %v", err)
	}
	if result.Valid || len(result.Issues) != 1 || result.Issues[0].Path != "extensions[0]" || result.Issues[0].Field != "task_id" || result.Issues[0].Suggestion == "" {
		t.Fatalf("closed owner must yield a repair issue at extensions[0].task_id: %#v", result)
	}
}

func TestValidateExecutionBatchRejectsAddingFifthDirectSprint(t *testing.T) {
	database, ctx, project, _, task := newExecutionBatchStoreFixture(t)
	for index := 0; index < 4; index++ {
		if _, err := database.CreateSprint(ctx, task.TaskID, model.CreateSprintInput{Name: "Existing Sprint " + strconv.Itoa(index+1), EstimatedDurationSeconds: 60}); err != nil {
			t.Fatalf("create existing Sprint %d: %v", index+1, err)
		}
	}
	result, err := database.ValidateExecutionBatch(ctx, project.ProjectID, model.ExecutionBatchInput{
		Version:    "timekeeper-execution-batch/v1",
		Extensions: []model.ExecutionBatchExtension{{TaskID: task.TaskID, Sprints: []model.CreateSprintInput{{Name: "Fifth Sprint", EstimatedDurationSeconds: 60}}}},
	})
	if err != nil {
		t.Fatalf("ValidateExecutionBatch: %v", err)
	}
	if result.Valid || len(result.Issues) != 1 || result.Issues[0].Path != "extensions[0]" || result.Issues[0].Field != "sprints" || result.Issues[0].Suggestion == "" {
		t.Fatalf("fifth direct Sprint must yield an actionable capacity issue: %#v", result)
	}
}

func TestValidateTasksBatchReportsMissingAndForeignCategories(t *testing.T) {
	database, ctx, project, _, _ := newExecutionBatchStoreFixture(t)
	foreignProject, err := database.CreateProject(ctx, model.CreateProjectInput{Name: "Foreign legacy project"})
	if err != nil {
		t.Fatal(err)
	}
	foreignCategory, err := database.CreateCategory(ctx, foreignProject.ProjectID, model.CreateCategoryInput{Name: "Foreign category"})
	if err != nil {
		t.Fatal(err)
	}
	result, err := database.ValidateTasksBatch(ctx, project.ProjectID, []model.CreateTaskInput{
		{CategoryID: "C-999999", Name: "Missing category"},
		{CategoryID: foreignCategory.CategoryID, Name: "Foreign category"},
	})
	if err != nil {
		t.Fatalf("ValidateTasksBatch: %v", err)
	}
	if result.Valid || len(result.Issues) != 2 {
		t.Fatalf("each task category error needs a repair issue: %#v", result)
	}
	for _, issue := range result.Issues {
		if issue.Field != "category_id" || issue.Suggestion == "" {
			t.Errorf("legacy task issue lacks category field/repair guidance: %#v", issue)
		}
	}
}

func TestExecutionBatchCanRegisterTaskIntoMissingDefaultCategory(t *testing.T) {
	database, ctx, project, _, _ := newExecutionBatchStoreFixture(t)
	input := model.ExecutionBatchInput{
		Version: "timekeeper-execution-batch/v1",
		Tasks:   []model.ExecutionBatchTask{{CategoryID: "C-00000", Name: "Default category Task"}},
	}
	validation, err := database.ValidateExecutionBatch(ctx, project.ProjectID, input)
	if err != nil {
		t.Fatal(err)
	}
	if !validation.Valid {
		t.Fatalf("implicit default Category should pass read-only preflight: %#v", validation.Issues)
	}
	created, validation, err := database.CreateExecutionBatch(ctx, project.ProjectID, input)
	if err != nil || !validation.Valid || len(created.Tasks) != 1 {
		t.Fatalf("default Category registration=%+v validation=%+v err=%v", created, validation, err)
	}
	if created.Tasks[0].CategoryID != "C-00000" {
		t.Fatalf("Task used Category %q, want default C-00000", created.Tasks[0].CategoryID)
	}
}

func TestExecutionBatchResolvesCategoryNameToTargetCategory(t *testing.T) {
	database, ctx, project, category, _ := newExecutionBatchStoreFixture(t)
	input := model.ExecutionBatchInput{
		Version: "timekeeper-execution-batch/v1",
		Tasks:   []model.ExecutionBatchTask{{CategoryID: category.Name, Name: "Category name Task"}},
	}
	validation, err := database.ValidateExecutionBatch(ctx, project.ProjectID, input)
	if err != nil || !validation.Valid {
		t.Fatalf("Category name preflight=%+v err=%v", validation, err)
	}
	created, validation, err := database.CreateExecutionBatch(ctx, project.ProjectID, input)
	if err != nil || !validation.Valid || len(created.Tasks) != 1 {
		t.Fatalf("Category name apply=%+v validation=%+v err=%v", created, validation, err)
	}
	if created.Tasks[0].CategoryID != category.CategoryID {
		t.Fatalf("resolved Category=%q want %q", created.Tasks[0].CategoryID, category.CategoryID)
	}
}

func TestCreateExecutionBatchCreatesNestedObjectsAndExtensions(t *testing.T) {
	database, ctx, project, category, existingTask := newExecutionBatchStoreFixture(t)
	input := model.ExecutionBatchInput{
		Version: "timekeeper-execution-batch/v1",
		Tasks: []model.ExecutionBatchTask{{
			CategoryID: category.CategoryID, Name: "New parent", Goal: "Outcome",
			Sprints:  []model.CreateSprintInput{{Name: "Parent sprint", EstimatedDurationSeconds: 600}},
			Subtasks: []model.ExecutionBatchSubtask{{Name: "Child", Sprints: []model.CreateSprintInput{{Name: "Child sprint", EstimatedDurationSeconds: 900}}}},
		}},
		Extensions: []model.ExecutionBatchExtension{{
			TaskID:   existingTask.TaskID,
			Sprints:  []model.CreateSprintInput{{Name: "Existing owner sprint", EstimatedDurationSeconds: 300}},
			Subtasks: []model.ExecutionBatchSubtask{{Name: "Extension child", Sprints: []model.CreateSprintInput{{Name: "Extension child sprint", EstimatedDurationSeconds: 180}}}},
		}},
	}
	result, validation, err := database.CreateExecutionBatch(ctx, project.ProjectID, input)
	if err != nil {
		t.Fatalf("CreateExecutionBatch: %v", err)
	}
	if !validation.Valid || len(validation.Issues) != 0 {
		t.Fatalf("valid batch returned validation issues: %#v", validation)
	}
	if result.Counts != (model.BatchCounts{Tasks: 1, Subtasks: 2, Sprints: 4, TotalObjects: 7}) {
		t.Fatalf("unexpected created counts: %#v", result.Counts)
	}
	if len(result.Tasks) != 1 || len(result.Subtasks) != 2 || len(result.Sprints) != 4 {
		t.Fatalf("expected all created records in result: %#v", result)
	}
	newTask, child := result.Tasks[0], result.Subtasks[0]
	if newTask.ProjectID != project.ProjectID || newTask.CategoryID != category.CategoryID || !strings.HasPrefix(newTask.ItemAddress, category.ItemAddress+".") {
		t.Fatalf("new Task lacks target ownership/address: %#v", newTask)
	}
	if child.TaskID != newTask.TaskID || !strings.HasPrefix(child.ItemAddress, newTask.ItemAddress+".") {
		t.Fatalf("new Subtask lacks parent/address: %#v", child)
	}
	if result.Subtasks[1].TaskID != existingTask.TaskID {
		t.Fatalf("extension Subtask does not reference its existing Task: %#v", result.Subtasks[1])
	}
	for _, sprint := range result.Sprints {
		if sprint.ProjectID != project.ProjectID || !strings.Contains(sprint.ItemAddress, ".") {
			t.Errorf("new Sprint lacks project/address: %#v", sprint)
		}
	}
}

func TestCreateExecutionBatchRevalidatesAndDoesNotWriteInvalidBatch(t *testing.T) {
	database, ctx, project, category, _ := newExecutionBatchStoreFixture(t)
	beforeTasks, err := database.ListTasks(ctx, project.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	result, validation, err := database.CreateExecutionBatch(ctx, project.ProjectID, model.ExecutionBatchInput{
		Version: "timekeeper-execution-batch/v1",
		Tasks: []model.ExecutionBatchTask{
			{CategoryID: category.CategoryID, Name: "Would be valid"},
			{CategoryID: "C-999999", Name: "Late invalid parent"},
		},
	})
	if err != nil || validation.Valid || len(validation.Issues) == 0 || result.Counts.TotalObjects != 0 {
		t.Fatalf("invalid batch must return repair issues without partial objects: result=%#v validation=%#v err=%v", result, validation, err)
	}
	afterTasks, err := database.ListTasks(ctx, project.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	if len(afterTasks) != len(beforeTasks) {
		t.Fatalf("invalid batch partially created Tasks: before=%d after=%d", len(beforeTasks), len(afterTasks))
	}
}
