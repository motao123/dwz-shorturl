package main

import (
	"testing"

	"dwz-admin/internal/config"
)

// A configuration that names the same database for both roles is the
// single-database deployment the installer produces; it must be detected
// without requiring the operator to pass -same-db.
func TestSameDatabaseDetectsSingleDBConfig(t *testing.T) {
	cfg := &config.Config{}
	cfg.Database.DBName = "dwz_single"
	cfg.PublicDB.DBName = "dwz_single"
	if !sameDatabase(cfg) {
		t.Fatal("expected same database to be detected")
	}
}

func TestSameDatabaseIgnoresSplitConfig(t *testing.T) {
	cfg := &config.Config{}
	cfg.Database.DBName = "dwz_admin"
	cfg.PublicDB.DBName = "dwz_public"
	if sameDatabase(cfg) {
		t.Fatal("split databases must not be reported as a single database")
	}
}

// An unset public db is filled in by resolvePublicDB from the admin
// credentials; the detection must not fire before that, otherwise the two
// roles would be silently collapsed for a genuinely split deployment whose
// public side is simply absent.
func TestSameDatabaseEmptyPublicIsNotSingle(t *testing.T) {
	cfg := &config.Config{}
	cfg.Database.DBName = "dwz_admin"
	if sameDatabase(cfg) {
		t.Fatal("empty public db name must not be treated as single-database mode")
	}
}
