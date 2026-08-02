package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/nelthaarion/breeze/migrate"
)

func runMigrate(args []string) error {
	if len(args) == 0 {
		return runMigrateUp(args)
	}

	// Check for subcommands
	subcommand := args[0]
	switch {
	case subcommand == "up":
		return runMigrateUp(args[1:])
	case subcommand == "down":
		return runMigrateDown(args[1:])
	case subcommand == "status":
		return runMigrateStatus(args[1:])
	default:
		return fmt.Errorf("unknown migrate subcommand %q — must be up, down, or status", subcommand)
	}
}

func runMakeMigration(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: breeze makemigration <name>")
	}

	name := args[0]
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("migration name cannot be empty")
	}

	// Ensure migrations directory exists
	migrationsDir := "migrations"
	if _, err := os.Stat(migrationsDir); err != nil {
		if os.IsNotExist(err) {
			if err := os.MkdirAll(migrationsDir, 0o755); err != nil {
				return fmt.Errorf("failed to create migrations directory: %w", err)
			}
		} else {
			return err
		}
	}

	// List existing migrations to determine next version
	entries, err := os.ReadDir(migrationsDir)
	if err != nil {
		return fmt.Errorf("failed to read migrations directory: %w", err)
	}

	// Parse existing migrations to find the next version
	var highestVersion int
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		filename := entry.Name()
		if strings.HasSuffix(filename, ".up.sql") {
			// Extract version number from filename like "0001_name.up.sql"
			if underscore := strings.Index(filename, "_"); underscore > 0 {
				versionStr := filename[:underscore]
				if v, err := strconv.Atoi(versionStr); err == nil {
					if v > highestVersion {
						highestVersion = v
					}
				}
			}
		}
	}

	nextVersion := highestVersion + 1
	versionStr := fmt.Sprintf("%04d", nextVersion)

	// Convert name to slug (CamelCase -> snake_case)
	slug := toSlug(name)

	upFile := filepath.Join(migrationsDir, fmt.Sprintf("%s_%s.up.sql", versionStr, slug))
	downFile := filepath.Join(migrationsDir, fmt.Sprintf("%s_%s.down.sql", versionStr, slug))

	// Create up migration file
	upContent := fmt.Sprintf("-- Migration %s: %s\n-- Created at %s\n\n", versionStr, slug, time.Now().Format(time.RFC3339))
	if err := os.WriteFile(upFile, []byte(upContent), 0o644); err != nil {
		return fmt.Errorf("failed to create migration file: %w", err)
	}

	// Create down migration file
	downContent := fmt.Sprintf("-- Rollback for migration %s: %s\n-- Created at %s\n\n", versionStr, slug, time.Now().Format(time.RFC3339))
	if err := os.WriteFile(downFile, []byte(downContent), 0o644); err != nil {
		// Clean up up file if down fails
		os.Remove(upFile)
		return fmt.Errorf("failed to create rollback file: %w", err)
	}

	fmt.Printf("Created migration %s:\n  %s\n  %s\n", versionStr, upFile, downFile)
	return nil
}

func runMigrateUp(args []string) error {
	db, err := openDatabase()
	if err != nil {
		return err
	}
	defer db.Close()

	migrationsDir := "migrations"
	fsys := os.DirFS(migrationsDir)
	runner := migrate.New(db, fsys)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := runner.Up(ctx); err != nil {
		return err
	}

	fmt.Println("All pending migrations applied successfully")
	return nil
}

func runMigrateDown(args []string) error {
	n := 1
	if len(args) > 0 {
		var err error
		n, err = strconv.Atoi(args[0])
		if err != nil || n <= 0 {
			return fmt.Errorf("invalid argument: n must be a positive integer")
		}
	}

	db, err := openDatabase()
	if err != nil {
		return err
	}
	defer db.Close()

	migrationsDir := "migrations"
	fsys := os.DirFS(migrationsDir)
	runner := migrate.New(db, fsys)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := runner.Down(ctx, n); err != nil {
		return err
	}

	fmt.Printf("Rolled back %d migration(s)\n", n)
	return nil
}

func runMigrateStatus(args []string) error {
	db, err := openDatabase()
	if err != nil {
		return err
	}
	defer db.Close()

	migrationsDir := "migrations"
	fsys := os.DirFS(migrationsDir)
	runner := migrate.New(db, fsys)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	entries, err := runner.Status(ctx)
	if err != nil {
		return err
	}

	if len(entries) == 0 {
		fmt.Println("No migrations found")
		return nil
	}

	// Print table header
	fmt.Printf("%-6s %-30s %-8s %-25s %s\n", "Version", "Name", "Applied", "Applied At", "Status")
	fmt.Println(strings.Repeat("-", 80))

	// Print each entry
	for _, e := range entries {
		applied := "no"
		appliedAt := ""
		status := ""

		if e.Applied {
			applied = "yes"
			if e.AppliedAt != nil {
				appliedAt = e.AppliedAt.Format("2006-01-02 15:04:05")
			}
			if e.ChecksumMismatch {
				status = "CHECKSUM MISMATCH"
			}
		}

		fmt.Printf("%-6d %-30s %-8s %-25s %s\n", e.Version, e.Name, applied, appliedAt, status)
	}

	return nil
}

// openDatabase opens a database connection using environment variables.
// BREEZE_DATABASE_DRIVER specifies the SQL driver (default: "postgres")
// BREEZE_DATABASE_URL specifies the connection string
func openDatabase() (*sql.DB, error) {
	driver := os.Getenv("BREEZE_DATABASE_DRIVER")
	if driver == "" {
		driver = "postgres"
	}

	dsn := os.Getenv("BREEZE_DATABASE_URL")
	if dsn == "" {
		return nil, fmt.Errorf(`missing BREEZE_DATABASE_URL environment variable

The breeze CLI cannot provide a database connection directly because the framework
is driver-agnostic. To use the migrate command from this CLI, set:

  export BREEZE_DATABASE_DRIVER=postgres  # or your driver: mysql, sqlite3, etc.
  export BREEZE_DATABASE_URL="..."        # your connection string

Alternatively, use the migrate package as a library in your own binary:

  import (
    _ "github.com/lib/pq"  // or your driver
    "github.com/nelthaarion/breeze/migrate"
  )

  func main() {
    db, err := sql.Open("postgres", os.Getenv("DATABASE_URL"))
    // ... error handling ...
    runner := migrate.New(db, os.DirFS("migrations"))
    ctx := context.Background()
    if err := runner.Up(ctx); err != nil {
      log.Fatal(err)
    }
  }
`)
	}

	db, err := sql.Open(driver, dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w (make sure the %q driver is registered, usually via blank import)", err, driver)
	}

	// Test the connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	return db, nil
}

// toSlug converts CamelCase to snake_case
func toSlug(name string) string {
	var buf strings.Builder
	for i, r := range name {
		if i > 0 && r >= 'A' && r <= 'Z' {
			buf.WriteRune('_')
		}
		lower := rune(strings.ToLower(string(r))[0])
		buf.WriteRune(lower)
	}
	return buf.String()
}
