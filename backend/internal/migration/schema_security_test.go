package migration

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// repoRoot walks up from the package directory to the repository root, which is
// the directory containing backend/.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 8; i++ {
		if _, err := os.Stat(filepath.Join(dir, "backend", "migrations")); err == nil {
			return dir
		}
		dir = filepath.Dir(dir)
	}
	t.Fatal("could not locate repository root")
	return ""
}

// bcryptHashRe matches a bcrypt hash literal. Any literal hash committed to the
// schema (or to the auto-generated PHP installer, or to the seed script) is a
// credential baked into a public repository: anyone can attempt to crack it
// offline, and if it matches the shipped default it is an instant backdoor.
var bcryptHashRe = regexp.MustCompile(`\$2[aby]\$\d\d\$[./A-Za-z0-9]{53}`)

// TestNoHardcodedAdminCredentialInSchema guards a regression that actually
// shipped: schema.sql used to INSERT a fixed bcrypt hash for user "admin". Every
// fresh deployment therefore had a working, publicly-known login until the
// operator happened to change it. Administrator rows must now be created by the
// installer (cmd/createadmin or deploy/docker/init.php) from an operator-supplied
// password.
func TestNoHardcodedAdminCredentialInSchema(t *testing.T) {
	root := repoRoot(t)
	schema := filepath.Join(root, "backend", "migrations", "schema.sql")
	b, err := os.ReadFile(schema)
	if err != nil {
		t.Fatal(err)
	}
	body := string(b)

	if m := bcryptHashRe.FindString(body); m != "" {
		t.Fatalf("schema.sql contains a hardcoded bcrypt hash %q; administrator "+
			"accounts must be created from an operator-supplied password "+
			"(cmd/createadmin / deploy/docker/init.php), never seeded", m)
	}

	// The seed must not insert into users at all: roles/permissions are fine
	// (they carry no secret), a user row is not.
	insertUsers := regexp.MustCompile(`(?is)INSERT\s+INTO\s+` + "`?" + `users` + "`?")
	if insertUsers.MatchString(body) {
		t.Fatal("schema.sql still inserts into `users`; no default administrator may be seeded")
	}
}

// TestNoHardcodedCredentialInInstallerAssets applies the same rule to the other
// files that can create accounts or ship example credentials.
//
// The "no INSERT INTO users" half was previously only enforced for schema.sql.
// deploy/scripts/seed.sql went on shipping an `admin` row with a
// REPLACE_WITH_YOUR_BCRYPT_HASH placeholder, so every fresh split-DB Docker
// deployment got an account that can never log in (and the comment advertised a
// `cmd/generate-hash` that does not exist). Both halves now cover both files.
func TestNoHardcodedCredentialInInstallerAssets(t *testing.T) {
	root := repoRoot(t)
	files := []string{
		filepath.Join(root, "install.sql"),
		filepath.Join(root, "deploy", "scripts", "seed.sql"),
	}
	insertUsers := regexp.MustCompile(`(?is)INSERT\s+INTO\s+` + "`?" + `users` + "`?")
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			t.Fatal(err)
		}
		body := string(b)
		if m := bcryptHashRe.FindString(body); m != "" {
			t.Errorf("%s contains a hardcoded bcrypt hash %q", filepath.Base(f), m)
		}
		if insertUsers.MatchString(body) {
			t.Errorf("%s inserts into `users`; administrator accounts must be created "+
				"by cmd/createadmin (or deploy/docker/init.php) from an operator-supplied "+
				"password, never seeded by a migration/seed script", filepath.Base(f))
		}
	}
}

// TestSuperAdminRoleStillSeeded makes sure the removal above did not also drop
// the role/permission seed: without super_admin the first administrator would
// be created with no permissions at all.
func TestSuperAdminRoleStillSeeded(t *testing.T) {
	root := repoRoot(t)
	b, err := os.ReadFile(filepath.Join(root, "backend", "migrations", "schema.sql"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(b)
	for _, want := range []string{"super_admin", "role_permissions"} {
		if !strings.Contains(body, want) {
			t.Errorf("schema.sql no longer seeds %q; the first admin would have no permissions", want)
		}
	}
}
