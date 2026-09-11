package migration

import (
	"os"
	"path/filepath"
	"testing"
)

// write creates a fixture file, creating parent directories as needed.
func write(t *testing.T, root, rel, body string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func keys(files []File) []string {
	out := make([]string, 0, len(files))
	for _, f := range files {
		out = append(out, f.Key)
	}
	return out
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// The real repository layout, as scanned by cmd/migrate: backend/migrations is
// the root and php/ is the namespaced sub-directory.
func realScopes() [][2]string {
	return [][2]string{
		{"backend/migrations", ""},
		{"backend/migrations/php", "php/"},
	}
}

var realBase = []string{
	"schema.sql",
	"add_missing_columns.sql",
	"add_password_hash.sql",
	"add_domains.sql",
	"add_webhooks.sql",
	"add_totp.sql",
	"optimize_domain_indexes.sql",
	"public_schema.sql",
}

// A brand-new file with no declared position must still be discovered, applied
// last, and flagged — that is the whole point of dropping the hard-coded list.
func TestDiscoverPicksUpUnknownFile(t *testing.T) {
	root := t.TempDir()
	write(t, root, "backend/migrations/schema.sql", "CREATE TABLE t (id INT);")
	write(t, root, "backend/migrations/public_schema.sql", "CREATE TABLE w (id INT);")
	write(t, root, "backend/migrations/php/add_members.sql", "-- migrate: after public_schema.sql\nCREATE TABLE m (id INT);")
	write(t, root, "backend/migrations/brand_new.sql", "ALTER TABLE t ADD COLUMN x INT;")

	files, err := Discover(root, realScopes(), []string{"schema.sql", "public_schema.sql"}, nil)
	if err != nil {
		t.Fatal(err)
	}

	got := keys(files)
	want := []string{"schema.sql", "public_schema.sql", "php/add_members.sql", "brand_new.sql"}
	if !equal(got, want) {
		t.Fatalf("order = %v, want %v", got, want)
	}
	if !files[3].Unclassified {
		t.Fatalf("new file should be flagged unclassified: %+v", files[3])
	}
	if files[2].Target != TargetPublic {
		t.Fatalf("php/ scope must target the public db, got %q", files[2].Target)
	}
}

// The `-- migrate: after <version>` directive inserts a file right after its
// anchor, which is how the bundled set declares its order.
func TestOverrideDirectiveInsertsAfterAnchor(t *testing.T) {
	root := t.TempDir()
	write(t, root, "backend/migrations/schema.sql", "SELECT 1;")
	write(t, root, "backend/migrations/add_missing_columns.sql", "SELECT 1;")
	write(t, root, "backend/migrations/extra.sql", "-- migrate: after add_missing_columns.sql\nSELECT 1;")

	files, err := Discover(root, [][2]string{{"backend/migrations", ""}}, []string{"schema.sql", "add_missing_columns.sql"}, nil)
	if err != nil {
		t.Fatal(err)
	}

	got := keys(files)
	want := []string{"schema.sql", "add_missing_columns.sql", "extra.sql"}
	if !equal(got, want) {
		t.Fatalf("order = %v, want %v", got, want)
	}
}

// An "after" anchor that does not exist is a hard error: silently dropping the
// migration would reproduce the very bug this registry removes.
func TestOverrideWithUnknownAnchorFails(t *testing.T) {
	root := t.TempDir()
	write(t, root, "backend/migrations/schema.sql", "SELECT 1;")
	write(t, root, "backend/migrations/extra.sql", "-- migrate: after nope.sql\nSELECT 1;")

	if _, err := Discover(root, [][2]string{{"backend/migrations", ""}}, []string{"schema.sql"}, nil); err == nil {
		t.Fatal("expected error for unknown anchor, got nil")
	}
}

// Namespaced scopes must not collide with bare backend file names.
func TestKeyNamespacingAvoidsCollision(t *testing.T) {
	root := t.TempDir()
	write(t, root, "backend/migrations/add_members.sql", "SELECT 1;")
	write(t, root, "backend/migrations/php/add_members.sql", "SELECT 1;")

	files, err := Discover(root, realScopes(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	got := keys(files)
	want := []string{"add_members.sql", "php/add_members.sql"}
	if !equal(got, want) {
		t.Fatalf("order = %v, want %v", got, want)
	}
}

// A PHP migration declaring its anchor inside a docblock must be picked up too.
func TestPHPOverrideInDocblock(t *testing.T) {
	root := t.TempDir()
	write(t, root, "backend/migrations/php/a.sql", "SELECT 1;")
	write(t, root, "backend/migrations/php/b.php", "<?php\n/**\n * -- migrate: after php/a.sql\n */\n")

	files, err := Discover(root, realScopes(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := keys(files); !equal(got, []string{"php/a.sql", "php/b.php"}) {
		t.Fatalf("order = %v, want [php/a.sql php/b.php]", got)
	}
	if files[1].IsSQL() {
		t.Fatalf("a .php migration must not be reported as SQL")
	}
}

// The real repository layout must always discover every bundled migration; this
// is the regression guard against a new file being silently ignored.
func TestRealRepoDiscoversEverything(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	files, err := Discover(root, realScopes(), realBase, nil)
	if err != nil {
		t.Fatal(err)
	}

	seen := map[string]bool{}
	for _, f := range files {
		seen[f.Key] = true
	}
	for _, want := range []string{
		"schema.sql", "public_schema.sql",
		"php/scope_url_hash.sql", "php/add_webhook_queue.sql",
		"php/add_violation_reviews.sql", "php/add_members.sql",
		"php/legacy_schema.php", "php/migrate_wjoy_log.sql",
	} {
		if !seen[want] {
			t.Errorf("migration %s was not discovered (got %v)", want, keys(files))
		}
	}
	if len(files) < 13 {
		t.Errorf("expected at least 13 migrations, got %d: %v", len(files), keys(files))
	}
	// scope_url_hash must come after public_schema (it rewrites wjoy_log rows).
	idx := map[string]int{}
	for i, f := range files {
		idx[f.Key] = i
	}
	if idx["php/scope_url_hash.sql"] < idx["public_schema.sql"] {
		t.Errorf("php/scope_url_hash.sql must be applied after public_schema.sql")
	}
}

// The real apply order is a hard contract, not an implementation detail: each of
// these relations was a real bug found by running the tool against MySQL.
func TestRealRepoApplyOrderContract(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	files, err := Discover(root, realScopes(), realBase, nil)
	if err != nil {
		t.Fatal(err)
	}

	pos := map[string]int{}
	for i, f := range files {
		pos[f.Key] = i
	}

	// Each pair is "must be applied before".
	before := [][2]string{
		// add_missing_columns supplies the member_id column the other backfills
		// anchor on and the schema.sql baseline does not guarantee on old DBs.
		{"add_missing_columns.sql", "add_domains.sql"},
		// public_schema.sql creates wjoy_log, which everything PHP-side touches.
		{"public_schema.sql", "php/legacy_schema.php"},
		// legacy_schema.php backfills wjoy_log.url_hash, which scope_url_hash
		// then rewrites; without it the rewrite fails on an unknown column.
		{"php/legacy_schema.php", "php/scope_url_hash.sql"},
		// migrate_wjoy_log copies legacy rows into short_urls; scope_url_hash
		// must run after so those copied rows also get scoped hashes.
		{"php/migrate_wjoy_log.sql", "php/scope_url_hash.sql"},
	}
	for _, pair := range before {
		a, okA := pos[pair[0]]
		b, okB := pos[pair[1]]
		if !okA || !okB {
			t.Errorf("missing migration in contract: %s -> %s", pair[0], pair[1])
			continue
		}
		if a >= b {
			t.Errorf("%s (index %d) must come before %s (index %d)", pair[0], a, pair[1], b)
		}
	}

	// Unclassified files are applied last, which is only acceptable for a file
	// whose position has not been declared yet. The bundled set must be clean.
	for _, f := range files {
		if f.Unclassified {
			t.Errorf("%s is unclassified; declare its position with '-- migrate: after <version>'", f.Key)
		}
	}
}

// Every bundled migration must be reachable from the version key an older tool
// would have recorded, otherwise an upgrade would re-run an already-applied
// rewrite (or report a false pending entry for it).
func TestLegacyKeysCoverHistoricalNames(t *testing.T) {
	cases := map[string]string{
		"040_scope_url_hash.sql":      "php/scope_url_hash.sql",
		"140_migrate_wjoy_log.sql":    "php/migrate_wjoy_log.sql",
		"070_add_domains.sql":         "add_domains.sql",
		"100_add_missing_columns.sql": "add_missing_columns.sql",
	}
	for legacy, current := range cases {
		found := false
		for _, a := range LegacyKeys(current) {
			if a == legacy {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("LegacyKeys(%q) does not cover the historical name %q", current, legacy)
		}
	}
	if got := LegacyKeys("php/add_members.sql"); len(got) == 0 {
		t.Error("LegacyKeys must not be empty")
	}
}
