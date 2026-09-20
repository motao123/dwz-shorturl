package migration

import (
	"errors"
	"strings"
	"testing"
)

// missingModuleOutput mimics `php -m` on a build without ext/mysqli.
const phpModulesWithoutMysqli = `[PHP Modules]
Core
ctype
curl
date
json
mbstring
pcre
PDO
tokenizer

[Zend Modules]
`

const phpModulesWithMysqli = `[PHP Modules]
Core
ctype
curl
date
json
mbstring
mysqli
mysqlnd
pcre
PDO
tokenizer

[Zend Modules]
`

// A PHP build without ext/mysqli must be rejected BEFORE any DDL runs: that was
// the real incident (8 migrations applied, then a fatal on mysqli_report()).
func TestCheckPHPReadyFailsOnMissingMysqli(t *testing.T) {
	origVer, origMods := PHPProbeVersion, PHPProbeModules
	defer func() { PHPProbeVersion, PHPProbeModules = origVer, origMods }()

	PHPProbeVersion = func(string) (string, error) { return "8.2.33", nil }
	PHPProbeModules = func(string) (map[string]string, error) {
		return map[string]string{"core": "Core", "pdo": "PDO"}, nil
	}

	err := CheckPHPReady("php")
	if err == nil {
		t.Fatal("expected a failure when ext/mysqli is missing, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "mysqli") {
		t.Errorf("error must name the missing extension, got %q", msg)
	}
	// The message has to be actionable, not just "it failed".
	for _, want := range []string{"php-mysql", "8.2.33"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error message should mention %q, got %q", want, msg)
		}
	}
}

func TestCheckPHPReadyPassesWithMysqli(t *testing.T) {
	origVer, origMods := PHPProbeVersion, PHPProbeModules
	defer func() { PHPProbeVersion, PHPProbeModules = origVer, origMods }()

	PHPProbeVersion = func(string) (string, error) { return "8.2.33", nil }
	PHPProbeModules = func(string) (map[string]string, error) {
		return map[string]string{"mysqli": "mysqli", "core": "Core"}, nil
	}

	if err := CheckPHPReady("php"); err != nil {
		t.Fatalf("expected php with mysqli to pass, got %v", err)
	}
}

// A missing interpreter (no php CLI at all) must also be a clean pre-flight
// failure rather than a mid-run crash on the first PHP migration.
func TestCheckPHPReadyFailsOnMissingInterpreter(t *testing.T) {
	origVer, origMods := PHPProbeVersion, PHPProbeModules
	defer func() { PHPProbeVersion, PHPProbeModules = origVer, origMods }()

	PHPProbeVersion = func(string) (string, error) {
		return "", errors.New("exec: \"php\": executable file not found in $PATH")
	}

	err := CheckPHPReady("php")
	if err == nil {
		t.Fatal("expected a failure when the php binary is absent, got nil")
	}
	if !strings.Contains(err.Error(), "DWZ_PHP_BIN") {
		t.Errorf("error should tell the operator how to point at another interpreter, got %q", err.Error())
	}
}

// The guidance attached to a pre-flight failure must say the run is resumable,
// otherwise an operator who already saw a half-migrated DB will assume the worst.
func TestEnsurePHPMigrationRecordsFixMentionsResume(t *testing.T) {
	out := EnsurePHPMigrationRecordsFix("缺少 mysqli")
	for _, want := range []string{"缺少 mysqli", "schema_migrations", "幂等"} {
		if !strings.Contains(out, want) {
			t.Errorf("guidance should mention %q, got %q", want, out)
		}
	}
}

// `php -m` output is column-formatted; the parser must not pick up the section
// headers ("[PHP Modules]") or padded whitespace as module names.
func TestPHPModuleListParsing(t *testing.T) {
	mods := map[string]string{}
	for _, m := range phpModuleListRe.FindAllStringSubmatch(phpModulesWithoutMysqli, -1) {
		mods[strings.ToLower(m[1])] = m[1]
	}
	if _, ok := mods["mysqli"]; ok {
		t.Error("mysqli must not be detected in output that does not list it")
	}
	for _, want := range []string{"core", "pdo", "curl"} {
		if _, ok := mods[want]; !ok {
			t.Errorf("module %q was not parsed out of `php -m` output", want)
		}
	}
	if _, ok := mods["php"]; ok {
		t.Error("the '[PHP Modules]' section header must not be parsed as a module")
	}
}

// With mysqli present the same parser must find it.
func TestPHPModuleListParsingFindsMysqli(t *testing.T) {
	mods := map[string]string{}
	for _, m := range phpModuleListRe.FindAllStringSubmatch(phpModulesWithMysqli, -1) {
		mods[strings.ToLower(m[1])] = m[1]
	}
	if _, ok := mods["mysqli"]; !ok {
		t.Fatal("mysqli was not detected in output that lists it")
	}
}
