// Copyright (c) 2026 https://github.com/JustSebNL/timekeeper. All rights reserved.

package store

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JustSebNL/timekeeper/internal/model"
)

func TestCreateProjectGemAutoCreatesCategoryAndAttachesChild(t *testing.T) {
	database, err := Open(filepath.Join(t.TempDir(), "timekeeper.db"))
	if err != nil { t.Fatal(err) }
	t.Cleanup(func() { _ = database.Close() })
	ctx := context.Background()
	project, err := database.CreateProject(ctx, model.CreateProjectInput{Name: "Treasure Chest Project"})
	if err != nil { t.Fatal(err) }

	root, err := database.CreateProjectGem(ctx, project.ProjectID, model.CreateProjectGemInput{
		Category: "Research", Subcategory: "Retrieval", Name: "Retry budget", Content: "Monotonic deadlines reduce retry storms.",
	})
	if err != nil { t.Fatal(err) }
	if root.Ref == "" || root.GemID < 1 || root.Category != "Research" || root.Subcategory != "Retrieval" || root.Revision != 1 {
		t.Fatalf("root gem=%#v", root)
	}
	child, err := database.CreateProjectGem(ctx, project.ProjectID, model.CreateProjectGemInput{
		ParentGemID: &root.GemID, Name: "Counterexample", Content: "Fixed delays can synchronize clients.",
	})
	if err != nil { t.Fatal(err) }
	if child.ParentGemID == nil || *child.ParentGemID != root.GemID || child.Category != root.Category || child.Subcategory != root.Subcategory {
		t.Fatalf("child=%#v parent=%#v", child, root)
	}
	if _, err := database.CreateProjectGem(ctx, project.ProjectID, model.CreateProjectGemInput{
		Category: "research", Subcategory: "retrieval", Name: "Case-insensitive label", Content: "The label path should not duplicate.",
	}); err != nil { t.Fatal(err) }
	categories, err := database.ListProjectGemCategories(ctx, project.ProjectID)
	if err != nil { t.Fatal(err) }
	if len(categories) != 1 || !strings.EqualFold(categories[0].Category, "Research") || len(categories[0].Subcategories) != 1 || !strings.EqualFold(categories[0].Subcategories[0], "Retrieval") {
		t.Fatalf("categories=%#v", categories)
	}
	filtered, err := database.ListProjectGems(ctx, project.ProjectID, model.ProjectGemFilter{Category: "RESEARCH", Subcategory: "RETRIEVAL", Limit: 10})
	if err != nil { t.Fatal(err) }
	if len(filtered.Items) != 3 { t.Fatalf("case-insensitive category filter returned %#v", filtered.Items) }
}

func TestProjectGemRejectsCrossProjectAttachment(t *testing.T) {
	database, err := Open(filepath.Join(t.TempDir(), "timekeeper.db"))
	if err != nil { t.Fatal(err) }
	t.Cleanup(func() { _ = database.Close() })
	ctx := context.Background()
	first, err := database.CreateProject(ctx, model.CreateProjectInput{Name: "First Gem Project"})
	if err != nil { t.Fatal(err) }
	second, err := database.CreateProject(ctx, model.CreateProjectInput{Name: "Second Gem Project"})
	if err != nil { t.Fatal(err) }
	parent, err := database.CreateProjectGem(ctx, first.ProjectID, model.CreateProjectGemInput{Category: "Patterns", Name: "Parent", Content: "Project-scoped insight."})
	if err != nil { t.Fatal(err) }
	_, err = database.CreateProjectGem(ctx, second.ProjectID, model.CreateProjectGemInput{ParentGemID: &parent.GemID, Name: "Invalid child", Content: "Must stay in one Project."})
	if err == nil { t.Fatal("cross-project attachment unexpectedly succeeded") }
}

func TestProjectGemEditPinDeleteRestoreAndRevisionHistory(t *testing.T) {
	database, err := Open(filepath.Join(t.TempDir(), "timekeeper.db"))
	if err != nil { t.Fatal(err) }
	t.Cleanup(func() { _ = database.Close() })
	ctx := context.Background()
	project, err := database.CreateProject(ctx, model.CreateProjectInput{Name: "Gem Lifecycle"})
	if err != nil { t.Fatal(err) }
	gem, err := database.CreateProjectGem(ctx, project.ProjectID, model.CreateProjectGemInput{Category: "Patterns", Name: "Retry budget", Content: "Original content."})
	if err != nil { t.Fatal(err) }
	updated, err := database.UpdateProjectGem(ctx, project.ProjectID, gem.GemID, model.UpdateProjectGemInput{Category: "Patterns", Name: "Retry budget v2", Content: "Edited content."})
	if err != nil { t.Fatal(err) }
	if updated.Name != "Retry budget v2" || updated.Content != "Edited content." || updated.Revision != 2 { t.Fatalf("updated=%#v", updated) }
	pinned, err := database.SetProjectGemPinned(ctx, gem.ProjectID, gem.GemID, true)
	if err != nil { t.Fatal(err) }
	if !pinned.Pinned { t.Fatalf("gem not pinned: %#v", pinned) }
	other, err := database.CreateProjectGem(ctx, project.ProjectID, model.CreateProjectGemInput{Category: "Patterns", Name: "Ordinary gem", Content: "Less urgent."})
	if err != nil { t.Fatal(err) }
	ordered, err := database.ListProjectGems(ctx, project.ProjectID, model.ProjectGemFilter{Limit: 10})
	if err != nil { t.Fatal(err) }
	if len(ordered.Items) != 2 || ordered.Items[0].GemID != gem.GemID || !ordered.Items[0].Pinned || ordered.Items[1].GemID != other.GemID {
		t.Fatalf("pin-prioritized list=%#v", ordered.Items)
	}
	if _, err := database.DeleteProjectGem(ctx, project.ProjectID, gem.GemID); err != nil { t.Fatal(err) }
	active, err := database.ListProjectGems(ctx, project.ProjectID, model.ProjectGemFilter{})
	if err != nil { t.Fatal(err) }
	if len(active.Items) != 1 || active.Items[0].GemID != other.GemID { t.Fatalf("active list after delete=%#v", active.Items) }
	archived, err := database.ListProjectGems(ctx, project.ProjectID, model.ProjectGemFilter{IncludeDeleted: true})
	if err != nil { t.Fatal(err) }
	if len(archived.Items) != 2 || archived.Items[0].GemID != gem.GemID || archived.Items[0].DeletedAt == nil || !archived.Items[0].Pinned { t.Fatalf("archived=%#v", archived.Items) }
	revisions, err := database.ListProjectGemRevisions(ctx, project.ProjectID, gem.GemID)
	if err != nil { t.Fatal(err) }
	if len(revisions) != 2 || revisions[0].Revision != 2 || revisions[1].Revision != 1 { t.Fatalf("revisions=%#v", revisions) }
	if _, err := database.RestoreProjectGem(ctx, project.ProjectID, gem.GemID); err != nil { t.Fatal(err) }
	restored, err := database.GetProjectGem(ctx, project.ProjectID, gem.GemID, false)
	if err != nil { t.Fatal(err) }
	if restored.DeletedAt != nil || restored.Revision != 2 || !restored.Pinned { t.Fatalf("restored=%#v", restored) }
}

func TestProjectGemLookupMatchesNameContentAndCategory(t *testing.T) {
	database, err := Open(filepath.Join(t.TempDir(), "timekeeper.db"))
	if err != nil { t.Fatal(err) }
	t.Cleanup(func() { _ = database.Close() })
	ctx := context.Background()
	project, err := database.CreateProject(ctx, model.CreateProjectInput{Name: "Gem Search"})
	if err != nil { t.Fatal(err) }
	if _, err := database.CreateProjectGem(ctx, project.ProjectID, model.CreateProjectGemInput{Category: "Research", Subcategory: "Retrieval", Name: "Retry budget", Content: "Monotonic deadlines reduce storms."}); err != nil { t.Fatal(err) }
	result, err := database.ListProjectGems(ctx, project.ProjectID, model.ProjectGemFilter{Query: "monotonic storms", Category: "Research", Subcategory: "Retrieval", Limit: 10})
	if err != nil { t.Fatal(err) }
	if len(result.Items) != 1 || result.Items[0].Name != "Retry budget" { t.Fatalf("lookup=%#v", result) }
}

func TestProjectGemSchemaReopensIdempotentlyWithStoredRows(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "timekeeper.db")
	database, err := Open(path)
	if err != nil { t.Fatal(err) }
	project, err := database.CreateProject(ctx, model.CreateProjectInput{Name: "Gem Migration"})
	if err != nil { t.Fatal(err) }
	created, err := database.CreateProjectGem(ctx, project.ProjectID, model.CreateProjectGemInput{Category: "Research", Name: "Durable", Content: "Survives reopening."})
	if err != nil { t.Fatal(err) }
	if err := database.Close(); err != nil { t.Fatal(err) }

	reopened, err := Open(path)
	if err != nil { t.Fatalf("reopen migrated database: %v", err) }
	t.Cleanup(func() { _ = reopened.Close() })
	loaded, err := reopened.GetProjectGem(ctx, project.ProjectID, created.GemID, false)
	if err != nil { t.Fatal(err) }
	if loaded.Ref != created.Ref || loaded.Name != created.Name || loaded.Content != created.Content {
		t.Fatalf("reopened gem=%#v, want %#v", loaded, created)
	}
}
