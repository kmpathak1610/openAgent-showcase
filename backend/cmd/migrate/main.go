package main

import (
	"database/sql"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func main() {
	var dbURL string
	flag.StringVar(&dbURL, "url", os.Getenv("DATABASE_URL"), "postgres url")
	flag.Parse()
	args := flag.Args()
	if len(args) == 0 {
		args = []string{"up"}
	}
	cmd := args[0]
	if dbURL == "" {
		fmt.Fprintln(os.Stderr, "DATABASE_URL required")
		os.Exit(1)
	}
	db, err := sql.Open("pgx", dbURL)
	if err != nil {
		panic(err)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		panic(err)
	}
	_, _ = db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (filename TEXT PRIMARY KEY, applied_at TIMESTAMPTZ DEFAULT now())`)
	dir := "migrations"
	if _, err := os.Stat("backend/migrations"); err == nil {
		dir = "backend/migrations"
	}
	files, _ := filepath.Glob(filepath.Join(dir, "*.sql"))
	sort.Strings(files)

	switch cmd {
	case "up":
		for _, f := range files {
			name := filepath.Base(f)
			var exists int
			_ = db.QueryRow(`SELECT 1 FROM schema_migrations WHERE filename=$1`, name).Scan(&exists)
			if exists == 1 {
				continue
			}
			b, _ := os.ReadFile(f)
			fmt.Printf("applying %s...\n", name)
			if _, err := db.Exec(string(b)); err != nil {
				fmt.Fprintf(os.Stderr, "failed %s: %v\n", name, err)
				os.Exit(1)
			}
			_, _ = db.Exec(`INSERT INTO schema_migrations(filename) VALUES($1)`, name)
		}
		fmt.Println("migrations up to date")
	case "down":
		// naive: drop all tables (dev only)
		if len(files) == 0 {
			fmt.Println("no migrations")
			return
		}
		// get applied
		rows, _ := db.Query(`SELECT filename FROM schema_migrations ORDER BY filename DESC`)
		defer func() { if rows != nil { rows.Close() } }()
		var toRevert []string
		for rows.Next() {
			var n string
			rows.Scan(&n)
			toRevert = append(toRevert, n)
		}
		if len(toRevert) == 0 {
			fmt.Println("nothing to revert")
			return
		}
		fmt.Println("dropping all tables (dev down)...")
		// This is destructive — only for foundation dev.
		// We drop in reverse dependency order manually via cascade.
		tables := []string{
			"audit_logs", "approvals", "memories", "document_chunks", "documents",
			"agent_actions", "agent_runs", "task_events", "task_assignments", "tasks",
			"message_threads", "messages", "channel_members", "channels",
			"project_agents", "project_members", "projects", "agent_versions", "agents",
			"organization_members", "users", "organizations",
		}
		for _, t := range tables {
			_, _ = db.Exec(fmt.Sprintf(`DROP TABLE IF EXISTS %s CASCADE`, t))
		}
		_, _ = db.Exec(`DELETE FROM schema_migrations`)
		_, _ = db.Exec(`DROP FUNCTION IF EXISTS update_updated_at() CASCADE`)
		// drop vector extension optionally kept
		fmt.Println("down complete")
	default:
		fmt.Fprintf(os.Stderr, "unknown cmd %s (up|down)\n", strings.Join(args, " "))
		os.Exit(1)
	}
}
