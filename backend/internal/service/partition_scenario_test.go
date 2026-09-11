package service

import (
	"database/sql/driver"
	"fmt"
	"strings"
	"testing"
	"time"
)

// TestEndToEndScenario walks the acceptance criteria as a story: a healthy
// table, an ALTER that starts failing, the alert surfacing, and the idempotent
// catch-up repairing it.
func TestEndToEndScenario(t *testing.T) {
	now := time.Now()
	// Start: covered through the target, plus a small p_future.
	db, scripted := newPartitionTestDB(t, []partitionRow{
		monthRow(pname(now.AddDate(0, -1, 0)), 1000),
		monthRow(pname(now), 1000),
		monthRow(pname(now.AddDate(0, 1, 0)), 1000),
		monthRow(pname(now.AddDate(0, 2, 0)), 1000),
		// A few rows naturally fall into the catch-all during a month rollover;
		// well below the overflow threshold.
		futureRow(5),
	})
	c := newCronWithDB(db)

	st := c.InspectClickLogsPartitions()
	t.Logf("1) healthy: coverage=%s~%s target=%s behind=%d level=%s future=%d/%d",
		st.EarliestMonth, st.LatestMonth, st.RequiredMonth, st.BehindMonths, st.AlertLevel, st.FutureRows, st.TotalRows)
	if !st.Healthy || st.AlertLevel != "ok" {
		t.Fatalf("expected a healthy start, got %+v", st)
	}

	// The calendar moves on: next month's partition must now be created, and the
	// ALTER starts failing (DDL lock / full disk) as the table grows.
	// Re-seed so coverage now stops one month short of the target (as if the
	// calendar advanced without the daily task managing to keep up).
	scripted.partitionRows = []partitionRow{
		monthRow(pname(now.AddDate(0, -1, 0)), 1000),
		monthRow(pname(now), 1000),
		monthRow(pname(now.AddDate(0, 1, 0)), 1000),
		futureRow(5),
	}
	scripted.execFunc = func(query string, args []driver.Value) error {
		return fmt.Errorf("Lock wait timeout exceeded")
	}
	if _, err := c.EnsurePartitionsWithStatus(2); err == nil {
		t.Fatal("expected the ALTER to fail")
	}
	c.ensureClickLogsPartitions() // the daily cron window

	st = c.InspectClickLogsPartitions()
	t.Logf("2) failing: behind=%d level=%s failures=%d alerts=%v",
		st.BehindMonths, st.AlertLevel, st.ConsecutiveFailures, st.Alerts)
	if st.AlertLevel != "critical" || !st.Failing {
		t.Fatalf("a repeatedly failing ALTER must be critical, got %+v", st)
	}
	if !containsSubstring(st.Alerts, "连续失败") {
		t.Fatalf("alerts must explain the repeated failure: %v", st.Alerts)
	}
	// The monitor cron table must show the task as failed, not merely "ran".
	_, lastErr, failures := c.partitionSnapshotValues()
	if lastErr == "" || failures < partitionAlertThreshold {
		t.Fatalf("cron bookkeeping missing: lastErr=%q failures=%d", lastErr, failures)
	}

	// The problem is fixed; the idempotent catch-up repairs the coverage.
	scripted.execFunc = nil
	res, err := c.EnsurePartitionsWithStatus(2)
	if err != nil {
		t.Fatalf("catch-up failed: %v", err)
	}
	t.Logf("3) repaired: created=%v pending=%v level=%s", res.Created, res.Pending, res.AlertLevel)
	if len(res.Created) == 0 {
		t.Fatal("expected the catch-up to create months")
	}
	if res.Status.BehindMonths != 0 || res.Status.AlertLevel != "ok" {
		t.Fatalf("coverage must be repaired: %+v", res.Status)
	}

	// Re-running is a no-op: no duplicate partitions, no new DDL.
	before := len(scripted.execs)
	res2, err := c.EnsurePartitionsWithStatus(2)
	if err != nil {
		t.Fatalf("second catch-up failed: %v", err)
	}
	if len(res2.Created) != 0 || len(scripted.execs) != before {
		t.Fatalf("catch-up must be idempotent: created=%v ddl=%d→%d", res2.Created, before, len(scripted.execs))
	}
	// No duplicate partition names in the end state.
	seen := map[string]bool{}
	for _, r := range scripted.partitionRows {
		if r.PartitionName == nil {
			continue
		}
		if seen[*r.PartitionName] {
			t.Fatalf("duplicate partition %s", *r.PartitionName)
		}
		seen[*r.PartitionName] = true
	}
	t.Logf("4) final partitions: %s", strings.Join(st.Months, ","))
}
