package migration

import (
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

// RequiredPHPModules are the extensions the bundled PHP migrations need. They
// are checked in one batch before the first PHP migration is dispatched.
//
// This exists because of a real incident: legacy_schema.php calls
// mysqli_report() at load time, so on a PHP build without ext/mysqli it died
// with a fatal "Call to undefined function mysqli_report()" — after 8 admin
// migrations had already been applied. The operator was left with a
// half-migrated database and a version table that gave no hint about where the
// run stopped. Failing up front turns an obscure mid-run crash into an
// actionable message.
var RequiredPHPModules = []string{"mysqli"}

// phpModuleListRe pulls the module names out of `php -m` output, which is
// loosely formatted (aligned columns, section headers like "[PHP Modules]").
var phpModuleListRe = regexp.MustCompile(`(?m)^[ \t]*([A-Za-z][A-Za-z0-9_]*)[ \t]*$`)

// PHPProbeVersion runs `php -r 'echo PHP_VERSION;'` with the given binary.
// It is a variable so tests can stub the interpreter out.
var PHPProbeVersion = func(bin string) (string, error) {
	out, err := exec.Command(bin, "-r", "echo PHP_VERSION;").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// PHPProbeModules runs `php -m` with the given binary.
var PHPProbeModules = func(bin string) (map[string]string, error) {
	out, err := exec.Command(bin, "-m").Output()
	if err != nil {
		return nil, err
	}
	mods := map[string]string{}
	for _, m := range phpModuleListRe.FindAllStringSubmatch(string(out), -1) {
		mods[strings.ToLower(m[1])] = m[1]
	}
	return mods, nil
}

// CheckPHPReady verifies that the php interpreter that will run the PHP
// migrations exists and carries every required extension, returning a message
// that names the fix rather than the symptom. It must be called before any
// migration is applied so a missing extension cannot leave a half-applied run.
func CheckPHPReady(bin string) error {
	version, err := PHPProbeVersion(bin)
	if err != nil {
		return fmt.Errorf("PHP 迁移需要 php CLI，但无法执行 %q：%w\n"+
			"  请安装 PHP 8.0+，或用 DWZ_PHP_BIN 指向解释器", bin, err)
	}

	mods, err := PHPProbeModules(bin)
	if err != nil {
		return fmt.Errorf("无法读取 PHP 扩展列表（%q -m）：%w", bin, err)
	}

	var missing []string
	for _, want := range RequiredPHPModules {
		if _, ok := mods[strings.ToLower(want)]; !ok {
			missing = append(missing, want)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("PHP %s 缺少扩展 %s，迁移无法继续（会在中途失败并留下半迁移的库）。\n"+
			"  安装示例：apt-get install -y php-mysql / yum install -y php-mysqlnd\n"+
			"  或确认 php.ini 中已启用 extension=%s",
			version, strings.Join(missing, ", "), missing[0])
	}
	return nil
}

// EnsurePHPMigrationRecordsFix explains that a partial run can be resumed: every
// migration is idempotent and recorded in schema_migrations, so re-running the
// tool after installing the extension picks up exactly where it stopped. Printed
// as guidance when a pre-check fails.
func EnsurePHPMigrationRecordsFix(msg string) string {
	return msg + "\n\n  说明：已应用的迁移都记录在 schema_migrations 中，安装扩展后重新执行本工具即可从中断处继续（迁移幂等）。"
}
