package desktop

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/JustSebNL/timekeeper/internal/version"
)

func TestCoreClientReadsProjectExecutionTree(t *testing.T) {
	const projectID = "P-7"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case "/api/v1/version":
			_, _ = fmt.Fprintf(w, `{"service":"TimeKeeper","version":%q}`, version.Get().Version)
		case "/api/v1/projects/P-7/execution-tree":
			_, _ = w.Write([]byte(`{"project":{"project_id":"P-7","project_name":"Example","status":"Open"},"categories":[{"category":{"category_id":"C-2","name":"Delivery","status":"Open"},"categories":[{"category":{"category_id":"C-3","name":"Quality","status":"Open"},"categories":[],"tasks":[]}],"tasks":[{"task":{"task_id":"T-4","name":"Build","status":"Open"},"sprints":[],"subtasks":[{"subtask":{"subtask_id":"ST-5","name":"Verify","status":"Open"},"sprints":[{"sprint_id":"SP-6","name":"Run checks","status":"Open"}]}]}]}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client, err := NewCoreClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	tree, err := client.ExecutionTree(context.Background(), projectID)
	if err != nil {
		t.Fatal(err)
	}
	if tree.Project.ProjectID != projectID || len(tree.Categories) != 1 || tree.Categories[0].Category.Name != "Delivery" || tree.Categories[0].Categories[0].Category.Name != "Quality" {
		t.Fatalf("execution tree root/recursive categories=%+v", tree)
	}
	task := tree.Categories[0].Tasks[0]
	if task.Task.TaskID != "T-4" || task.Subtasks[0].Subtask.SubtaskID != "ST-5" || task.Subtasks[0].Sprints[0].SprintID != "SP-6" {
		t.Fatalf("nested execution records=%+v", task)
	}
}

func TestCoreClientExecutionTreeRejectsEmptyProjectID(t *testing.T) {
	client, err := NewCoreClient("http://127.0.0.1:1618")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.ExecutionTree(context.Background(), ""); err == nil {
		t.Fatal("empty Project ID should be rejected before HTTP request")
	}
}
