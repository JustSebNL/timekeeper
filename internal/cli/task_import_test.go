// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package cli

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"
)

func TestImportTasks_UnknownFormat(t *testing.T) {
	_, err := ImportTasks("dummy.txt", "unknown")
	if err == nil {
		t.Fatal("expected error for unknown format")
	}
}

func TestImportTasks_CSV(t *testing.T) {
	content := "name,category_id,estimate,auto_close\nTask 1,cat-1,2h,true\nTask 2,,1h30m,0\n"
	path := filepath.Join(t.TempDir(), "test.csv")
	_ = os.WriteFile(path, []byte(content), 0644)

	tasks, err := ImportTasks(path, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(tasks) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(tasks))
	}

	if tasks[0].Name != "Task 1" || tasks[0].CategoryID != "cat-1" || tasks[0].EstimatedDurationSeconds != 7200 || tasks[0].AutoClose == nil || !*tasks[0].AutoClose {
		t.Errorf("task 1 mismatch: %+v", tasks[0])
	}

	if tasks[1].Name != "Task 2" || tasks[1].EstimatedDurationSeconds != 5400 || tasks[1].AutoClose == nil || *tasks[1].AutoClose {
		t.Errorf("task 2 mismatch: %+v", tasks[1])
	}
}

func TestImportTasks_TSV(t *testing.T) {
	content := "name\tcategory_id\testimate\tauto_close\nTask 1\tcat-1\t2h\ttrue\nTask 2\t\t1h30m\t0\n"
	path := filepath.Join(t.TempDir(), "test.tsv")
	_ = os.WriteFile(path, []byte(content), 0644)

	tasks, err := ImportTasks(path, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(tasks) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(tasks))
	}
}

func TestImportTasks_JSON(t *testing.T) {
	contentBare := `[{"name": "Task 1", "estimated_duration_seconds": 3600}]`
	pathBare := filepath.Join(t.TempDir(), "bare.json")
	_ = os.WriteFile(pathBare, []byte(contentBare), 0644)

	tasks, err := ImportTasks(pathBare, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tasks) != 1 || tasks[0].EstimatedDurationSeconds != 3600 {
		t.Errorf("bare json mismatch: %+v", tasks)
	}

	contentWrapped := `{"tasks": [{"name": "Task 1", "estimated_duration_seconds": 3600}]}`
	pathWrapped := filepath.Join(t.TempDir(), "wrapped.json")
	_ = os.WriteFile(pathWrapped, []byte(contentWrapped), 0644)

	tasks2, err := ImportTasks(pathWrapped, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tasks2) != 1 || tasks2[0].EstimatedDurationSeconds != 3600 {
		t.Errorf("wrapped json mismatch: %+v", tasks2)
	}
}

func TestImportTasks_YAML(t *testing.T) {
	contentBare := "- name: Task 1\n  estimated_duration_seconds: 3600\n"
	pathBare := filepath.Join(t.TempDir(), "bare.yaml")
	_ = os.WriteFile(pathBare, []byte(contentBare), 0644)

	tasks, err := ImportTasks(pathBare, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tasks) != 1 || tasks[0].EstimatedDurationSeconds != 3600 {
		t.Errorf("bare yaml mismatch: %+v", tasks)
	}

	contentWrapped := "tasks:\n  - name: Task 1\n    estimated_duration_seconds: 3600\n"
	pathWrapped := filepath.Join(t.TempDir(), "wrapped.yaml")
	_ = os.WriteFile(pathWrapped, []byte(contentWrapped), 0644)

	tasks2, err := ImportTasks(pathWrapped, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tasks2) != 1 || tasks2[0].EstimatedDurationSeconds != 3600 {
		t.Errorf("wrapped yaml mismatch: %+v", tasks2)
	}
}

func TestImportTasks_Excel(t *testing.T) {
	f := excelize.NewFile()
	sheet := "Sheet1"
	f.SetCellValue(sheet, "A1", "name")
	f.SetCellValue(sheet, "B1", "estimate")
	f.SetCellValue(sheet, "A2", "Task 1")
	f.SetCellValue(sheet, "B2", "30m")

	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		t.Fatalf("write excel: %v", err)
	}

	path := filepath.Join(t.TempDir(), "test.xlsx")
	_ = os.WriteFile(path, buf.Bytes(), 0644)

	tasks, err := ImportTasks(path, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}
	if tasks[0].Name != "Task 1" || tasks[0].EstimatedDurationSeconds != 1800 {
		t.Errorf("excel task mismatch: %+v", tasks[0])
	}
}

func TestImportTasks_SQL(t *testing.T) {
	content := `
-- Some comment
/* Block comment */
INSERT INTO tasks (category_id, name, description, goal, priority, estimated_duration_seconds, auto_close) VALUES
('C-123', 'Task name', 'description', 'goal', 'Normal', 1800, 1),
('C-123', 'Another task', '', '', 'High', 3600, 1);
`
	path := filepath.Join(t.TempDir(), "test.sql")
	_ = os.WriteFile(path, []byte(content), 0644)

	tasks, err := ImportTasks(path, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(tasks) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(tasks))
	}

	if tasks[0].Name != "Task name" || tasks[0].EstimatedDurationSeconds != 1800 || tasks[0].CategoryID != "C-123" {
		t.Errorf("sql task 1 mismatch: %+v", tasks[0])
	}
	if tasks[1].Name != "Another task" || tasks[1].EstimatedDurationSeconds != 3600 {
		t.Errorf("sql task 2 mismatch: %+v", tasks[1])
	}
}

func TestImportTasks_ZIP(t *testing.T) {
	content1 := "name,category_id,estimate,auto_close\nTask 1,cat-1,2h,true\n"
	content2 := "name\tcategory_id\testimate\tauto_close\nTask 2\t\t1h30m\t0\n"
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	f1, _ := zw.Create("f1.csv")
	f1.Write([]byte(content1))
	f2, _ := zw.Create("f2.tsv")
	f2.Write([]byte(content2))
	zw.Close()
	path := filepath.Join(t.TempDir(), "test.zip")
	_ = os.WriteFile(path, buf.Bytes(), 0644)
	tasks, err := ImportTasks(path, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tasks) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(tasks))
	}
}

func TestImportTasks_TarGz(t *testing.T) {
	content1 := "name,category_id,estimate,auto_close\nTask 1,cat-1,2h,true\n"
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)
	hdr := &tar.Header{Name: "f1.csv", Mode: 0600, Size: int64(len(content1)), ModTime: time.Now()}
	tw.WriteHeader(hdr)
	tw.Write([]byte(content1))
	tw.Close()
	gw.Close()
	path := filepath.Join(t.TempDir(), "test.tar.gz")
	_ = os.WriteFile(path, buf.Bytes(), 0644)
	tasks, err := ImportTasks(path, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}
	if tasks[0].Name != "Task 1" {
		t.Errorf("mismatch: %+v", tasks[0])
	}
}
