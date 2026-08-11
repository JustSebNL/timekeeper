// Copyright (c) 2026 Seb. All rights reserved.

package store_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JustSebNL/timekeeper/internal/model"
	"github.com/JustSebNL/timekeeper/internal/store"
)

func TestProjectExecutionTreeNestsChildCategories(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(filepath.Join(t.TempDir(), "timekeeper.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	project, _ := database.CreateProject(ctx, model.CreateProjectInput{Name: "Nested tree"})
	parent, _ := database.CreateCategory(ctx, project.ProjectID, model.CreateCategoryInput{Name: "Backend"})
	child, _ := database.CreateCategory(ctx, project.ProjectID, model.CreateCategoryInput{Name: "Database", ParentCategoryID: parent.CategoryID})
	tree, err := database.ProjectExecutionTree(ctx, project.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	if len(tree.Categories) != 1 || tree.Categories[0].Category.CategoryID != parent.CategoryID || len(tree.Categories[0].Categories) != 1 || tree.Categories[0].Categories[0].Category.CategoryID != child.CategoryID {
		t.Fatalf("tree=%#v", tree)
	}
}

func TestCreateCategoryTrimsParentCategoryID(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(filepath.Join(t.TempDir(), "timekeeper.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	project, err := database.CreateProject(ctx, model.CreateProjectInput{Name: "Trim category parent"})
	if err != nil {
		t.Fatal(err)
	}
	parent, err := database.CreateCategory(ctx, project.ProjectID, model.CreateCategoryInput{Name: "Backend"})
	if err != nil {
		t.Fatal(err)
	}
	child, err := database.CreateCategory(ctx, project.ProjectID, model.CreateCategoryInput{Name: "Database", ParentCategoryID: "  " + parent.CategoryID + "\t"})
	if err != nil {
		t.Fatal(err)
	}
	if child.ParentCategoryID != parent.CategoryID {
		t.Fatalf("parent=%q want=%q", child.ParentCategoryID, parent.CategoryID)
	}
}

func TestCreateCategoryAllowsParentInSameProject(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(filepath.Join(t.TempDir(), "timekeeper.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	project, err := database.CreateProject(ctx, model.CreateProjectInput{Name: "Nested categories"})
	if err != nil {
		t.Fatal(err)
	}
	parent, err := database.CreateCategory(ctx, project.ProjectID, model.CreateCategoryInput{Name: "Backend"})
	if err != nil {
		t.Fatal(err)
	}
	child, err := database.CreateCategory(ctx, project.ProjectID, model.CreateCategoryInput{Name: "Database", ParentCategoryID: parent.CategoryID})
	if err != nil {
		t.Fatal(err)
	}
	if child.ParentCategoryID != parent.CategoryID {
		t.Fatalf("parent=%q want=%q", child.ParentCategoryID, parent.CategoryID)
	}
}

func TestNestedCategoryItemAddressIncludesParentAddress(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(filepath.Join(t.TempDir(), "timekeeper.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	project, err := database.CreateProject(ctx, model.CreateProjectInput{Name: "Nested address"})
	if err != nil {
		t.Fatal(err)
	}
	parent, err := database.CreateCategory(ctx, project.ProjectID, model.CreateCategoryInput{Name: "Backend"})
	if err != nil {
		t.Fatal(err)
	}
	child, err := database.CreateCategory(ctx, project.ProjectID, model.CreateCategoryInput{Name: "Database", ParentCategoryID: parent.CategoryID})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(child.ItemAddress, parent.ItemAddress+".") {
		t.Fatalf("child address=%q must include parent address=%q", child.ItemAddress, parent.ItemAddress)
	}
}
