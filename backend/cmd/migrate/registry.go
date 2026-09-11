package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// This file replaces the hand-maintained `order` / `publicOrder` filename
// arrays with a directory scan plus a naming convention, so adding a migration
// no longer requires editing Go code (and can no longer be silently ignored).
//
// Naming convention (all files live in migrations/):
//
//	<seq>_<name>.sql              → admin DB (short_urls, users, roles, …)
//	public__<seq>_<name>.sql      → public DB (wjoy_log, members, webhook_queue)
//	<seq>_<name>.public.sql       → same as public__ prefix
//
// Files without a numeric prefix are treated as follows:
//   - known baseline names (schema.sql, public_schema.sql) are ordered first;
//   - anything else is rejected loudly rather than skipped, so a typo cannot
//     turn into "migration silently never runs".

// dbTarget says which database a migration must run against.
type dbTarget int

const (
	adminDB dbTarget = iota
	publicDB
)

// migration describes one discovered SQL file.
type migration struct {
	File   string   // file name, also the schema_migrations version key
	Path   string   // absolute path
	Target dbTarget // which database it belongs to
	Seq    int      // ordering weight (lower runs first)
}

// baselineFiles are applied before numbered migrations, in this exact order.
// schema.sql is the admin baseline, public_schema.sql the public one.
var baselineFiles = []string{"schema.sql", "public_schema.sql"}

var seqPrefixRe = regexp.MustCompile(`^(?:public__)?(\d{3,})_`)

// discoverMigrations scans dir and returns the admin + public migrations in
// apply order. Files that do not follow the convention are reported as errors.
func discoverMigrations(dir string) ([]migration, []migration, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, nil, fmt.Errorf("read migrations dir failed: %w", err)
	}

	var admins, publics []migration

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".sql") {
			continue
		}
		name := e.Name()

		// Baseline files keep their historical position at the front.
		if idx := indexOf(baselineFiles, name); idx >= 0 {
			m := migration{File: name, Path: filepath.Join(dir, name), Seq: -100 + idx}
			if name == "public_schema.sql" {
				m.Target = publicDB
				publics = append(publics, m)
			} else {
				m.Target = adminDB
				admins = append(admins, m)
			}
			continue
		}

		// Non-baseline files MUST carry an explicit numeric prefix so the order
		// is deterministic and reviewable.
		match := seqPrefixRe.FindStringSubmatch(name)
		if match == nil {
			return nil, nil, fmt.Errorf(
				"migration %q does not follow the naming convention: expected <seq>_<name>.sql, "+
					"public__<seq>_<name>.sql or <seq>_<name>.public.sql", name)
		}
		seq := 0
		for _, c := range match[1] {
			seq = seq*10 + int(c-'0')
		}

		m := migration{File: name, Path: filepath.Join(dir, name), Seq: seq}
		if isPublicMigration(name) {
			m.Target = publicDB
			publics = append(publics, m)
		} else {
			m.Target = adminDB
			admins = append(admins, m)
		}
	}

	sortMigrations(admins)
	sortMigrations(publics)
	return admins, publics, nil
}

// isPublicMigration reports whether a numbered migration targets the public DB.
func isPublicMigration(name string) bool {
	if strings.HasPrefix(name, "public__") {
		return true
	}
	lower := strings.ToLower(name)
	return strings.HasSuffix(strings.TrimSuffix(lower, ".sql")+".sql", ".public.sql")
}

func sortMigrations(list []migration) {
	sort.SliceStable(list, func(i, j int) bool {
		if list[i].Seq != list[j].Seq {
			return list[i].Seq < list[j].Seq
		}
		return list[i].File < list[j].File
	})
}

func indexOf(list []string, v string) int {
	for i, s := range list {
		if s == v {
			return i
		}
	}
	return -1
}

// names returns just the file names of a migration slice.
func names(list []migration) []string {
	out := make([]string, 0, len(list))
	for _, m := range list {
		out = append(out, m.File)
	}
	return out
}
