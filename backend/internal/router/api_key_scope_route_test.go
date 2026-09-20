package router

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestEveryPublicApiRouteDeclaresScope is the anti-regression pin for #42.
//
// The bug: api_keys.permissions was written by the create endpoint, rendered in
// the admin UI, and never read by any middleware — so a key minted with
// `permissions: []` could call every /public/api endpoint. Enforcing the scope
// per route fixes today's routes, but nothing stopped the NEXT route from being
// added without a scope, which is the same shape as the original defect (the
// column existed and was ignored).
//
// This is a source-level assertion on purpose. A behavioural test over the two
// routes that exist today cannot fail when a third route is added scope-less;
// reading the router source can.
func TestEveryPublicApiRouteDeclaresScope(t *testing.T) {
	root := repoRootForRouter(t)
	src, err := os.ReadFile(filepath.Join(root, "backend", "internal", "router", "router.go"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(src)

	// Isolate the public API group body (from `public := engine.Group("/public/api")`
	// to the closing brace at the same indentation).
	start := strings.Index(body, `public := engine.Group("/public/api")`)
	if start < 0 {
		t.Fatal(`could not find the /public/api group; update this test if it was renamed`)
	}
	// Find the end of the group: the first line that is exactly "\t}" after start.
	rest := body[start:]
	end := strings.Index(rest, "\n\t}\n")
	if end < 0 {
		t.Fatal("could not find the end of the /public/api group block")
	}
	group := rest[:end]

	routeRe := regexp.MustCompile(`public\.(GET|POST|PUT|PATCH|DELETE)\(`)
	matches := routeRe.FindAllStringIndex(group, -1)
	if len(matches) == 0 {
		t.Fatal("no /public/api routes found; update this test if the routes moved")
	}

	// Each route declaration spans from its `public.METHOD(` up to the closing
	// parenthesis of that call. Walk braces/parens to slice it precisely rather
	// than relying on line boundaries (the calls are multi-line).
	for _, m := range matches {
		call := sliceCall(group[m[0]:])
		if !strings.Contains(call, "middleware.RequireApiKey(") {
			t.Errorf("a /public/api route is missing RequireApiKey:\n%s", call)
		}
		if !strings.Contains(call, "middleware.ScopeApiKey(") {
			t.Errorf("a /public/api route does not declare a required scope (#42): "+
				"an API key with no permissions would be able to call it.\n%s", call)
		}
	}
}

// sliceCall returns the substring from the start of a call up to and including
// the parenthesis that closes it, ignoring nesting.
func sliceCall(s string) string {
	open := strings.Index(s, "(")
	if open < 0 {
		return s
	}
	depth := 0
	for i := open; i < len(s); i++ {
		switch s[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return s[:i+1]
			}
		}
	}
	return s
}

// TestApiKeyScopesUsedByRoutesAreDeclared pins the scope constants actually used
// on routes to the exported set, so a typo'd string literal (which would make
// ScopeApiKey reject everything) is caught at build time rather than in prod.
func TestApiKeyScopesUsedByRoutesAreDeclared(t *testing.T) {
	root := repoRootForRouter(t)
	routerSrc, err := os.ReadFile(filepath.Join(root, "backend", "internal", "router", "router.go"))
	if err != nil {
		t.Fatal(err)
	}
	// Only constants may be passed: a raw string literal would bypass the type.
	litRe := regexp.MustCompile(`middleware\.ScopeApiKey\("([^"]*)"\)`)
	if bad := litRe.FindAllStringSubmatch(string(routerSrc), -1); len(bad) > 0 {
		for _, b := range bad {
			t.Errorf("ScopeApiKey 被传入字面量 %q：应使用 middleware.Scope* 常量", b[1])
		}
	}

	constSrc, err := os.ReadFile(filepath.Join(root, "backend", "internal", "middleware", "apikey.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"ScopeShortURLsCreate", "ScopeShortURLsBatch"} {
		if !strings.Contains(string(constSrc), name+" ApiKeyScope") {
			t.Errorf("middleware 中缺少 scope 常量 %s 的声明", name)
		}
	}
}

func repoRootForRouter(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 8; i++ {
		if _, err := os.Stat(filepath.Join(dir, "backend", "internal", "router")); err == nil {
			return dir
		}
		dir = filepath.Dir(dir)
	}
	t.Fatal("could not locate repository root")
	return ""
}

// TestApiKeyManagementRoutesScopeToActor is the anti-regression pin for the
// second half of #40.
//
// The bug: List filtered by user_id, but the revoke and stats handlers looked a
// key up by a bare id with no ownership constraint, so any account holding
// api_keys.revoke could walk the id space and revoke every API key on the
// platform (verified live against a real database: a user with an empty list of
// their own keys revoked a super_admin's key and it stopped working instantly).
//
// The service now exposes RevokeAs/GetByIDAs, and the handlers use them. A
// behavioural test over the two endpoints that exist today cannot fail when a
// THIRD per-key endpoint is added using the ownership-blind variant — which is
// exactly how the original defect survived review. Reading the handler source
// can.
func TestApiKeyManagementRoutesScopeToActor(t *testing.T) {
	root := repoRootForRouter(t)
	src, err := os.ReadFile(filepath.Join(root, "backend", "internal", "handler", "api_key.go"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(src)

	// The ownership-blind variants must not be reachable from the HTTP layer.
	// They stay in the service for internal/background callers.
	if strings.Contains(body, "h.svc.Revoke(") {
		t.Error("api_key handler calls ownership-blind Revoke(): per-key operations must use " +
			"RevokeAs(actorID, id) so a non-owner cannot revoke another account's key (#40)")
	}
	if strings.Contains(body, "h.svc.GetByID(") {
		t.Error("api_key handler calls ownership-blind GetByID(): per-key operations must use " +
			"GetByIDAs(actorID, id) so a non-owner cannot read another account's key metadata (#40)")
	}

	// And the actor-aware variants must actually be the ones in use, so deleting
	// the guard call cannot make this test vacuously pass.
	for _, want := range []string{"RevokeAs(", "GetByIDAs("} {
		if !strings.Contains(body, want) {
			t.Errorf("api_key handler does not call %s: the ownership guard is missing", want)
		}
	}
}
