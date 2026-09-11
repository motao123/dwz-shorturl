// Package migration provides the single source of truth for the migration set
// applied by cmd/migrate: which files exist, in which order they must run, and
// which target database each one belongs to.
//
// Historically the file lists were hard-coded inside cmd/migrate, so a new
// migration file was silently ignored until someone remembered to edit Go code.
// The list is now derived from directory scans, which makes adding a migration a
// pure file-level operation.
//
// Discovery rules — every migration lives under backend/migrations, which is the
// single directory the tool and ops/one_click_migrate.sh both scan:
//
//	backend/migrations/*.sql       admin schema baseline + admin-side backfills
//	backend/migrations/php/*       PHP-side migrations (.sql and .php), applied
//	                               with the public database selected
//
// Cross-database files never hard-code a database name: they reference the
// management schema with the {{ADMIN_DB}} placeholder, which cmd/migrate
// substitutes from the active configuration before the file is executed.
//
// Ordering is explicit on purpose: the migration set is a schema evolution, so
// lexical order must never be allowed to reorder it by accident. A file is
// placed by, in priority order:
//
//  1. the base sequence declared in cmd/migrate (schema baselines and the
//     backfills everything else depends on);
//  2. an explicit append list passed by the caller, for the rare file that
//     cannot express its position as "after X";
//  3. a `-- migrate: after <version>` directive written in the migration file
//     itself (SQL comments and PHP docblocks both work).
//
// A file matching none of these is still discovered, appended last, and flagged
// as Unclassified so it can never be dropped from `-status` silently.
package migration

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// IsSQL reports whether a migration is plain SQL (executed inline) rather than
// a PHP script run through the php CLI.
func (f File) IsSQL() bool { return strings.HasSuffix(f.Key, ".sql") }

// BareName is Key without the scope prefix ("php/migrate_wjoy_log.sql" ->
// "migrate_wjoy_log.sql"). It is used to build the compatibility aliases below.
func (f File) BareName() string {
	if i := strings.LastIndex(f.Key, "/"); i >= 0 {
		return f.Key[i+1:]
	}
	return f.Key
}

// legacyKeyPrefixes are the numeric prefixes the migration files carried before
// the migration set was unified. A database migrated by the old tool records
// "040_scope_url_hash.sql"; the file is now "php/scope_url_hash.sql". Both must
// be recognised as the same migration so an upgrade never re-runs a rewrite and
// never reports a false pending entry.
var legacyKeyPrefixes = []string{"010", "020", "030", "040", "050", "060", "070", "080", "090", "100", "110", "120", "130", "140", "150", "160", "170", "180", "190", "200"}

// LegacyKeys returns the version keys an older revision of this repository
// recorded for the same migration file. The lookup is deliberately explicit
// rather than "strip any numeric prefix": a real file name may start with
// digits, and a false alias would silently skip a pending migration.
func LegacyKeys(key string) []string {
	var aliases []string
	base := key
	if i := strings.LastIndex(key, "/"); i >= 0 {
		base = key[i+1:]
	}
	for _, p := range legacyKeyPrefixes {
		aliases = append(aliases, p+"_"+base)
	}
	// The PHP migration used to live at migrations/legacy_schema.php, i.e. with
	// no scope and no numeric prefix; an old schema_migrations row looks exactly
	// like that bare file name.
	if key == "php/legacy_schema.php" {
		aliases = append(aliases, "legacy_schema.php")
	}
	return aliases
}

// isMigrationFile filters the files the registry is responsible for. .sql is the
// default; a handful of old migrations ship as PHP scripts because they need to
// inspect data before altering the schema (legacy_schema.php).
func isMigrationFile(name string) bool {
	return strings.HasSuffix(name, ".sql") || strings.HasSuffix(name, ".php")
}

// Target identifies which database a migration is applied to.
type Target string

const (
	// TargetAdmin is the management database (short_urls, users, ...).
	TargetAdmin Target = "admin"
	// TargetPublic is the public frontend database (wjoy_log, members, ...).
	// Cross-database migrations are recorded here after they finish, because
	// they always run with the public database as the active one.
	TargetPublic Target = "public"
)

// File is a single discovered migration.
type File struct {
	// Key is the version recorded in schema_migrations. It is unique across the
	// whole set: bare names inside backend/migrations, scope-prefixed for other
	// directories (e.g. "php/scope_url_hash.sql"), so a version never collides
	// with a backend file of the same name.
	Key string
	// Path is the on-disk location.
	Path string
	// Scope is the source directory, relative to the repository root
	// ("backend/migrations", "backend/migrations/php").
	Scope string
	// Target is the database the file is applied to.
	Target Target
	// Override is the `-- migrate: after <version>` directive, empty when the
	// file does not declare one.
	Override string
	// Unclassified reports that the file was not found in any declared order
	// list and has no override; it is still applied, but last, and the caller
	// should surface a warning so the reviewer can move it into the right slot.
	Unclassified bool
}

// Discover scans the given migration scopes relative to root and returns the
// full migration set in apply order.
//
// scopes is a list of dir:keyPrefix pairs. keyPrefix is prepended to the file
// name when building Key ("" for the backend directory, whose bare names are
// already the historical schema_migrations values).
//
// base lists versions that must always be applied first, in the given order.
// appendOrder lists versions whose relative order is fixed; they are appended
// after the base files in that exact order. Any remaining file (unknown to
// both) is appended at the end by name and flagged Unclassified.
func Discover(root string, scopes [][2]string, base, appendOrder []string) ([]File, error) {
	found := map[string]File{}
	var names []string

	for _, scope := range scopes {
		relDir, keyPrefix := scope[0], scope[1]
		absDir := filepath.Join(root, filepath.FromSlash(relDir))
		entries, err := os.ReadDir(absDir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		for _, entry := range entries {
			if entry.IsDir() || !isMigrationFile(entry.Name()) {
				continue
			}
			key := keyPrefix + entry.Name()
			override, err := readOverride(filepath.Join(absDir, entry.Name()))
			if err != nil {
				return nil, err
			}
			if _, dup := found[key]; dup {
				return nil, fmt.Errorf("duplicate migration version %q in %s", key, relDir)
			}
			found[key] = File{
				Key:      key,
				Path:     filepath.Join(absDir, entry.Name()),
				Scope:    relDir,
				Target:   TargetAdmin,
				Override: override,
			}
			names = append(names, key)
		}
	}

	// Target assignment. A migration is applied to the public database when it
	// manages public-side objects (wjoy_log, members, webhook_queue) — that is
	// every PHP-side file plus public_schema.sql, which creates wjoy_log. The
	// cross-schema files reach the management schema through @admin_schema
	// instead of hard-coded `USE`, so they still work from this side.
	for _, n := range names {
		f := found[n]
		if strings.HasPrefix(f.Key, "php/") || f.Key == "public_schema.sql" {
			f.Target = TargetPublic
			found[n] = f
		}
	}
	ordered, err := orderFiles(found, names, base, appendOrder)
	if err != nil {
		return nil, err
	}
	return ordered, nil
}

func orderFiles(found map[string]File, names, base, appendOrder []string) ([]File, error) {
	// Everything starts as "unordered". Pinned groups and override directives
	// then consume files from that pool in a fixed number of passes; whatever is
	// left over is applied last, by name, and flagged so reviewers notice.
	rest := map[string]File{}
	for k, v := range found {
		rest[k] = v
	}

	var ordered []File
	take := func(key string) {
		f, ok := rest[key]
		if !ok {
			return
		}
		ordered = append(ordered, f)
		delete(rest, key)
	}

	// Pass 1: the declared base sequence (schema baselines, in dependency order).
	for _, key := range base {
		take(key)
	}

	// Pass 2: pinned append order for the PHP-side migrations.
	for _, key := range appendOrder {
		take(key)
	}

	// Pass 3: `-- migrate: after <version>` directives, resolved repeatedly so
	// chains work and an anchor placed by a later pass still gets its dependants.
	for {
		progress := false
		for _, key := range names {
			f, ok := rest[key]
			if !ok || f.Override == "" {
				continue
			}
			idx := indexOf(ordered, f.Override)
			if idx < 0 {
				continue
			}
			// Insert *after* the anchor (index+1), not at it.
			ordered = append(ordered, File{})
			copy(ordered[idx+2:], ordered[idx+1:])
			ordered[idx+1] = f
			delete(rest, key)
			progress = true
		}
		if !progress {
			break
		}
	}

	// Pass 4: leftovers. A brand-new file needs no Go change; it lands here,
	// flagged, until its position is declared.
	leftovers := make([]string, 0, len(rest))
	for k := range rest {
		leftovers = append(leftovers, k)
	}
	sort.Strings(leftovers)
	for _, k := range leftovers {
		f := rest[k]
		f.Unclassified = true
		ordered = append(ordered, f)
	}

	// An override that could not be resolved at all is a typo, not a new file:
	// fail loudly rather than silently mis-ordering a schema change.
	for _, key := range names {
		f, ok := found[key]
		if !ok || f.Override == "" {
			continue
		}
		if indexOf(ordered, f.Override) < 0 {
			return nil, fmt.Errorf("%s declares '-- migrate: after %s' but %s is not in the migration set", key, f.Override, f.Override)
		}
	}
	return ordered, nil
}

func indexOf(list []File, key string) int {
	for i, f := range list {
		if f.Key == key {
			return i
		}
	}
	return -1
}

// overrideRe matches the ordering directive in both SQL (`-- migrate: after X`)
// and PHP docblock (`* -- migrate: after X`) comments.
var overrideRe = regexp.MustCompile(`(?m)^[\s*]*(?:--|//|#)\s*migrate:\s*after\s+(\S+)\s*$`)

// readOverride extracts the optional "-- migrate: after <version>" directive
// from the first 4KB of a file. The directive is a comment, so MySQL ignores it
// when the file is piped in directly, and the PHP docblock form is ignored by
// the PHP interpreter for the same reason.
func readOverride(path string) (string, error) {
	fh, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer fh.Close()

	buf := make([]byte, 4096)
	n, err := fh.Read(buf)
	if err != nil && n == 0 {
		return "", nil
	}
	m := overrideRe.FindSubmatch(buf[:n])
	if m == nil {
		return "", nil
	}
	return string(m[1]), nil
}
