package cli_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/JustSebNL/timekeeper/internal/cli"
	"github.com/JustSebNL/timekeeper/internal/model"
)

func TestTreeDepthLimitStopsBeforeSprintAndSubtaskDetails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"project":{"project_id":"P-10000","project_name":"Atlas"},"categories":[{"category":{"category_id":"C-1","name":"Backend"},"categories":[],"tasks":[{"task":{"task_id":"T-1","name":"Queue owner","status":"Open"},"sprints":[{"sprint_id":"SP-1","name":"Direct sprint","status":"Open"}],"subtasks":[{"subtask":{"subtask_id":"ST-1","name":"Nested owner","status":"Open"},"sprints":[{"sprint_id":"SP-2","name":"Nested sprint","status":"Open"}]}]}]}]}`))
	}))
	defer server.Close()
	var out, errOut bytes.Buffer
	code := cli.Run([]string{"tree", "P-10000", "--depth", "2"}, &out, &errOut, server.URL)
	if code != 0 {
		t.Fatalf("tree code=%d stdout=%q stderr=%q", code, out.String(), errOut.String())
	}
	for _, want := range []string{"Backend", "T-1", "Queue owner"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("depth-limited tree missing %q: %q", want, out.String())
		}
	}
	for _, unwanted := range []string{"SP-1", "ST-1", "SP-2", "Direct sprint", "Nested owner", "Nested sprint"} {
		if strings.Contains(out.String(), unwanted) {
			t.Errorf("depth-limited tree leaked %q: %q", unwanted, out.String())
		}
	}
}

func TestTreeActiveFilterKeepsOnlyActiveSprintsAndOwnerPath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"project":{"project_id":"P-10000","project_name":"Atlas"},"categories":[{"category":{"category_id":"C-1","name":"Backend"},"categories":[],"tasks":[{"task":{"task_id":"T-1","name":"Active owner","status":"Open"},"sprints":[{"sprint_id":"SP-1","name":"Active sprint","status":"Active"},{"sprint_id":"SP-2","name":"Queued sprint","status":"Open"}],"subtasks":[{"subtask":{"subtask_id":"ST-1","name":"Nested owner","status":"Open"},"sprints":[{"sprint_id":"SP-3","name":"Nested active sprint","status":"Active"}]}]},{"task":{"task_id":"T-2","name":"Inactive owner","status":"Open"},"sprints":[{"sprint_id":"SP-4","name":"Held sprint","status":"On Hold"}],"subtasks":[]}]}]}`))
	}))
	defer server.Close()
	var out, errOut bytes.Buffer
	code := cli.Run([]string{"tree", "P-10000", "--active"}, &out, &errOut, server.URL)
	if code != 0 {
		t.Fatalf("tree code=%d stdout=%q stderr=%q", code, out.String(), errOut.String())
	}
	for _, want := range []string{"Backend", "T-1", "SP-1", "SP-3", "Nested owner"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("active tree missing %q: %q", want, out.String())
		}
	}
	for _, unwanted := range []string{"T-2", "SP-2", "SP-4", "Queued sprint", "Held sprint", "Inactive owner"} {
		if strings.Contains(out.String(), unwanted) {
			t.Errorf("active tree leaked %q: %q", unwanted, out.String())
		}
	}
}

func TestTreeLimitShowsStablePrefixAndTruncationMarker(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"project":{"project_id":"P-10000","project_name":"Atlas"},"categories":[{"category":{"category_id":"C-1","name":"Backend"},"categories":[],"tasks":[{"task":{"task_id":"T-1","name":"First","status":"Open"},"sprints":[],"subtasks":[]},{"task":{"task_id":"T-2","name":"Second","status":"Open"},"sprints":[],"subtasks":[]},{"task":{"task_id":"T-3","name":"Third","status":"Open"},"sprints":[],"subtasks":[]}]}]}`))
	}))
	defer server.Close()
	var out, errOut bytes.Buffer
	code := cli.Run([]string{"tree", "P-10000", "--limit", "2"}, &out, &errOut, server.URL)
	if code != 0 {
		t.Fatalf("tree code=%d stdout=%q stderr=%q", code, out.String(), errOut.String())
	}
	if !strings.Contains(out.String(), "T-1") || !strings.Contains(out.String(), "T-2") || strings.Contains(out.String(), "T-3") || !strings.Contains(out.String(), "truncated: showing first 2 of 3") {
		t.Fatalf("limited tree output=%q", out.String())
	}
	out.Reset()
	errOut.Reset()
	if code := cli.Run([]string{"--json", "tree", "P-10000", "--limit", "2"}, &out, &errOut, server.URL); code != 0 {
		t.Fatalf("JSON limited tree code=%d stdout=%q stderr=%q", code, out.String(), errOut.String())
	}
	var page struct {
		Tree       model.ExecutionTree `json:"tree"`
		Limit      int                 `json:"limit"`
		TotalTasks int                 `json:"total_tasks"`
		ShownTasks int                 `json:"shown_tasks"`
		Truncated  bool                `json:"truncated"`
	}
	if err := json.Unmarshal(out.Bytes(), &page); err != nil {
		t.Fatalf("decode JSON tree page: %v", err)
	}
	if page.Limit != 2 || page.TotalTasks != 3 || page.ShownTasks != 2 || !page.Truncated || len(page.Tree.Categories[0].Tasks) != 2 {
		t.Fatalf("limited JSON metadata=%+v", page)
	}
}

func TestTreeStatusFilterKeepsOnlyMatchingTasksAndOwnerCategories(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"project":{"project_id":"P-10000","project_name":"Atlas"},"categories":[{"category":{"category_id":"C-1","name":"Backend"},"categories":[],"tasks":[{"task":{"task_id":"T-1","name":"Open task","status":"Open"},"sprints":[],"subtasks":[]},{"task":{"task_id":"T-2","name":"Completed task","status":"Completed"},"sprints":[],"subtasks":[]}]},{"category":{"category_id":"C-2","name":"Frontend"},"categories":[],"tasks":[{"task":{"task_id":"T-3","name":"Another open task","status":"Open"},"sprints":[],"subtasks":[]}]}]}`))
	}))
	defer server.Close()
	var out, errOut bytes.Buffer
	code := cli.Run([]string{"tree", "P-10000", "--status", "open"}, &out, &errOut, server.URL)
	if code != 0 {
		t.Fatalf("tree code=%d stdout=%q stderr=%q", code, out.String(), errOut.String())
	}
	for _, want := range []string{"Backend", "T-1", "Open task", "Frontend", "T-3", "Another open task"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("status-filtered tree missing %q: %q", want, out.String())
		}
	}
	for _, unwanted := range []string{"T-2", "Completed task"} {
		if strings.Contains(out.String(), unwanted) {
			t.Errorf("status-filtered tree leaked %q: %q", unwanted, out.String())
		}
	}
}

func TestTreeTaskFilterSupportsJSONAndReportsMissingTask(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"project":{"project_id":"P-10000","project_name":"Atlas"},"categories":[{"category":{"category_id":"C-1","name":"Backend"},"categories":[],"tasks":[{"task":{"task_id":"T-2","name":"Queue delivery","status":"Open"},"sprints":[],"subtasks":[]},{"task":{"task_id":"T-3","name":"Other work","status":"Open"},"sprints":[],"subtasks":[]}]}]}`))
	}))
	defer server.Close()
	var out, errOut bytes.Buffer
	if code := cli.Run([]string{"--json", "tree", "P-10000", "--task", "T-2"}, &out, &errOut, server.URL); code != 0 {
		t.Fatalf("JSON tree code=%d stdout=%q stderr=%q", code, out.String(), errOut.String())
	}
	var tree model.ExecutionTree
	if err := json.Unmarshal(out.Bytes(), &tree); err != nil {
		t.Fatalf("decode tree: %v", err)
	}
	if len(tree.Categories) != 1 || len(tree.Categories[0].Tasks) != 1 || tree.Categories[0].Tasks[0].Task.TaskID != "T-2" {
		t.Fatalf("filtered JSON tree=%+v", tree)
	}
	out.Reset()
	errOut.Reset()
	if code := cli.Run([]string{"tree", "P-10000", "--task", "T-404"}, &out, &errOut, server.URL); code != 1 || !strings.Contains(errOut.String(), "Task T-404 was not found") {
		t.Fatalf("missing task result code=%d stdout=%q stderr=%q", code, out.String(), errOut.String())
	}
}

func TestTreeTaskFilterKeepsTheOwnerPathAndHidesUnrelatedWork(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet || request.URL.Path != "/api/v1/projects/P-10000/execution-tree" {
			t.Fatalf("request=%s %s", request.Method, request.URL.Path)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"project":{"project_id":"P-10000","project_name":"Atlas","calculated_completion_pct":50},"categories":[{"category":{"category_id":"C-1","name":"Backend"},"categories":[],"tasks":[{"task":{"task_id":"T-1","name":"Old work","status":"Completed"},"sprints":[],"subtasks":[]},{"task":{"task_id":"T-2","name":"Queue delivery","status":"Open"},"sprints":[],"subtasks":[]}]},{"category":{"category_id":"C-2","name":"Unrelated"},"categories":[],"tasks":[{"task":{"task_id":"T-3","name":"Other work","status":"Open"},"sprints":[],"subtasks":[]}]}]}`))
	}))
	defer server.Close()
	var out, errOut bytes.Buffer
	code := cli.Run([]string{"tree", "P-10000", "--task", "T-2"}, &out, &errOut, server.URL)
	if code != 0 {
		t.Fatalf("tree code=%d stdout=%q stderr=%q", code, out.String(), errOut.String())
	}
	if !strings.Contains(out.String(), "Backend") || !strings.Contains(out.String(), "T-2") || !strings.Contains(out.String(), "Queue delivery") {
		t.Fatalf("filtered tree lost target path: %q", out.String())
	}
	for _, unwanted := range []string{"T-1", "T-3", "Old work", "Other work", "Unrelated"} {
		if strings.Contains(out.String(), unwanted) {
			t.Errorf("filtered tree leaked %q: %q", unwanted, out.String())
		}
	}
}

func TestTreeRequestsServerFiltersAndUsesServerTruncationMetadata(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/v1/projects/P-10000/execution-tree" {
			t.Errorf("path=%q", request.URL.Path)
		}
		query := request.URL.Query()
		for key, want := range map[string]string{"task_id": "T-2", "status": "Open", "active": "true", "depth": "4", "limit": "1"} {
			if got := query.Get(key); got != want {
				t.Errorf("query %s=%q, want %q", key, got, want)
			}
		}
		writer.Header().Set("Content-Type", "application/json")
		writer.Header().Set("X-TimeKeeper-Execution-Tree-Total-Tasks", "4")
		writer.Header().Set("X-TimeKeeper-Execution-Tree-Shown-Tasks", "1")
		writer.Header().Set("X-TimeKeeper-Execution-Tree-Truncated", "true")
		_, _ = writer.Write([]byte(`{"project":{"project_id":"P-10000","project_name":"Atlas"},"categories":[{"category":{"category_id":"C-1","name":"Backend"},"categories":[],"tasks":[{"task":{"task_id":"T-2","name":"Queue owner","status":"Open"},"sprints":[{"sprint_id":"SP-2","name":"Active Sprint","status":"Active"}],"subtasks":[]}]}]}`))
	}))
	defer server.Close()
	var out, errOut bytes.Buffer
	code := cli.Run([]string{"tree", "P-10000", "--task", "T-2", "--status", "Open", "--active", "--depth", "4", "--limit", "1"}, &out, &errOut, server.URL)
	if code != 0 {
		t.Fatalf("tree code=%d stdout=%q stderr=%q", code, out.String(), errOut.String())
	}
	if !strings.Contains(out.String(), "T-2") || !strings.Contains(out.String(), "SP-2") || !strings.Contains(out.String(), "truncated: showing first 1 of 4 Task matches (limit 1)") {
		t.Fatalf("server-filtered tree output=%q", out.String())
	}
}
