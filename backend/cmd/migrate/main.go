// Command migrate applies pending SQL migrations to the admin database and
// records them in a schema_migrations version table. All bundled migrations are
// idempotent, so re-applying an already-applied migration is a no-op.
//
// Usage (run from the backend build dir):
//
//	./migrate -status                  # list applied / pending migrations
//	./migrate -dry-run                 # print what would be applied
//	./migrate -migrations ../migrations  # apply pending (default dir)
//
// The admin DB credentials come from configs/config.yaml (same as the server).
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"dwz-admin/internal/config"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// order lists the admin-DB migrations in apply order. schema.sql is the
// baseline; the add_* files are idempotent column/index backfills.
var order = []string{
	"schema.sql",
	"add_domains.sql",
	"add_webhooks.sql",
	"add_missing_columns.sql",
	"add_password_hash.sql",
	"add_totp.sql",
	"optimize_domain_indexes.sql",
}

type migrationRow struct {
	Version   string
	AppliedAt time.Time
}

func main() {
	status := flag.Bool("status", false, "list applied/pending migrations")
	dryRun := flag.Bool("dry-run", false, "print pending migrations without applying")
	dir := flag.String("migrations", "migrations", "directory containing the SQL migration files")
	configPath := flag.String("config", "configs/config.yaml", "path to the config yaml")
	flag.Parse()

	if err := config.Init(*configPath); err != nil {
		fmt.Fprintln(os.Stderr, "load config failed:", err)
		os.Exit(1)
	}
	cfg := config.Get()

	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=%s&parseTime=True&loc=Local&multiStatements=true",
		cfg.Database.User, cfg.Database.Password, cfg.Database.Host, cfg.Database.Port,
		cfg.Database.DBName, cfg.Database.Charset)
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		fmt.Fprintln(os.Stderr, "connect failed:", err)
		os.Exit(1)
	}

	// Ensure the version table exists.
	if err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version    VARCHAR(128) NOT NULL,
		applied_at DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
		PRIMARY KEY (version)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`).Error; err != nil {
		fmt.Fprintln(os.Stderr, "ensure schema_migrations failed:", err)
		os.Exit(1)
	}

	var applied []string
	if err := db.Table("schema_migrations").Pluck("version", &applied).Error; err != nil {
		fmt.Fprintln(os.Stderr, "read migrations failed:", err)
		os.Exit(1)
	}
	appliedSet := map[string]bool{}
	for _, v := range applied {
		appliedSet[v] = true
	}

	// Verify every file in order exists on disk.
	var pending []string
	for _, name := range order {
		if appliedSet[name] {
			continue
		}
		path := filepath.Join(*dir, name)
		if _, err := os.Stat(path); err != nil {
			fmt.Fprintln(os.Stderr, "migration file missing:", path)
			os.Exit(1)
		}
		pending = append(pending, name)
	}

	if *status || *dryRun {
		fmt.Println("applied:", len(applied))
		for _, v := range order {
			mark := "  [x]"
			if !appliedSet[v] {
				mark = "  [ ]"
			}
			fmt.Printf("%s %s\n", mark, v)
		}
		if *dryRun {
			fmt.Printf("would apply %d pending\n", len(pending))
		}
		return
	}

	if len(pending) == 0 {
		fmt.Println("no pending migrations")
		return
	}

	for _, name := range pending {
		path := filepath.Join(*dir, name)
		sqlBytes, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintln(os.Stderr, "read migration failed:", name, err)
			os.Exit(1)
		}
		// Apply + record in one transaction so a failed script is not marked done.
		if err := db.Transaction(func(tx *gorm.DB) error {
			if err := tx.Exec(string(sqlBytes)).Error; err != nil {
				return err
			}
			return tx.Exec("INSERT INTO schema_migrations (version) VALUES (?)", name).Error
		}); err != nil {
			fmt.Fprintln(os.Stderr, "apply failed:", name, "->", err)
			os.Exit(1)
		}
		fmt.Println("applied", name)
	}
	fmt.Println("done")
}
