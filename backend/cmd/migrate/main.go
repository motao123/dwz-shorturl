// Command migrate applies pending SQL migrations and records them in a
// schema_migrations version table. All bundled migrations are idempotent, so
// re-applying an already-applied migration is a no-op.
//
// The migration set is discovered from disk (see internal/migration), so adding
// a migration file never requires touching Go code. Its apply position is
// declared in the file itself with a `-- migrate: after <version>` directive
// (also accepted inside a PHP docblock); files with no such directive are still
// discovered, applied last, and flagged so the omission is visible.
//
// Usage (run from the backend dir):
//
//	./migrate -status                     # list every migration file + state
//	./migrate -dry-run                    # print what would be applied
//	./migrate -all                        # apply both DBs + url_hash parity check
//	./migrate -same-db                    # single-database deployment: one schema, one version table
//	./migrate -baseline php/add_members.sql   # mark an out-of-band migration done
//	./migrate -migrations ../backend/migrations
//
// PHP migrations (php/*.php) are executed through the php CLI and recorded in
// the same version table; DWZ_PHP_BIN selects the interpreter.
//
// Cross-database statements reference the management schema through the
// {{ADMIN_DB}} / {{PUBLIC_DB}} placeholders, substituted from the active config
// before a file runs, so no migration hard-codes a database name.
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
	"os/exec"
	"strconv"
	"strings"

	"dwz-admin/internal/config"
	"dwz-admin/internal/migration"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// scanDirs are the directories scanned for migrations, relative to the
// -migrations root. Keeping the PHP-side files in a sub-directory namespaces
// their version keys ("php/...") so they can never collide with an admin file
// of the same name.
var scanDirs = [][2]string{
	{"", ""},
	{"php", "php/"},
}

// base must always be applied first, in this order: schema.sql is the admin
// baseline, public_schema.sql creates the public-side wjoy_log table, and the
// add_* files are idempotent column/index backfills. Everything else declares
// its own position with a `-- migrate: after <version>` directive.
var base = []string{
	"schema.sql",
	// add_missing_columns backfills the columns (category_id, member_id, ...)
	// that the other backfills anchor on or index, so it must come first.
	"add_missing_columns.sql",
	"add_password_hash.sql",
	"add_domains.sql",
	"add_webhooks.sql",
	"add_totp.sql",
	"optimize_domain_indexes.sql",
	"public_schema.sql",
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

// sameDatabase reports whether the admin and public configurations resolve to
// the same MySQL schema. The host is deliberately not compared: a single
// database reached through two different hostnames/Docker service names is
// still one schema, and applying every migration to it is what the operator
// wants.
func sameDatabase(cfg *config.Config) bool {
	return cfg.PublicDB.DBName != "" &&
		cfg.PublicDB.DBName == cfg.Database.DBName
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

// recorded reports whether a version has been applied, tolerating the version
// key that older tooling wrote for the same file (a bare file name). The
// backend files used to carry a numeric prefix, so a database migrated before
// this tool was unified still records "040_scope_url_hash.sql" while the file
// is now "php/scope_url_hash.sql".
func recorded(applied map[string]bool, f migration.File) bool {
	if applied[f.Key] {
		return true
	}
	for _, alias := range migration.LegacyKeys(f.Key) {
		if applied[alias] {
			return true
		}
	}
	return false
}

// split returns the subsets of migrations belonging to each database, keeping
// the global apply order inside each list.
func split(files []migration.File) (adminFiles, publicFiles []migration.File) {
	for _, f := range files {
		if f.Target == migration.TargetPublic {
			publicFiles = append(publicFiles, f)
		} else {
			adminFiles = append(adminFiles, f)
		}
	}
	return adminFiles, publicFiles
}

// printGroup lists migrations recorded against one database, plus which of them
// are still pending.
func printGroup(db *gorm.DB, title string, files []migration.File) (int, error) {
	appliedSet, _, err := readApplied(db)
	if err != nil {
		return 0, err
	}
	pending := 0
	fmt.Printf("== %s\n", title)
	for _, f := range files {
		mark := "[x]"
		if !recorded(appliedSet, f) {
			mark = "[ ]"
			pending++
		}
		suffix := ""
		if f.Unclassified {
			suffix = "   (unclassified: not in any declared order, applied last — pin it with '-- migrate: after <version>')"
		}
		fmt.Printf("  %s %-40s%s\n", mark, f.Key, suffix)
	}
	return pending, nil
}

// substituteVars replaces {{KEY}} placeholders with their configured values, so
// no migration carries a hard-coded database name. Values are injected as bare
// identifiers and the names come from config, which is validated first.
func substituteVars(sql string, vars map[string]string) string {
	for k, v := range vars {
		sql = strings.ReplaceAll(sql, "{{"+k+"}}", v)
	}
	return sql
}

// validateDBName rejects names that cannot be safely interpolated into a
// migration script.
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

// pendingFor returns the migrations that are not yet recorded.
func pendingFor(db *gorm.DB, list []migration.File) ([]migration.File, error) {
	appliedSet, _, err := readApplied(db)
	if err != nil {
		return nil, err
	}
	var pending []migration.File
	for _, m := range list {
		if recorded(appliedSet, m) {
			continue
		}
		if _, err := os.Stat(m.Path); err != nil {
			return nil, fmt.Errorf("migration file missing: %s", m.Path)
		}
		pending = append(pending, m)
	}
	return pending, nil
}

// applyOne executes a single migration and records its version. PHP scripts are
// dispatched to the php CLI.
func applyOne(db *gorm.DB, m migration.File, cfg *config.Config, vars map[string]string, dryRun bool) error {
	if !m.IsSQL() {
		return applyPHP(db, m, cfg, dryRun)
	}

	sqlBytes, err := os.ReadFile(m.Path)
	if err != nil {
		return fmt.Errorf("read migration %s failed: %w", m.Key, err)
	}

	if dryRun {
		fmt.Printf("  would apply %s  (%d bytes)\n", m.Key, len(sqlBytes))
		return nil
	}

	// Substitute the {{PUBLIC_DB}} / {{ADMIN_DB}} placeholders so no migration
	// carries a hard-coded database name any more.
	stmt := substituteVars(string(sqlBytes), vars)

	// Apply + record in one transaction so a failed script is not marked done.
	// DDL is not transactional in MySQL, so this does not roll the schema back;
	// it only guarantees the version row is written after the script succeeds.
	if err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(stmt).Error; err != nil {
			return err
		}
		return tx.Exec("INSERT INTO schema_migrations (version) VALUES (?)", m.Key).Error
	}); err != nil {
		return fmt.Errorf("apply failed: %s -> %w", m.Key, err)
	}
	fmt.Println("applied", m.Key)
	return nil
}

// phpBin returns the PHP interpreter to run PHP-side migrations with.
func phpBin() string {
	if v := os.Getenv("DWZ_PHP_BIN"); v != "" {
		return v
	}
	return "php"
}

// preflightPHP fails fast when the configured PHP interpreter cannot run the
// bundled PHP migrations (missing binary or missing extension). It runs before
// ANY migration is applied so a missing ext/mysqli cannot leave the database
// half-migrated with the version table giving no hint about where it stopped.
// No-op when the migration set has no PHP files.
func preflightPHP(files []migration.File) error {
	for _, f := range files {
		if f.IsSQL() {
			continue
		}
		if err := migration.CheckPHPReady(phpBin()); err != nil {
			return fmt.Errorf("%s", migration.EnsurePHPMigrationRecordsFix(err.Error()))
		}
		fmt.Printf("php preflight: %s ready (extensions %s)\n",
			phpBin(), strings.Join(migration.RequiredPHPModules, ", "))
		return nil
	}
	return nil
}

// applyPHP runs a PHP migration script through the php CLI. These exist because
// a few migrations have to read data before deciding which DDL is safe
// (legacy_schema.php only adds the unique indexes when there are no duplicate
// uids/hashes); running them from Go would mean reimplementing that logic.
//
// The script is given the resolved connection details explicitly instead of
// relying on it finding config.php, so a deployment that only configures
// config.yaml still migrates.
func applyPHP(db *gorm.DB, m migration.File, cfg *config.Config, dryRun bool) error {
	if dryRun {
		fmt.Printf("  would run %s  (php script)\n", m.Key)
		return nil
	}

	// PHP-side migrations operate on the public database; pass the admin name too
	// so nothing has to hard-code it.
	publicCfg := cfg.PublicDB
	if publicCfg.DBName == "" {
		publicCfg = cfg.Database
	}

	args := []string{
		m.Path,
		"--host=" + publicCfg.Host,
		fmt.Sprintf("--port=%d", publicCfg.Port),
		"--user=" + publicCfg.User,
		"--pwd=" + publicCfg.Password,
		"--db=" + publicCfg.DBName,
		"--admin-db=" + cfg.Database.DBName,
	}
	if err := runPHP(phpBin(), args); err != nil {
		return fmt.Errorf("php script failed: %w", err)
	}
	return db.Exec("INSERT INTO schema_migrations (version) VALUES (?)", m.Key).Error
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
		return fmt.Errorf("short_urls still has %d rows on the legacy MD5(long_url) hash; run php/scope_url_hash.sql", legacyAdmin)
	}

	var legacyPublic int64
	if err := public.Table("wjoy_log").
		Where("url_hash = MD5(longurl)").
		Count(&legacyPublic).Error; err != nil {
		fmt.Println("parity: skip wjoy_log check ->", err)
	} else if legacyPublic > 0 {
		return fmt.Errorf("wjoy_log still has %d rows on the legacy MD5(longurl) hash; run php/scope_url_hash.sql", legacyPublic)
	}

	fmt.Println("parity: both databases are on the scoped url_hash layout")
	return nil
}

func main() {
	status := flag.Bool("status", false, "list every migration file with applied/pending state")
	dryRun := flag.Bool("dry-run", false, "print pending migrations without applying")
	all := flag.Bool("all", false, "apply admin + public migrations and run parity checks")
	baseline := flag.Bool("baseline", false, "mark every migration in the dir as applied WITHOUT executing it (for databases upgraded by hand)")
	only := flag.String("only", "", "limit to one database: admin | public")
	sameDB := flag.Bool("same-db", false, "treat admin and public as one schema: apply every migration to it and record every version there")
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

	// Single-database deployments point public_db at the admin schema. Every
	// migration then targets that one schema and is recorded in one
	// schema_migrations table, so -status cannot report a version as applied on
	// one side while pending on the other. Detected as well as opted into:
	// a config that names the same database is unambiguous.
	singleDB := *sameDB || sameDatabase(cfg)

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
	migs, err := discovery(*dir)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	adminMigs, publicMigs := split(migs)

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

	var public *gorm.DB
	if singleDB {
		// One schema: reuse the admin connection so both migration groups land
		// in the same database and share one version table.
		fmt.Printf("single-database mode: admin and public both resolve to %q\n", cfg.Database.DBName)
		public = admin
	} else {
		var pubErr error
		public, pubErr = openDB(cfg, "public")
		if pubErr != nil {
			fmt.Fprintln(os.Stderr, "public db unavailable:", pubErr)
		}
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
					if err := markApplied(admin, m.Key); err != nil {
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
					if err := markApplied(public, m.Key); err != nil {
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
			if _, err := printGroup(admin, "admin migrations", adminMigs); err != nil {
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
			if _, err := printGroup(public, "public migrations", publicMigs); err != nil {
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
	//
	// The two groups are NOT independent, so apply them in one global order
	// instead of running one group to completion and then the other. The
	// cross-database migrations read short_urls (created by the admin baseline
	// schema.sql) while writing wjoy_log (created by public_schema.sql), so a
	// fresh install that applies either group alone fails with "table ...
	// doesn't exist":
	//
	//	schema.sql                -> creates short_urls      (admin)
	//	public_schema.sql         -> creates wjoy_log        (public)
	//	php/*.sql                 -> rewrite both sides      (public)
	//
	// Walking the single discovered order and dispatching each file to its own
	// database keeps the two baselines ahead of the cross-DB files, which is
	// exactly the order the files declare.
	adminApplied, _, err := readApplied(admin)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	publicApplied := map[string]bool{}
	if wantPublic {
		if err := ensureVersionTable(public); err != nil {
			fmt.Fprintln(os.Stderr, "ensure public schema_migrations failed:", err)
			os.Exit(1)
		}
		if publicApplied, _, err = readApplied(public); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}

	// Pre-flight the PHP interpreter before touching the schema: the PHP
	// migrations need ext/mysqli, and discovering that mid-run leaves a
	// half-migrated database behind.
	if err := preflightPHP(migs); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	appliedAny := false
	for _, m := range migs {
		var (
			db      *gorm.DB
			applied map[string]bool
		)
		switch {
		case m.Target == migration.TargetPublic:
			if !wantPublic {
				continue
			}
			db, applied = public, publicApplied
		default:
			if !wantAdmin {
				continue
			}
			db, applied = admin, adminApplied
		}
		if recorded(applied, m) {
			continue
		}
		if m.Unclassified {
			fmt.Println("warning: applying unclassified migration", m.Key)
		}
		if err := applyOne(db, m, cfg, vars, false); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		applied[m.Key] = true
		appliedAny = true
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

// discovery scans the migrations root using the directory/key-prefix pairs in
// scanDirs and returns the full migration set in apply order.
func discovery(root string) ([]migration.File, error) {
	return migration.Discover(root, scanDirs, base, nil)
}

// markApplied records a migration as done without running its SQL. Used by
// -baseline to adopt databases that were migrated by hand before
// schema_migrations existed, so a later `migrate` run does not re-execute DDL.
func markApplied(db *gorm.DB, version string) error {
	return db.Exec(
		"INSERT INTO schema_migrations (version) VALUES (?) "+
			"ON DUPLICATE KEY UPDATE version = version", version).Error
}

// runPHP executes a PHP migration script. Kept as a named function so tests can
// wrap it without spawning a real interpreter.
func runPHP(bin string, args []string) error {
	cmd := exec.Command(bin, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
