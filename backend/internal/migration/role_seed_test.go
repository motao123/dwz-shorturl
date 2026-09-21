package migration

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// #22 的种子矩阵必须由 CI 把关，而不是靠人读 SQL：
//   * 资源/动作名写错一个字母，INSERT ... SELECT 匹配不到任何行，**执行成功但一条也不授**
//     ——正是本项目反复出现的"控制面说谎"形状（#4/#22/#30 同源）。
//   * 反过来，把 roles.update / configs.update 这类高危权限顺手授给非超管，
//     会把 #2/#3 刚收口的提权面重新打开。
// 因此这里把种子文件与 router 里真实生效的 RequirePermission 集合做双向比对。

var (
	seedGrantRe = regexp.MustCompile("`resource`\\s*=\\s*'([a-z_]+)'\\s*AND\\s+(?:p\\.)?`action`\\s+IN\\s*\\(([^)]*)\\)")
	roleNameRe  = regexp.MustCompile("`name`\\s*=\\s*'([a-z_]+)'")
	quotedRe    = regexp.MustCompile("'([a-z_.]+)'")
	routerPerm  = regexp.MustCompile(`RequirePermission\("([a-z_]+)",\s*"([a-z_]+)"\)`)
)

func readRepoFile(t *testing.T, parts ...string) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	parts = append([]string{root}, parts...)
	b, err := os.ReadFile(filepath.Join(parts...))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// seedPermissions returns every resource.action pair the seed file grants,
// grouped by the role named in that INSERT statement. Parsing is per-statement
// on purpose: the role lives in the WHERE clause of its own statement, so a
// grant can never be attributed to the wrong role.
func seedPermissions(t *testing.T) map[string]map[string]bool {
	t.Helper()
	sql := readRepoFile(t, "backend", "migrations", "seed_builtin_role_permissions.sql")
	perRole := map[string]map[string]bool{}

	stmts := strings.Split(sql, "INSERT INTO `role_permissions`")
	for _, stmt := range stmts[1:] {
		if i := strings.Index(stmt, ";"); i >= 0 {
			stmt = stmt[:i]
		}
		rm := roleNameRe.FindStringSubmatch(stmt)
		if rm == nil {
			t.Fatal("种子语句里找不到目标角色，#22 的按名解析失配了")
		}
		role := rm[1]
		if perRole[role] == nil {
			perRole[role] = map[string]bool{}
		}
		for _, m := range seedGrantRe.FindAllStringSubmatch(stmt, -1) {
			for _, a := range quotedRe.FindAllStringSubmatch(m[2], -1) {
				perRole[role][m[1]+"."+a[1]] = true
			}
		}
	}
	for _, role := range []string{"admin", "operator", "viewer"} {
		if len(perRole[role]) == 0 {
			t.Fatalf("%s 角色没有解析到任何授权，种子文件或解析逻辑被改坏了", role)
		}
	}
	return perRole
}

func routerPermissionSet(t *testing.T) map[string]bool {
	t.Helper()
	src := readRepoFile(t, "backend", "internal", "router", "router.go")
	out := map[string]bool{}
	for _, m := range routerPerm.FindAllStringSubmatch(src, -1) {
		out[m[1]+"."+m[2]] = true
	}
	if len(out) < 20 {
		t.Fatalf("从 router 解析出的权限点过少(%d)，正则失配了", len(out))
	}
	return out
}

func TestSeedGrantsOnlyRealRouterPermissions(t *testing.T) {
	live := routerPermissionSet(t)
	perRole := seedPermissions(t)
	for role := range perRole {
		if len(perRole[role]) == 0 {
			t.Fatalf("%s 角色没有解析到任何授权", role)
		}
	}
	for role, granted := range perRole {
		for perm := range granted {
			if !live[perm] {
				t.Errorf("%s 被授予 %q，但 router 里没有这个权限点：名字写错会静默不授权（#22）", role, perm)
			}
		}
	}
}

// 提权面必须留在超管手里。这里点名断言，而不是只在注释里说"有意保守"。
func TestSeedKeepsEscalationSurfaceWithSuperAdmin(t *testing.T) {
	perRole := seedPermissions(t)
	forbidden := map[string][]string{
		"admin":    {"roles.create", "roles.update", "roles.delete", "configs.update", "users.delete", "audit.delete"},
		"operator": {"roles.create", "roles.read", "roles.update", "roles.delete", "configs.update", "configs.read", "users.read", "users.create", "users.update", "users.delete", "users.assign_roles", "api_keys.create", "api_keys.revoke", "audit.delete"},
		"viewer":   {"roles.create", "roles.update", "roles.delete", "configs.update", "users.create", "users.update", "users.delete", "users.assign_roles", "short_urls.update", "short_urls.delete", "audit.update", "api_keys.revoke"},
	}
	for role, list := range forbidden {
		for _, perm := range list {
			if perRole[role][perm] {
				t.Errorf("%s 拿到了 %q：这会把 #2/#3 收口的提权或破坏面重新打开", role, perm)
			}
		}
	}
}

// 每个角色都得有实际可用的权限，否则 #22 等于没修。
func TestSeedGivesEveryBuiltinRoleUsableAccess(t *testing.T) {
	perRole := seedPermissions(t)
	if len(perRole["admin"]) < 12 {
		t.Errorf("admin 权限过少(%d)，日常运营跑不起来", len(perRole["admin"]))
	}
	if len(perRole["operator"]) < 4 {
		t.Errorf("operator 权限过少(%d)", len(perRole["operator"]))
	}
	if len(perRole["viewer"]) < 6 {
		t.Errorf("viewer 权限过少(%d)", len(perRole["viewer"]))
	}
	names := make([]string, 0, len(perRole))
	for r := range perRole {
		names = append(names, r)
	}
	sort.Strings(names)
	t.Logf("已授权角色: %v", names)
}
