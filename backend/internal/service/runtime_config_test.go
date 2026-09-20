package service

import (
	"errors"
	"testing"

	"dwz-admin/internal/model"
)

type cfgRepoStub struct {
	rows []model.SystemConfig
	err  error
}

func (c *cfgRepoStub) GetAll() ([]model.SystemConfig, error) {
	if c.err != nil {
		return nil, c.err
	}
	return c.rows, nil
}
func (c *cfgRepoStub) GetByKey(string) (*model.SystemConfig, error) { return nil, errors.New("nf") }
func (c *cfgRepoStub) Upsert(*model.SystemConfig) error             { return nil }
func (c *cfgRepoStub) BatchUpdate([]model.SystemConfig) error       { return nil }

func row(key, value string) model.SystemConfig {
	return model.SystemConfig{ConfigKey: key, ConfigValue: value}
}

// TestRuntimeConfig_DefaultsMatchPHP makes sure an unconfigured deployment
// behaves the same on the Go path as on the PHP path, so shipping config
// awareness does not silently change behaviour for existing installs.
func TestRuntimeConfig_DefaultsMatchPHP(t *testing.T) {
	rc := NewRuntimeConfig(&cfgRepoStub{}, nil)

	if !rc.AllowCustomCode() {
		t.Error("allow_custom_code should default to true (PHP default)")
	}
	if rc.RequireRegistration() {
		t.Error("require_registration should default to false (PHP default)")
	}
	if got := rc.BatchMaxURLs(); got != defaultBatchMaxURLs {
		t.Errorf("batch max default = %d, want %d", got, defaultBatchMaxURLs)
	}
	for _, d := range []int{0, 1, 7, 30, 365} {
		if !rc.IsExpireDaysAllowed(d) {
			t.Errorf("expire_days %d should be allowed by default", d)
		}
	}
	if rc.IsExpireDaysAllowed(999999) {
		t.Error("999999 days must not be allowed")
	}
}

// TestRuntimeConfig_ReadsOperatorsValues is the core of #4: the switches the
// admin UI writes must actually reach the Go runtime.
func TestRuntimeConfig_ReadsOperatorsValues(t *testing.T) {
	rc := NewRuntimeConfig(&cfgRepoStub{rows: []model.SystemConfig{
		row(KeyAllowCustomCode, "false"),
		row(KeyRequireRegistration, "true"),
		row(KeyBatchMaxURLs, "250"),
		row(KeyAllowedExpireDays, "[0,1,30]"),
	}}, nil)

	if rc.AllowCustomCode() {
		t.Error("allow_custom_code=false was ignored")
	}
	if !rc.RequireRegistration() {
		t.Error("require_registration=true was ignored")
	}
	if got := rc.BatchMaxURLs(); got != 250 {
		t.Errorf("batch max = %d, want 250", got)
	}
	if rc.IsExpireDaysAllowed(7) {
		t.Error("7 should no longer be allowed once the whitelist is [0,1,30]")
	}
	if !rc.IsExpireDaysAllowed(30) {
		t.Error("30 should be allowed by [0,1,30]")
	}
}

// TestRuntimeConfig_CommaForm covers the PHP-style comma list as well as the
// JSON array the config catalog stores, since both have existed in this repo.
func TestRuntimeConfig_CommaForm(t *testing.T) {
	rc := NewRuntimeConfig(&cfgRepoStub{rows: []model.SystemConfig{
		row(KeyAllowedExpireDays, "0, 7,365"),
	}}, nil)

	for _, d := range []int{0, 7, 365} {
		if !rc.IsExpireDaysAllowed(d) {
			t.Errorf("expire_days %d should be allowed", d)
		}
	}
	if rc.IsExpireDaysAllowed(1) {
		t.Error("1 should not be allowed")
	}
}

// TestRuntimeConfig_InvalidValuesFallBack guards the "never delete more than
// intended" style of failure: a garbage value must not turn into a live-but-wrong
// setting (e.g. a batch cap of 0 rejecting every request).
func TestRuntimeConfig_InvalidValuesFallBack(t *testing.T) {
	rc := NewRuntimeConfig(&cfgRepoStub{rows: []model.SystemConfig{
		row(KeyBatchMaxURLs, "not-a-number"),
		row(KeyAllowedExpireDays, "[1,x]"),
	}}, nil)

	if got := rc.BatchMaxURLs(); got != defaultBatchMaxURLs {
		t.Errorf("garbage batch max = %d, want default %d", got, defaultBatchMaxURLs)
	}
	if !rc.IsExpireDaysAllowed(7) {
		t.Error("garbage whitelist should fall back to the default list")
	}
}

// TestRuntimeConfig_InvalidateMakesChangeImmediate proves the config save path
// can drop the cache; without it a toggle would appear saved but stay inert.
func TestRuntimeConfig_InvalidateMakesChangeImmediate(t *testing.T) {
	repo := &cfgRepoStub{rows: []model.SystemConfig{row(KeyAllowCustomCode, "true")}}
	rc := NewRuntimeConfig(repo, nil)

	if !rc.AllowCustomCode() {
		t.Fatal("expected the initial value to be read")
	}
	repo.rows = []model.SystemConfig{row(KeyAllowCustomCode, "false")}
	if !rc.AllowCustomCode() {
		t.Fatal("cache should still hold the old value (true) before Invalidate")
	}
	rc.Invalidate()
	if rc.AllowCustomCode() {
		t.Error("after Invalidate the new value (false) must be visible")
	}
}

// TestRuntimeConfig_DBErrorIsLoudAndConservative verifies the degraded path:
// when system_configs cannot be read the dangerous switches close, not open.
func TestRuntimeConfig_DBErrorIsLoudAndConservative(t *testing.T) {
	rc := NewRuntimeConfig(&cfgRepoStub{err: errors.New("db down")}, nil)

	if rc.AllowCustomCode() {
		t.Error("with an unreadable config, custom codes must be refused (fail-closed)")
	}
	if !rc.RequireRegistration() {
		t.Error("with an unreadable config, anonymous creation must be refused")
	}
	if rc.LoadError() == nil {
		t.Error("LoadError must surface the degraded state")
	}
}

// TestRuntimeConfig_MemberRateLimit covers the #8/#37 quota resolver: the admin
// UI must be able to tune both numbers, illegal values must fall back (not
// silently disable the gate), and an explicit 0 must stay 0 because that is the
// documented "unlimited" opt-out.
func TestRuntimeConfig_MemberRateLimit(t *testing.T) {
	// Defaults: an install that never opened the config page still gets a bound.
	rc := NewRuntimeConfig(&cfgRepoStub{}, nil)
	if max, window := rc.MemberRateLimit(); max != defaultMemberRateMax || window.Seconds() != defaultMemberRateWindow {
		t.Fatalf("defaults = (%d, %v), want (%d, %ds)",
			max, window, defaultMemberRateMax, defaultMemberRateWindow)
	}

	// Operator values win.
	rc = NewRuntimeConfig(&cfgRepoStub{rows: []model.SystemConfig{
		row(KeyMemberRateMax, "30"),
		row(KeyMemberRateWindow, "120"),
	}}, nil)
	if max, window := rc.MemberRateLimit(); max != 30 || window.Seconds() != 120 {
		t.Fatalf("operator values = (%d, %v), want (30, 120s)", max, window)
	}

	// Explicit 0 is the documented opt-out and must survive.
	rc = NewRuntimeConfig(&cfgRepoStub{rows: []model.SystemConfig{row(KeyMemberRateMax, "0")}}, nil)
	if max, _ := rc.MemberRateLimit(); max != 0 {
		t.Fatalf("explicit 0 must mean unlimited, got %d", max)
	}

	// Garbage must fall back to the default, never to "unlimited".
	rc = NewRuntimeConfig(&cfgRepoStub{rows: []model.SystemConfig{
		row(KeyMemberRateMax, "abc"),
		row(KeyMemberRateWindow, "-5"),
	}}, nil)
	if max, window := rc.MemberRateLimit(); max != defaultMemberRateMax || window.Seconds() != defaultMemberRateWindow {
		t.Fatalf("garbage values = (%d, %v), want defaults", max, window)
	}

	// An absurd window is clamped back to the default so a typo cannot create a
	// window that rejects every request.
	rc = NewRuntimeConfig(&cfgRepoStub{rows: []model.SystemConfig{row(KeyMemberRateWindow, "999999")}}, nil)
	if _, window := rc.MemberRateLimit(); window.Seconds() != defaultMemberRateWindow {
		t.Fatalf("absurd window = %v, want default %ds", window, defaultMemberRateWindow)
	}
}
