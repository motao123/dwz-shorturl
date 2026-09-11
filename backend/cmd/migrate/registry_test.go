package main

import (
	"os"
	"path/filepath"
	"testing"
)

// write creates a file with the given name in dir.
func write(t *testing.T, dir, name string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte("SELECT 1;"), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

// A new numbered file must be discovered with zero Go changes: this is the
// acceptance criterion that used to require editing the `order` array.
func TestDiscoverPicksUpNewFilesWithoutCodeChange(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "schema.sql")
	write(t, dir, "public_schema.sql")
	write(t, dir, "010_add_domains.sql")
	write(t, dir, "100_brand_new.sql") // never mentioned in any Go file

	admins, publics, err := discoverMigrations(dir)
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if !containsFile(admins, "100_brand_new.sql") {
		t.Fatalf("new migration not discovered; admins=%v", names(admins))
	}
	if !containsFile(publics, "public_schema.sql") {
		t.Fatalf("public_schema.sql not classified as public; publics=%v", names(publics))
	}
}

// Ordering must be deterministic: baselines first, then by numeric prefix.
func TestDiscoverOrdering(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "030_third.sql")
	write(t, dir, "010_first.sql")
	write(t, dir, "schema.sql")

	admins, _, err := discoverMigrations(dir)
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	want := []string{"schema.sql", "010_first.sql", "030_third.sql"}
	if got := names(admins); !equal(got, want) {
		t.Fatalf("order = %v, want %v", got, want)
	}
}

// Public migrations must be routed to the public DB by both supported suffixes.
func TestDiscoverPublicRouting(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "public__010_members.sql")
	write(t, dir, "020_webhook.public.sql")
	write(t, dir, "030_admin_only.sql")

	admins, publics, err := discoverMigrations(dir)
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if !equal(names(publics), []string{"public__010_members.sql", "020_webhook.public.sql"}) {
		t.Fatalf("publics = %v", names(publics))
	}
	if !equal(names(admins), []string{"030_admin_only.sql"}) {
		t.Fatalf("admins = %v", names(admins))
	}
}

// A file that breaks the convention must fail loudly instead of being skipped:
// silently ignoring it is exactly the bug this change fixes.
func TestDiscoverRejectsUnconventionalName(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "schema.sql")
	write(t, dir, "forgot_the_prefix.sql")

	if _, _, err := discoverMigrations(dir); err == nil {
		t.Fatal("expected an error for a migration without a sequence prefix")
	}
}

func TestSubstituteVars(t *testing.T) {
	in := "USE `{{PUBLIC_DB}}`;\nJOIN `{{ADMIN_DB}}`.`short_urls` s"
	got := substituteVars(in, map[string]string{"PUBLIC_DB": "pub", "ADMIN_DB": "adm"})
	want := "USE `pub`;\nJOIN `adm`.`short_urls` s"
	if got != want {
		t.Fatalf("substituteVars = %q, want %q", got, want)
	}
}

func TestValidateDBName(t *testing.T) {
	for _, ok := range []string{"dwz_admin", "1_xk7_cn", "db-1", "a$b"} {
		if err := validateDBName(ok); err != nil {
			t.Fatalf("validateDBName(%q) = %v, want nil", ok, err)
		}
	}
	for _, bad := range []string{"", "a`b", "a;b", "a b", "a'b"} {
		if err := validateDBName(bad); err == nil {
			t.Fatalf("validateDBName(%q) = nil, want error", bad)
		}
	}
}

func containsFile(list []migration, name string) bool {
	for _, m := range list {
		if m.File == name {
			return true
		}
	}
	return false
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

// Cross-DB admin migrations read public tables (040_scope_url_hash.sql joins
// wjoy_log, 140_migrate_wjoy_log.sql reads it). The public baseline therefore
// has to be ordered before them, which is why main() applies public first.
// This test pins the classification the ordering relies on.
func TestCrossDBMigrationsAreAdminTargeted(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "public_schema.sql")
	write(t, dir, "040_scope_url_hash.sql")
	write(t, dir, "140_migrate_wjoy_log.sql")

	admins, publics, err := discoverMigrations(dir)
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if !equal(names(publics), []string{"public_schema.sql"}) {
		t.Fatalf("publics = %v, want [public_schema.sql]", names(publics))
	}
	if !equal(names(admins), []string{"040_scope_url_hash.sql", "140_migrate_wjoy_log.sql"}) {
		t.Fatalf("admins = %v", names(admins))
	}
}
