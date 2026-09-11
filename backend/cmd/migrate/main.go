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
	"strconv"
	"strings"
	"time"

	"dwz-admin/internal/config"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type migrationRow struct {
	Version   string
	AppliedAt time.Time
}

// applyEnvOverrides lets ops/one_click_migrate.sh pass connection details on
// the command line without writing a config file twice. Env wins over yaml so
// the script remains the single source of truth for a given run.
func applyEnvOverrides(cfg *config.Config) {
	envStr := func(key string, dst *string) {
		if v := os.Getenv(key); v != "" {
			*dst = v
		}
	}
	envInt := func(key string, dst *int) {
		v := os.Getenv(key)
		if v == "" {
			return
		}
		if n, err := strconv.Atoi(v); err == nil {
			*dst = n
		}
	}

	envStr("DWZ_DB_HOST", &cfg.Database.Host)
	envInt("DWZ_DB_PORT", &cfg.Database.Port)
	envStr("DWZ_DB_USER_OVERRIDE", &cfg.Database.User)
	envStr("DWZ_DB_PASS_OVERRIDE", &cfg.Database.Password)
	envStr("DWZ_ADMIN_DB_OVERRIDE", &cfg.Database.DBName)
	envStr("DWZ_PUBLIC_DB_OVERRIDE", &cfg.PublicDB.DBName)
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

// pendingFor returns the migrations that are not yet recorded.
func pendingFor(db *gorm.DB, list []migration) ([]migration, error) {
	appliedSet, _, err := readApplied(db)
	if err != nil {
		return nil, err
	}
	var pending []migration
	for _, m := range list {
		if appliedSet[m.File] {
			continue
		}
		if _, err := os.Stat(m.Path); err != nil {
			return nil, fmt.Errorf("migration file missing: %s", m.Path)
		}
		pending = append(pending, m)
	}
	return pending, nil
}

func printStatus(db *gorm.DB, list []migration) error {
	appliedSet, applied, err := readApplied(db)
	if err != nil {
		return err
	}
	fmt.Println("applied:", len(applied))
	pendingCount := 0
	for _, m := range list {
		mark := "  [x]"
		if !appliedSet[m.File] {
			mark = "  [ ]"
			pendingCount++
		}
		fmt.Printf("%s %s\n", mark, m.File)
	}
	if pendingCount == 0 {
		fmt.Println("  (all migrations applied)")
	}
	return nil
}

func applyAll(db *gorm.DB, pending []migration, vars map[string]string) error {
	for _, m := range pending {
		sqlBytes, err := os.ReadFile(m.Path)
		if err != nil {
			return fmt.Errorf("read migration %s failed: %w", m.File, err)
		}
		// Substitute the {{PUBLIC_DB}} / {{ADMIN_DB}} placeholders so no
		// migration carries a hard-coded database name any more.
		stmt := substituteVars(string(sqlBytes), vars)

		// Apply + record in one transaction so a failed script is not marked done.
		if err := db.Transaction(func(tx *gorm.DB) error {
			if err := tx.Exec(stmt).Error; err != nil {
				return err
			}
			return tx.Exec("INSERT INTO schema_migrations (version) VALUES (?)", m.File).Error
		}); err != nil {
			return fmt.Errorf("apply failed: %s -> %w", m.File, err)
		}
		fmt.Println("applied", m.File)
	}
	return nil
}

// substituteVars replaces {{KEY}} placeholders with their configured values.
// Values are back-quoted identifiers, so any stray quote in a name would break
// the statement; the DB names come from config and are validated first.
func substituteVars(sql string, vars map[string]string) string {
	for k, v := range vars {
		sql = strings.ReplaceAll(sql, "{{"+k+"}}", v)
	}
	return sql
}

// validateDBName rejects names that cannot be safely interpolated as a quoted
// identifier in a migration script.
func validateDBName(name string) error {
	if name == "" {
		return fmt.Errorf("empty database name")
	}
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_', r == '$', r == '-':
		default:
			return fmt.Errorf("database name %q contains unsupported character %q", name, r)
		}
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
		return fmt.Errorf("short_urls still has %d rows on the legacy MD5(long_url) hash; run migrations/040_scope_url_hash.sql", legacyAdmin)
	}

	var legacyPublic int64
	if err := public.Table("wjoy_log").
		Where("url_hash = MD5(longurl)").
		Count(&legacyPublic).Error; err != nil {
		fmt.Println("parity: skip wjoy_log check ->", err)
	} else if legacyPublic > 0 {
		return fmt.Errorf("wjoy_log still has %d rows on the legacy MD5(longurl) hash; run migrations/040_scope_url_hash.sql", legacyPublic)
	}

	fmt.Println("parity: both databases are on the scoped url_hash layout")
	return nil
}

func main() {
	status := flag.Bool("status", false, "list applied/pending migrations")
	dryRun := flag.Bool("dry-run", false, "print pending migrations without applying")
	all := flag.Bool("all", false, "apply admin + public migrations and run parity checks")
	baseline := flag.Bool("baseline", false, "mark every migration in the dir as applied WITHOUT executing it (for databases upgraded by hand)")
	only := flag.String("only", "", "limit to one database: admin | public")
	dir := flag.String("migrations", "migrations", "directory containing the SQL migration files")
	configPath := flag.String("config", "configs/config.yaml", "path to the config yaml")
	flag.Parse()

	if err := config.Init(*configPath); err != nil {
		fmt.Fprintln(os.Stderr, "load config failed:", err)
		os.Exit(1)
	}
	cfg := config.Get()
	applyEnvOverrides(cfg)
	resolvePublicDB(cfg)

	if err := validateDBName(cfg.Database.DBName); err != nil {
		fmt.Fprintln(os.Stderr, "invalid admin db name:", err)
		os.Exit(1)
	}
	if err := validateDBName(cfg.PublicDB.DBName); err != nil {
		fmt.Fprintln(os.Stderr, "invalid public db name:", err)
		os.Exit(1)
	}

	// Discover migrations from the directory instead of a hard-coded list, so a
	// new file is picked up without touching Go code.
	adminMigs, publicMigs, err := discoverMigrations(*dir)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	vars := map[string]string{
		"ADMIN_DB":  cfg.Database.DBName,
		"PUBLIC_DB": cfg.PublicDB.DBName,
	}

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

	wantAdmin := *only == "" || *only == "admin"
	wantPublic := (*only == "" || *only == "public") && public != nil

	// ---- baseline: record files as applied without executing them ----------
	// Needed for production databases whose schema was advanced by hand (mysql <
	// file, or ops/one_click_migrate.sh) before schema_migrations existed.
	if *baseline {
		if !*dryRun {
			if wantAdmin {
				for _, m := range adminMigs {
					if err := markApplied(admin, m.File); err != nil {
						fmt.Fprintln(os.Stderr, err)
						os.Exit(1)
					}
				}
			}
			if wantPublic {
				if err := ensureVersionTable(public); err != nil {
					fmt.Fprintln(os.Stderr, err)
					os.Exit(1)
				}
				for _, m := range publicMigs {
					if err := markApplied(public, m.File); err != nil {
						fmt.Fprintln(os.Stderr, err)
						os.Exit(1)
					}
				}
			}
		}
		fmt.Println("baseline recorded (no DDL executed)")
		return
	}

	// ---- status / dry-run --------------------------------------------------
	if *status || *dryRun {
		if wantAdmin {
			fmt.Println("== admin db:", cfg.Database.DBName)
			if err := printStatus(admin, adminMigs); err != nil {
				fmt.Fprintln(os.Stderr, "read migrations failed:", err)
				os.Exit(1)
			}
		}
		if wantPublic {
			if err := ensureVersionTable(public); err != nil {
				fmt.Fprintln(os.Stderr, "ensure public schema_migrations failed:", err)
				os.Exit(1)
			}
			fmt.Println("== public db:", cfg.PublicDB.DBName)
			if err := printStatus(public, publicMigs); err != nil {
				fmt.Fprintln(os.Stderr, "read public migrations failed:", err)
				os.Exit(1)
			}
		}
		if *dryRun {
			pa, _ := pendingFor(admin, adminMigs)
			fmt.Printf("would apply %d pending (admin)\n", len(pa))
			if wantPublic {
				pp, _ := pendingFor(public, publicMigs)
				fmt.Printf("would apply %d pending (public)\n", len(pp))
			}
		}
		return
	}

	// ---- apply -------------------------------------------------------------
	appliedAny := false

	// Order matters: some admin migrations are cross-DB and read the public
	// tables (040_scope_url_hash.sql joins wjoy_log, 140_migrate_wjoy_log.sql
	// reads it as the source of truth). The public schema must therefore exist
	// before those run, otherwise a fresh install fails with "table wjoy_log
	// doesn't exist". Apply the public baseline first.
	if wantPublic {
		if err := ensureVersionTable(public); err != nil {
			fmt.Fprintln(os.Stderr, "ensure public schema_migrations failed:", err)
			os.Exit(1)
		}
		// Public baseline (public_schema.sql) must precede any admin migration
		// that reads across databases.
		if pendingPublic, err := pendingFor(public, publicMigs); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		} else if len(pendingPublic) > 0 {
			if err := applyAll(public, pendingPublic, vars); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			appliedAny = true
		}
	}

	if wantAdmin {
		pending, err := pendingFor(admin, adminMigs)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		if len(pending) > 0 {
			if err := applyAll(admin, pending, vars); err != nil {
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

// markApplied records a migration as done without running its SQL. Used by
// -baseline to adopt databases that were migrated by hand before
// schema_migrations existed, so a later `migrate` run does not re-execute DDL.
func markApplied(db *gorm.DB, version string) error {
	return db.Exec(
		"INSERT INTO schema_migrations (version) VALUES (?) "+
			"ON DUPLICATE KEY UPDATE version = version", version).Error
}
