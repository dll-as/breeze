package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMakeMigration(t *testing.T) {
	t.Chdir(t.TempDir())

	// Create migrations directory first
	if err := os.Mkdir("migrations", 0o755); err != nil {
		t.Fatalf("failed to create migrations dir: %v", err)
	}

	err := runMakeMigration([]string{"CreateUsersTable"})
	if err != nil {
		t.Fatalf("runMakeMigration() error = %v", err)
	}

	// Check that up and down files were created
	upFile := filepath.Join("migrations", "0001_create_users_table.up.sql")
	downFile := filepath.Join("migrations", "0001_create_users_table.down.sql")

	if _, err := os.Stat(upFile); os.IsNotExist(err) {
		t.Errorf("up file not created: %s", upFile)
	}
	if _, err := os.Stat(downFile); os.IsNotExist(err) {
		t.Errorf("down file not created: %s", downFile)
	}
}

func TestMakeMigrationCreatesDir(t *testing.T) {
	t.Chdir(t.TempDir())

	// Don't create migrations directory; runMakeMigration should create it
	err := runMakeMigration([]string{"AddEmailColumn"})
	if err != nil {
		t.Fatalf("runMakeMigration() error = %v", err)
	}

	if _, err := os.Stat("migrations"); os.IsNotExist(err) {
		t.Error("migrations directory was not created")
	}
}

func TestMakeMigrationMultiple(t *testing.T) {
	t.Chdir(t.TempDir())

	if err := os.Mkdir("migrations", 0o755); err != nil {
		t.Fatalf("failed to create migrations dir: %v", err)
	}

	// Create first migration
	if err := runMakeMigration([]string{"CreateUsers"}); err != nil {
		t.Fatalf("first migration failed: %v", err)
	}

	// Create second migration
	if err := runMakeMigration([]string{"AddEmail"}); err != nil {
		t.Fatalf("second migration failed: %v", err)
	}

	// Check version numbers
	upFile1 := filepath.Join("migrations", "0001_create_users.up.sql")
	upFile2 := filepath.Join("migrations", "0002_add_email.up.sql")

	if _, err := os.Stat(upFile1); os.IsNotExist(err) {
		t.Errorf("first migration not created")
	}
	if _, err := os.Stat(upFile2); os.IsNotExist(err) {
		t.Errorf("second migration not created")
	}
}

func TestMakeMigrationEmptyName(t *testing.T) {
	err := runMakeMigration([]string{""})
	if err == nil {
		t.Fatal("expected error for empty name")
	}
	if !strings.Contains(err.Error(), "cannot be empty") {
		t.Errorf("error = %v, want error containing 'cannot be empty'", err)
	}
}

func TestMakeMigrationNoArgs(t *testing.T) {
	err := runMakeMigration([]string{})
	if err == nil {
		t.Fatal("expected error for no arguments")
	}
}

func TestToSlug(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"CreateUsersTable", "create_users_table"},
		{"AddEmail", "add_email"},
		{"user", "user"},
		{"User", "user"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := toSlug(tt.input)
			if got != tt.want {
				t.Errorf("toSlug(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestMakeMigrationFileContent(t *testing.T) {
	t.Chdir(t.TempDir())

	if err := os.Mkdir("migrations", 0o755); err != nil {
		t.Fatalf("failed to create migrations dir: %v", err)
	}

	if err := runMakeMigration([]string{"TestMigration"}); err != nil {
		t.Fatalf("runMakeMigration() error = %v", err)
	}

	upFile := filepath.Join("migrations", "0001_test_migration.up.sql")
	content, err := os.ReadFile(upFile)
	if err != nil {
		t.Fatalf("failed to read up file: %v", err)
	}

	// Check that file contains comment header
	contentStr := string(content)
	if !strings.Contains(contentStr, "Migration 0001") {
		t.Errorf("up file missing migration header comment")
	}
	if !strings.Contains(contentStr, "test_migration") {
		t.Errorf("up file missing name in header comment")
	}
}
