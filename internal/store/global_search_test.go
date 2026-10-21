package store_test

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/JustSebNL/timekeeper/internal/model"
)

func TestGlobalSearchMatchesWholeWordsNotSubstrings(t *testing.T) {
	ctx := context.Background()
	database := openMessageStore(t)
	project := createMessageProject(t, database, "Cartography")
	category, err := database.CreateCategory(ctx, project, model.CreateCategoryInput{Name: "Work"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.CreateTask(ctx, project, model.CreateTaskInput{CategoryID: category.CategoryID, Name: "Startup"}); err != nil {
		t.Fatal(err)
	}
	result, err := database.SearchGlobal(ctx, model.GlobalSearchInput{Query: "art", Scopes: []string{model.GlobalSearchScopeProjects, model.GlobalSearchScopeTasks}})
	if err != nil {
		t.Fatalf("SearchGlobal: %v", err)
	}
	if result.Total != 0 || len(result.Items) != 0 {
		t.Fatalf("substring matches returned: %+v", result.Items)
	}
}

func TestGlobalSearchMessageFiltersApplyKindTagsAndDateRange(t *testing.T) {
	ctx := context.Background()
	database := openMessageStore(t)
	projectID := createMessageProject(t, database, "Filtered Search")
	decision, err := database.CreateProjectMessage(ctx, projectID, model.CreateProjectMessageInput{Kind: model.MessageKindDecision, Body: "Queue retention policy", Tags: "queue, durable"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.CreateProjectMessage(ctx, projectID, model.CreateProjectMessageInput{Kind: model.MessageKindNote, Body: "Queue reminder", Tags: "queue, routine"}); err != nil {
		t.Fatal(err)
	}
	start, end := time.Now().Add(-time.Minute), time.Now().Add(time.Minute)
	result, err := database.SearchGlobal(ctx, model.GlobalSearchInput{Query: "queue", ProjectID: projectID, Scopes: []string{model.GlobalSearchScopeMessages}, Kind: "decision", Tags: []string{"durable"}, After: &start, Before: &end, Limit: 10})
	if err != nil {
		t.Fatalf("filtered SearchGlobal: %v", err)
	}
	if result.Total != 1 || len(result.Items) != 1 || result.Items[0].Ref != decision.Ref || result.Items[0].Kind != "decision" {
		t.Fatalf("filtered message results=%+v", result)
	}
}

func TestGlobalSearchScopeProjectFilterAndLimitMetadata(t *testing.T) {
	ctx := context.Background()
	database := openMessageStore(t)
	projectA := createMessageProject(t, database, "Archive A")
	projectB := createMessageProject(t, database, "Archive B")
	for _, projectID := range []string{projectA, projectB} {
		category, err := database.CreateCategory(ctx, projectID, model.CreateCategoryInput{Name: "Work"})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := database.CreateTask(ctx, projectID, model.CreateTaskInput{CategoryID: category.CategoryID, Name: "Needle task"}); err != nil {
			t.Fatal(err)
		}
	}

	filtered, err := database.SearchGlobal(ctx, model.GlobalSearchInput{Query: "needle", ProjectID: projectB, Scopes: []string{model.GlobalSearchScopeTasks}, Limit: 1})
	if err != nil {
		t.Fatalf("filtered search: %v", err)
	}
	if filtered.Total != 1 || len(filtered.Items) != 1 || filtered.HasMore || filtered.Items[0].ProjectID != projectB || filtered.Items[0].Type != "task" {
		t.Fatalf("filtered results=%+v", filtered)
	}
	all, err := database.SearchGlobal(ctx, model.GlobalSearchInput{Query: "needle", Scopes: []string{model.GlobalSearchScopeTasks}, Limit: 1})
	if err != nil {
		t.Fatalf("limited search: %v", err)
	}
	if all.Total != 2 || len(all.Items) != 1 || !all.HasMore {
		t.Fatalf("limit metadata=%+v", all)
	}
}

func TestGlobalSearchFindsProjectsTasksAndMessagesAcrossProjects(t *testing.T) {
	ctx := context.Background()
	database := openMessageStore(t)

	createProjectWithSearchData := func(name, taskName, messageBody string) (string, string, string) {
		t.Helper()
		project, err := database.CreateProject(ctx, model.CreateProjectInput{Name: name})
		if err != nil {
			t.Fatalf("create project %q: %v", name, err)
		}
		category, err := database.CreateCategory(ctx, project.ProjectID, model.CreateCategoryInput{Name: "Search"})
		if err != nil {
			t.Fatalf("create category: %v", err)
		}
		task, err := database.CreateTask(ctx, project.ProjectID, model.CreateTaskInput{
			CategoryID:  category.CategoryID,
			Name:        taskName,
			Description: "Persist every offered job",
		})
		if err != nil {
			t.Fatalf("create task: %v", err)
		}
		message, err := database.CreateProjectMessage(ctx, project.ProjectID, model.CreateProjectMessageInput{
			Kind: model.MessageKindDecision,
			Body: messageBody,
			Tags: "queue, durable",
		})
		if err != nil {
			t.Fatalf("create message: %v", err)
		}
		return project.ProjectID, task.TaskID, message.Ref
	}

	projectA, taskA, messageA := createProjectWithSearchData("Queue Archive 🦊", "Durable Queue Runner", "Résumé: queue offers 🦊 persist before delivery.")
	projectB, taskB, messageB := createProjectWithSearchData("Work Ledger", "Queue Worker", "Queue handoff retry rules.")

	result, err := database.SearchGlobal(ctx, model.GlobalSearchInput{Query: "queue", Limit: 20})
	if err != nil {
		t.Fatalf("SearchGlobal: %v", err)
	}
	if result.Query != "queue" || result.Total != 5 || len(result.Items) != 5 || result.HasMore {
		t.Fatalf("search summary=%+v", result)
	}
	repeated, err := database.SearchGlobal(ctx, model.GlobalSearchInput{Query: "queue", Limit: 20})
	if err != nil {
		t.Fatalf("repeat SearchGlobal: %v", err)
	}
	if !reflect.DeepEqual(result.Items, repeated.Items) {
		t.Fatalf("search ranking is not deterministic:\nfirst=%+v\nsecond=%+v", result.Items, repeated.Items)
	}
	if result.Items[0].Type != "project" || result.Items[len(result.Items)-1].Type != "message" {
		t.Fatalf("search rank order=%+v", result.Items)
	}
	found := map[string]model.GlobalSearchResult{}
	for _, item := range result.Items {
		found[item.Ref] = item
		if item.ProjectID == "" || item.ProjectName == "" || item.ObjectID == "" {
			t.Errorf("search result lacks owning project/object attribution: %+v", item)
		}
	}
	for _, expected := range []struct {
		ref       string
		projectID string
		kind      string
	}{
		{ref: projectA, projectID: projectA, kind: "project"},
		{ref: taskA, projectID: projectA, kind: "task"},
		{ref: messageA, projectID: projectA, kind: "message"},
		{ref: taskB, projectID: projectB, kind: "task"},
		{ref: messageB, projectID: projectB, kind: "message"},
	} {
		item, ok := found[expected.ref]
		if !ok {
			t.Errorf("missing %s result %s in %+v", expected.kind, expected.ref, result.Items)
			continue
		}
		if item.ProjectID != expected.projectID || item.Type != expected.kind {
			t.Errorf("result %s attribution/type=%+v, want project=%s type=%s", expected.ref, item, expected.projectID, expected.kind)
		}
	}
	if item := found[messageA]; item.Snippet == "" || !strings.Contains(strings.ToLower(item.Snippet), "résumé") || !strings.Contains(item.Snippet, "🦊") {
		t.Errorf("Unicode message snippet not preserved: %+v", item)
	}
}
