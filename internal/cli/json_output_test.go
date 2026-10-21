// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/JustSebNL/timekeeper/internal/api"
	"github.com/JustSebNL/timekeeper/internal/cli"
	"github.com/JustSebNL/timekeeper/internal/model"
	"github.com/JustSebNL/timekeeper/internal/store"
)

func TestCLIJSONOutputFlag(t *testing.T) {
	database, err := store.Open(filepath.Join(t.TempDir(), "timekeeper.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	server := httptest.NewServer(api.New(database))
	t.Cleanup(server.Close)

	project, err := database.CreateProject(context.Background(), model.CreateProjectInput{Name: "JSON Test Project"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	category, err := database.CreateCategory(context.Background(), project.ProjectID, model.CreateCategoryInput{Name: "Core Category"})
	if err != nil {
		t.Fatalf("create category: %v", err)
	}

	t.Run("tk list --json", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := cli.Run([]string{"list", "--json"}, &stdout, &stderr, server.URL)
		if code != 0 {
			t.Fatalf("cli list --json code = %d, err = %s", code, stderr.String())
		}
		var res struct {
			Items []model.Project `json:"items"`
		}
		if err := json.Unmarshal(stdout.Bytes(), &res); err != nil {
			t.Fatalf("unmarshal json list: %v, raw = %s", err, stdout.String())
		}
		if len(res.Items) != 1 || res.Items[0].ProjectID != project.ProjectID {
			t.Fatalf("unexpected list output: %#v", res)
		}
	})

	t.Run("tk tree --json", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := cli.Run([]string{"tree", project.ProjectID, "--json"}, &stdout, &stderr, server.URL)
		if code != 0 {
			t.Fatalf("cli tree --json code = %d, err = %s", code, stderr.String())
		}
		var tree model.ExecutionTree
		if err := json.Unmarshal(stdout.Bytes(), &tree); err != nil {
			t.Fatalf("unmarshal json tree: %v, raw = %s", err, stdout.String())
		}
		if tree.Project.ProjectID != project.ProjectID {
			t.Fatalf("unexpected tree output: %#v", tree)
		}
	})

	t.Run("tk t new --json", func(t *testing.T) {
		t.Setenv("TIMEKEEPER_STORE_STATE_DIR", t.TempDir())
		var stdout, stderr bytes.Buffer
		code := cli.Run([]string{"t", "new", project.ProjectID, category.CategoryID, "JSON Task", "30m", "--json"}, &stdout, &stderr, server.URL)
		if code != 0 {
			t.Fatalf("cli t new --json code = %d, err = %s", code, stderr.String())
		}
		var task map[string]any
		if err := json.Unmarshal(stdout.Bytes(), &task); err != nil {
			t.Fatalf("unmarshal json task: %v, raw = %s", err, stdout.String())
		}
		if task["task_id"] != "T-10002" || task["name"] != "JSON Task" {
			t.Fatalf("unexpected task output: %#v", task)
		}
	})
}
