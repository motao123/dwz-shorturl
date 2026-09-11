// Command migrate applies pending SQL migrations to the admin database and
// records them in a schema_migrations version table. All bundled migrations are
// idempotent, so re-applying an already-applied migration is a no-op.
//
// Usage (run from the backend build dir):
//
//	./migrate -status                  # list applied / pending migrations
//	./migrate -dry-run                 # print what would be applied
//	./migrate -all                     # admin migrations + public-DB parity checks
//	./migrate -migrations ../migrations  # apply pending (default dir)
//
// The admin DB credentials come from configs/config.yaml (same as the server).
// The public frontend DB (public_db.*) is optional; when it is unset the tool
// derives it from the admin credentials so a single-admin configuration still
// works for both databases.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

// publicOrder lists migrations that must run against the *public* frontend DB
// (the one holding wjoy_log). Keeping them separate lets -all cover both
// databases in a single invocation.
var publicOrder = []string{
	"public_schema.sql",
}

type migrationRow struct {
	Version   string
	AppliedAt time.Time
}

// resolvePublicDB fills in public_db from the admin credentials when the
// operator only configured one database. The public DB is where wjoy_log
// lives; in single-DB deployments it is the same schema as the admin DB.
func resolvePublicDB(cfg *config.Config) {
	if cfg.PublicDB.DBName == "" {
		cfg.PublicDB.DBName = cfg.Database.DBName
	}
	if cfg.PublicDB.Host == "" {
		cfg.PublicDB.Host = cfg.Database.Host
	}
	if cfg.PublicDB.Port == 0 {
		cfg.PublicDB.Port = cfg.Database.Port
	}
	if cfg.PublicDB.User == "" {
		cfg.PublicDB.User = cfg.Database.User
	}
	if cfg.PublicDB.Password == "" {
		cfg.PublicDB.Password = cfg.Database.Password
	}
	if cfg.PublicDB.Charset == "" {
		cfg.PublicDB.Charset = cfg.Database.Charset
	}
}

func openDB(cfg *config.Config, which string) (*gorm.DB, error) {
	var dbCfg config.DatabaseConfig
	switch which {
	case "admin":
		dbCfg = cfg.Database
	case "public":
		dbCfg = cfg.PublicDB
	default:
		return nil, fmt.Errorf("unknown database %q", which)
	}

	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=%s&parseTime=True&loc=Local&multiStatements=true",
		dbCfg.User, dbCfg.Password, dbCfg.Host, dbCfg.Port, dbCfg.DBName, dbCfg.Charset)

	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		return nil, fmt.Errorf("connect %s db (%s) failed: %w", which, dbCfg.DBName, err)
	}
	return db, nil
}

func ensureVersionTable(db *gorm.DB) error {
	return db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version    VARCHAR(128) NOT NULL,
		applied_at DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
		PRIMARY KEY (version)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`).Error
}

func readApplied(db *gorm.DB) (map[string]bool, []string, error) {
	var applied []string
	if err := db.Table("schema_migrations").Pluck("version", &applied).Error; err != nil {
		return nil, nil, err
	}
	set := map[string]bool{}
	for _, v := range applied {
		set[v] = true
	}
	return set, applied, nil
}

// pendingFor returns the migration files that are not yet recorded.
func pendingFor(db *gorm.DB, dir string, list []string) ([]string, error) {
	appliedSet, _, err := readApplied(db)
	if err != nil {
		return nil, err
	}
	var pending []string
	for _, name := range list {
		if appliedSet[name] {
			continue
		}
		path := filepath.Join(dir, name)
		if _, err := os.Stat(path); err != nil {
			return nil, fmt.Errorf("migration file missing: %s", path)
		}
		pending = append(pending, name)
	}
	return pending, nil
}

func printStatus(db *gorm.DB, list []string) error {
	appliedSet, applied, err := readApplied(db)
	if err != nil {
		return err
	}
	fmt.Println("applied:", len(applied))
	for _, v := range list {
		mark := "  [x]"
		if !appliedSet[v] {
			mark = "  [ ]"
		}
		fmt.Printf("%s %s\n", mark, v)
	}
	return nil
}

func applyAll(db *gorm.DB, dir string, pending []string) error {
	for _, name := range pending {
		path := filepath.Join(dir, name)
		sqlBytes, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read migration %s failed: %w", name, err)
		}
		// Apply + record in one transaction so a failed script is not marked done.
		if err := db.Transaction(func(tx *gorm.DB) error {
			if err := tx.Exec(string(sqlBytes)).Error; err != nil {
				return err
			}
			return tx.Exec("INSERT INTO schema_migrations (version) VALUES (?)", name).Error
		}); err != nil {
			return fmt.Errorf("apply failed: %s -> %w", name, err)
		}
		fmt.Println("applied", name)
	}
	return nil
}

// parityCheck verifies that the two databases agree on the scoped url_hash
// layout, so the PHP and Go jump paths resolve the same row. It reports
// mismatches instead of changing data.
func parityCheck(admin, public *gorm.DB) error {
	var legacyAdmin int64
	if err := admin.Table("short_urls").
		Where("url_hash = MD5(long_url)").
		Count(&legacyAdmin).Error; err != nil {
		// Column may not exist on very old schemas; treat as non-fatal.
		fmt.Println("parity: skip short_urls check ->", err)
	} else if legacyAdmin > 0 {
		return fmt.Errorf("short_urls still has %d rows on the legacy MD5(long_url) hash; run migrations/scope_url_hash.sql", legacyAdmin)
	}

	var legacyPublic int64
	if err := public.Table("wjoy_log").
		Where("url_hash = MD5(longurl)").
		Count(&legacyPublic).Error; err != nil {
		fmt.Println("parity: skip wjoy_log check ->", err)
	} else if legacyPublic > 0 {
		return fmt.Errorf("wjoy_log still has %d rows on the legacy MD5(longurl) hash; run migrations/scope_url_hash.sql", legacyPublic)
	}

	fmt.Println("parity: both databases are on the scoped url_hash layout")
	return nil
}

func main() {
	status := flag.Bool("status", false, "list applied/pending migrations")
	dryRun := flag.Bool("dry-run", false, "print pending migrations without applying")
	all := flag.Bool("all", false, "apply admin + public migrations and run parity checks")
	dir := flag.String("migrations", "migrations", "directory containing the SQL migration files")
	configPath := flag.String("config", "configs/config.yaml", "path to the config yaml")
	flag.Parse()

	if err := config.Init(*configPath); err != nil {
		fmt.Fprintln(os.Stderr, "load config failed:", err)
		os.Exit(1)
	}
	cfg := config.Get()
	resolvePublicDB(cfg)

	admin, err := openDB(cfg, "admin")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := ensureVersionTable(admin); err != nil {
		fmt.Fprintln(os.Stderr, "ensure schema_migrations failed:", err)
		os.Exit(1)
	}

	public, pubErr := openDB(cfg, "public")
	if pubErr != nil {
		fmt.Fprintln(os.Stderr, "public db unavailable:", pubErr)
	}

	if *status || *dryRun {
		fmt.Println("== admin db:", cfg.Database.DBName)
		if err := printStatus(admin, order); err != nil {
			fmt.Fprintln(os.Stderr, "read migrations failed:", err)
			os.Exit(1)
		}
		if public != nil {
			if err := ensureVersionTable(public); err != nil {
				fmt.Fprintln(os.Stderr, "ensure public schema_migrations failed:", err)
				os.Exit(1)
			}
			fmt.Println("== public db:", cfg.PublicDB.DBName)
			if err := printStatus(public, publicOrder); err != nil {
				fmt.Fprintln(os.Stderr, "read public migrations failed:", err)
				os.Exit(1)
			}
		}
		if *dryRun {
			pending, err := pendingFor(admin, *dir, order)
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			fmt.Printf("would apply %d pending (admin)\n", len(pending))
		}
		return
	}

	appliedAny := false

	pending, err := pendingFor(admin, *dir, order)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if len(pending) > 0 {
		if err := applyAll(admin, *dir, pending); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		appliedAny = true
	}

	if public != nil {
		if err := ensureVersionTable(public); err != nil {
			fmt.Fprintln(os.Stderr, "ensure public schema_migrations failed:", err)
			os.Exit(1)
		}
		pendingPublic, err := pendingFor(public, *dir, publicOrder)
		if err != nil {
			// public_schema.sql is optional decoration; keep going.
			if !strings.Contains(err.Error(), "missing") {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
		} else if len(pendingPublic) > 0 {
			if err := applyAll(public, *dir, pendingPublic); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			appliedAny = true
		}
	}

	if *all {
		if public == nil {
			fmt.Fprintln(os.Stderr, "parity check needs a reachable public db")
			os.Exit(1)
		}
		if err := parityCheck(admin, public); err != nil {
			fmt.Fprintln(os.Stderr, "parity:", err)
			os.Exit(1)
		}
	}

	if !appliedAny {
		fmt.Println("no pending migrations")
	}
	fmt.Println("done")
}
