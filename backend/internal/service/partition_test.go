package service

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/callbacks"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"
)

// ---------------------------------------------------------------------------
// Fake MySQL driver
//
// The partition maintenance path is glue between the calendar and two SQL
// statements, so the interesting branches (正常补建 / ALTER 失败 / 无月度分区 /
// 已覆盖 no-op) are driven through a scripted driver instead of a live server.
// The driver understands exactly the queries the service issues and records
// every statement so the tests can assert on them.
// ---------------------------------------------------------------------------

type fakeDB struct {
	mu      sync.Mutex
	queries []string
	execs   []string
	// partitionRows is the current information_schema state.
	partitionRows []partitionRow
	// execFunc is called for every Exec; returning an error simulates a failing
	// ALTER (DDL lock timeout, disk full, missing privilege, ...).
	execFunc func(query string, args []driver.Value) error
	// failInspect makes the information_schema query itself fail.
	failInspect error
	// queryHook is a debug seam: it sees every query before matching.
	queryHook func(string) ([]string, [][]driver.Value, bool)
	// DDL applied through execFunc is reflected here so a later inspection sees
	// the new partition (needed for "partial progress" assertions).
	applyDDL bool
}

var theFake = struct {
	mu  sync.Mutex
	dbs []*fakeDB
}{}

func registerFakeDB(db *fakeDB) {
	theFake.mu.Lock()
	theFake.dbs = append(theFake.dbs, db)
	theFake.mu.Unlock()
}

func takeFakeDB() *fakeDB {
	theFake.mu.Lock()
	defer theFake.mu.Unlock()
	if len(theFake.dbs) == 0 {
		return nil
	}
	db := theFake.dbs[0]
	theFake.dbs = theFake.dbs[1:]
	return db
}

type fakeConn struct{ db *fakeDB }
type fakeStmt struct {
	conn  *fakeConn
	query string
}

func (c *fakeConn) Prepare(query string) (driver.Stmt, error) {
	return &fakeStmt{conn: c, query: query}, nil
}
func (c *fakeConn) Close() error              { return nil }
func (c *fakeConn) Begin() (driver.Tx, error) { return nil, errors.New("not supported") }

func (s *fakeStmt) Close() error  { return nil }
func (s *fakeStmt) NumInput() int { return -1 }

func (s *fakeStmt) Exec(args []driver.Value) (driver.Result, error) {
	db := s.conn.db
	db.mu.Lock()
	defer db.mu.Unlock()
	db.execs = append(db.execs, s.query)
	if db.execFunc != nil {
		if err := db.execFunc(s.query, args); err != nil {
			return nil, err
		}
	}
	// Apply the DDL after the scripted hook succeeded, so a later inspection
	// observes the new partition (mirrors a committed ALTER).
	if db.applyDDL {
		applyReorganize(db, s.query)
	}
	return driver.RowsAffected(1), nil
}

func (s *fakeStmt) Query(args []driver.Value) (driver.Rows, error) {
	db := s.conn.db
	db.mu.Lock()
	defer db.mu.Unlock()
	db.queries = append(db.queries, s.query)
	if db.queryHook != nil {
		if cols, rows, handled := db.queryHook(s.query); handled {
			return newFakeRows(cols, rows), nil
		}
	}

	q := strings.ToLower(s.query)
	switch {
	case strings.Contains(q, "from information_schema.partitions"):
		if db.failInspect != nil {
			return nil, db.failInspect
		}
		// database/sql cannot use *string values from a driver, so the fake
		// emits plain values (nil for SQL NULL), exactly like a real driver.
		rows := make([][]driver.Value, 0, len(db.partitionRows))
		for _, r := range db.partitionRows {
			rows = append(rows, []driver.Value{
				nullableString(r.PartitionName),
				nullableString(r.PartitionMethod),
				nullableString(r.PartitionDescription),
				nullableInt(r.TableRows),
			})
		}
		return newFakeRows([]string{"PARTITION_NAME", "PARTITION_METHOD", "PARTITION_DESCRIPTION", "TABLE_ROWS"}, rows), nil
	case strings.Contains(q, "max(partition_name)"):
		var latest interface{}
		for _, r := range db.partitionRows {
			if r.PartitionName == nil {
				continue
			}
			if t, ok := parseMonthPartition(*r.PartitionName); ok {
				name := *r.PartitionName
				if latest == nil {
					latest = name
					continue
				}
				if prev, _ := parseMonthPartition(latest.(string)); t.After(prev) {
					latest = name
				}
			}
		}
		return newFakeRows([]string{"MAX(PARTITION_NAME)"}, [][]driver.Value{{latest}}), nil
	}
	return newFakeRows([]string{"x"}, nil), nil
}

// applyReorganize extracts the new pYYYYMM partition from an ALTER statement and
// adds it to the fake information_schema state, so a following inspection sees
// the partition exactly as MySQL would after a committed REORGANIZE.
func applyReorganize(db *fakeDB, query string) {
	fields := strings.Fields(query)
	for i, f := range fields {
		if !strings.EqualFold(f, "PARTITION") || i+1 >= len(fields) {
			continue
		}
		name := fields[i+1]
		if _, ok := parseMonthPartition(name); !ok {
			continue
		}
		desc := "TO_DAYS"
		db.partitionRows = append(db.partitionRows, partitionRow{
			PartitionName:        strPtr(name),
			PartitionMethod:      strPtr("RANGE"),
			PartitionDescription: &desc,
		})
		return
	}
}

type fakeRows struct {
	cols []string
	rows [][]driver.Value
	pos  int
}

func newFakeRows(cols []string, rows [][]driver.Value) *fakeRows {
	if rows == nil {
		rows = [][]driver.Value{}
	}
	return &fakeRows{cols: cols, rows: rows}
}

func (r *fakeRows) Columns() []string { return r.cols }
func (r *fakeRows) Close() error      { return nil }
func (r *fakeRows) Next(dest []driver.Value) error {
	if r.pos >= len(r.rows) {
		return io.EOF
	}
	copy(dest, r.rows[r.pos])
	r.pos++
	return nil
}

// newPartitionTestDB builds a *gorm.DB backed by the scripted driver, seeded
// with the given partitions.
func newPartitionTestDB(t *testing.T, rows []partitionRow) (*gorm.DB, *fakeDB) {
	t.Helper()
	scripted := &fakeDB{partitionRows: rows, applyDDL: true}
	registerFakeDB(scripted)
	registerFakeDriverOnce()

	db, err := gorm.Open(&fakeDialector{}, &gorm.Config{Logger: silentLogger})
	if err != nil {
		t.Fatalf("open fake db: %v", err)
	}
	return db, scripted
}

func strPtr(v string) *string { return &v }
func int64Ptr(v int64) *int64 { return &v }

func monthRow(name string, rows int64) partitionRow {
	return partitionRow{
		PartitionName:        strPtr(name),
		PartitionMethod:      strPtr("RANGE"),
		PartitionDescription: strPtr("TO_DAYS"),
		TableRows:            int64Ptr(rows),
	}
}

func futureRow(rows int64) partitionRow {
	return partitionRow{
		PartitionName:        strPtr("p_future"),
		PartitionMethod:      strPtr("RANGE"),
		PartitionDescription: strPtr("MAXVALUE"),
		TableRows:            int64Ptr(rows),
	}
}

func newCronWithDB(db *gorm.DB) *CronService {
	return &CronService{db: db, logger: zap.NewNop()}
}

func pname(t time.Time) string { return "p" + t.Format("200601") }

// ---------------------------------------------------------------------------
// Pure helpers
// ---------------------------------------------------------------------------

func TestMonthStartAndParseMonth(t *testing.T) {
	in := time.Date(2026, 9, 11, 23, 21, 30, 0, time.Local)
	got := monthStart(in)
	want := time.Date(2026, 9, 1, 0, 0, 0, 0, time.Local)
	if !got.Equal(want) {
		t.Fatalf("monthStart = %v, want %v", got, want)
	}
	if v := parseMonth("202611"); v.Year() != 2026 || v.Month() != time.November {
		t.Fatalf("parseMonth(202611) = %v", v)
	}
	if !parseMonth("not-a-month").IsZero() {
		t.Fatal("invalid month must return zero time")
	}
	if !parseMonth("").IsZero() {
		t.Fatal("empty month must return zero time")
	}
}

func TestParseMonthPartitionRejectsForeignNames(t *testing.T) {
	cases := map[string]bool{
		"p202610":  true,
		"p000000":  false, // catch-all sentinel, not a month
		"p_future": false,
		"202610":   false,
		"p20x610":  false,
		"":         false,
	}
	for name, want := range cases {
		if _, ok := parseMonthPartition(name); ok != want {
			t.Fatalf("parseMonthPartition(%q) ok = %v, want %v", name, ok, want)
		}
	}
}

func TestIsFuturePartition(t *testing.T) {
	if !isFuturePartition("p_future", nil) {
		t.Fatal("p_future must be recognised by name")
	}
	if !isFuturePartition("somethingelse", strPtr("MAXVALUE")) {
		t.Fatal("MAXVALUE must be recognised as the catch-all")
	}
	if isFuturePartition("p202610", strPtr("TO_DAYS('2026-11-01')")) {
		t.Fatal("a month partition must not be treated as the catch-all")
	}
}

func TestMonthsBetween(t *testing.T) {
	cases := []struct {
		a, b time.Time
		want int
	}{
		{time.Date(2026, 9, 1, 0, 0, 0, 0, time.Local), time.Date(2026, 11, 1, 0, 0, 0, 0, time.Local), 2},
		{time.Date(2026, 12, 1, 0, 0, 0, 0, time.Local), time.Date(2027, 2, 1, 0, 0, 0, 0, time.Local), 2},
		{time.Date(2026, 9, 1, 0, 0, 0, 0, time.Local), time.Date(2026, 9, 1, 0, 0, 0, 0, time.Local), 0},
		{time.Date(2026, 10, 1, 0, 0, 0, 0, time.Local), time.Date(2026, 9, 1, 0, 0, 0, 0, time.Local), 0},
	}
	for _, c := range cases {
		if got := monthsBetween(c.a, c.b); got != c.want {
			t.Fatalf("monthsBetween(%v,%v) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

// ---------------------------------------------------------------------------
// pendingPartitionMonths: the idempotency / gap-free contract
// ---------------------------------------------------------------------------

func TestPendingPartitionMonthsNoMonthlyPartitionStartsAtCurrentMonth(t *testing.T) {
	now := time.Date(2026, 9, 11, 10, 0, 0, 0, time.Local)
	got := pendingPartitionMonths(time.Time{}, now, 2)
	want := []string{"202609", "202610", "202611"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("pending = %v, want %v", got, want)
	}
}

func TestPendingPartitionMonthsAlreadyCoveredIsNoOp(t *testing.T) {
	now := time.Date(2026, 9, 11, 10, 0, 0, 0, time.Local)
	if got := pendingPartitionMonths(parseMonth("202611"), now, 2); len(got) != 0 {
		t.Fatalf("pending = %v, want none", got)
	}
	// Even further ahead stays a no-op (never negative).
	if got := pendingPartitionMonths(parseMonth("202701"), now, 2); len(got) != 0 {
		t.Fatalf("pending = %v, want none", got)
	}
}

func TestPendingPartitionMonthsFillsEveryGapIncludingCurrentMonth(t *testing.T) {
	now := time.Date(2026, 9, 11, 10, 0, 0, 0, time.Local)
	// Coverage ended in July. Separating August from the rest would require
	// moving rows between partitions, which REORGANIZE cannot do, so the run
	// starts at the current month and leaves a gap-free range up to the target.
	got := pendingPartitionMonths(parseMonth("202607"), now, 2)
	want := []string{"202609", "202610", "202611"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("pending = %v, want %v", got, want)
	}
}

func TestPendingPartitionMonthsHandlesYearBoundary(t *testing.T) {
	now := time.Date(2026, 12, 31, 23, 0, 0, 0, time.Local)
	got := pendingPartitionMonths(parseMonth("202612"), now, 2)
	want := []string{"202701", "202702"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("pending = %v, want %v", got, want)
	}
}

// ---------------------------------------------------------------------------
// Inspect: coverage, p_future share, mismatch detection
// ---------------------------------------------------------------------------

func TestInspectReportsCoverageAndFutureRatio(t *testing.T) {
	now := time.Now()
	db, _ := newPartitionTestDB(t, []partitionRow{
		monthRow(pname(now.AddDate(0, -1, 0)), 100),
		monthRow(pname(now), 200),
		monthRow(pname(now.AddDate(0, 1, 0)), 300),
		monthRow(pname(now.AddDate(0, 2, 0)), 400),
		futureRow(50),
	})
	c := newCronWithDB(db)

	st := c.InspectClickLogsPartitions()
	if !st.Available || !st.HasFuture {
		t.Fatalf("expected RANGE + p_future, got available=%v has_future=%v mismatch=%q", st.Available, st.HasFuture, st.Mismatch)
	}
	if st.LatestMonth != pname(now.AddDate(0, 2, 0)) {
		t.Fatalf("latest = %q", st.LatestMonth)
	}
	if st.BehindMonths != 0 || !st.Healthy {
		t.Fatalf("coverage should be healthy: behind=%d healthy=%v alerts=%v", st.BehindMonths, st.Healthy, st.Alerts)
	}
	if st.FutureRows != 50 || st.TotalRows != 1050 {
		t.Fatalf("rows = %d/%d, want 50/1050", st.FutureRows, st.TotalRows)
	}
	if st.FutureRatio < 0.047 || st.FutureRatio > 0.048 {
		t.Fatalf("future ratio = %v", st.FutureRatio)
	}
	if st.AlertLevel != "ok" || len(st.Alerts) != 0 {
		t.Fatalf("level=%q alerts=%v", st.AlertLevel, st.Alerts)
	}
}

func TestInspectFlagsLaggingCoverageAsWarning(t *testing.T) {
	now := time.Now()
	db, _ := newPartitionTestDB(t, []partitionRow{
		monthRow(pname(now), 10),
		futureRow(0),
	})
	c := newCronWithDB(db)

	st := c.InspectClickLogsPartitions()
	if st.BehindMonths != 2 {
		t.Fatalf("behind = %d, want 2", st.BehindMonths)
	}
	if st.AlertLevel != "warning" {
		t.Fatalf("level = %q, want warning", st.AlertLevel)
	}
	if len(st.Alerts) == 0 || st.Healthy {
		t.Fatalf("expected alerts and unhealthy, got %v healthy=%v", st.Alerts, st.Healthy)
	}
	if st.PendingMonths == nil || len(st.PendingMonths) != 2 {
		t.Fatalf("pending = %v, want 2 months", st.PendingMonths)
	}
}

func TestInspectFlagsMissingFuturePartitionAsCritical(t *testing.T) {
	now := time.Now()
	db, _ := newPartitionTestDB(t, []partitionRow{
		monthRow(pname(now), 10),
		monthRow(pname(now.AddDate(0, 1, 0)), 10),
		monthRow(pname(now.AddDate(0, 2, 0)), 10),
	})
	c := newCronWithDB(db)

	st := c.InspectClickLogsPartitions()
	if st.Mismatch != partitionMismatchNoFuture {
		t.Fatalf("mismatch = %q", st.Mismatch)
	}
	if st.AlertLevel != "critical" || !st.Failing || st.Healthy {
		t.Fatalf("level=%q failing=%v healthy=%v", st.AlertLevel, st.Failing, st.Healthy)
	}
}

func TestInspectFlagsFutureOverflowAsCritical(t *testing.T) {
	now := time.Now()
	db, _ := newPartitionTestDB(t, []partitionRow{
		monthRow(pname(now), 100),
		monthRow(pname(now.AddDate(0, 1, 0)), 100),
		monthRow(pname(now.AddDate(0, 2, 0)), 100),
		futureRow(900), // 75% of the table fell out of the monthly range
	})
	c := newCronWithDB(db)

	st := c.InspectClickLogsPartitions()
	if st.AlertLevel != "critical" || !st.Failing {
		t.Fatalf("level=%q failing=%v alerts=%v", st.AlertLevel, st.Failing, st.Alerts)
	}
	if !containsSubstring(st.Alerts, "p_future") {
		t.Fatalf("expected a p_future overflow alert, got %v", st.Alerts)
	}
}

func TestInspectMissingTableIsCriticalNotPanic(t *testing.T) {
	db, _ := newPartitionTestDB(t, nil)
	c := newCronWithDB(db)

	st := c.InspectClickLogsPartitions()
	if st == nil {
		t.Fatal("nil status")
	}
	if st.Table != "click_logs" {
		t.Fatalf("table = %q", st.Table)
	}
	if st.AlertLevel != "critical" || !st.Failing {
		t.Fatalf("level=%q failing=%v alerts=%v", st.AlertLevel, st.Failing, st.Alerts)
	}
}

func TestInspectSurvivesQueryFailure(t *testing.T) {
	db, scripted := newPartitionTestDB(t, []partitionRow{monthRow(pname(time.Now()), 1)})
	scripted.failInspect = errors.New("information_schema unavailable")
	c := newCronWithDB(db)

	st := c.InspectClickLogsPartitions()
	if st.AlertLevel != "critical" || !st.Failing {
		t.Fatalf("level=%q failing=%v", st.AlertLevel, st.Failing)
	}
	if st.LastError == "" {
		t.Fatal("expected the failure to be recorded")
	}
}

// A failing maintenance window must be reported, not just logged. This mirrors
// what the monitor endpoint exposes (failing=true ⇒ red badge in the UI).
func TestInspectReportsFailingAfterThreshold(t *testing.T) {
	db, _ := newPartitionTestDB(t, []partitionRow{futureRow(0)})
	c := newCronWithDB(db)
	c.notePartitionRun(nil, errors.New("boom"))
	c.notePartitionRun(nil, errors.New("boom"))

	st := c.InspectClickLogsPartitions()
	if !st.Failing {
		t.Fatal("failing must be true after consecutive failures")
	}
	if st.ConsecutiveFailures < partitionAlertThreshold {
		t.Fatalf("consecutive failures = %d, want >= %d", st.ConsecutiveFailures, partitionAlertThreshold)
	}
	if st.LastErrorAt == nil || st.LastError == "" {
		t.Fatal("last error + timestamp must be exposed")
	}
	if st.AlertLevel != "critical" {
		t.Fatalf("level = %q, want critical", st.AlertLevel)
	}
}

func TestNotePartitionRunResetsOnSuccess(t *testing.T) {
	c := &CronService{}
	c.notePartitionRun(nil, errors.New("ALTER failed: lock wait timeout"))
	if c.partitionFailures != 1 || c.partitionLastError == "" {
		t.Fatalf("failures=%d lastErr=%q", c.partitionFailures, c.partitionLastError)
	}
	c.notePartitionRun([]string{"p202611", "p202612"}, nil)
	if c.partitionFailures != 0 || c.partitionLastError != "" {
		t.Fatalf("failures=%d lastErr=%q after success", c.partitionFailures, c.partitionLastError)
	}
	if c.partitionCreatedLastRun != 2 {
		t.Fatalf("created = %d, want 2", c.partitionCreatedLastRun)
	}
}

// ---------------------------------------------------------------------------
// ensureClickLogsPartitionsAhead: the write path
// ---------------------------------------------------------------------------

func TestEnsureCreatesMissingMonthsUpToTarget(t *testing.T) {
	now := time.Now()
	db, scripted := newPartitionTestDB(t, []partitionRow{
		monthRow(pname(now), 10),
		futureRow(0),
	})
	c := newCronWithDB(db)

	created, err := c.ensureClickLogsPartitionsAhead(2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{pname(now.AddDate(0, 1, 0)), pname(now.AddDate(0, 2, 0))}
	if strings.Join(created, ",") != strings.Join(want, ",") {
		t.Fatalf("created = %v, want %v", created, want)
	}
	if len(scripted.execs) != 2 {
		t.Fatalf("exec count = %d, want 2 ALTERs", len(scripted.execs))
	}
	for _, q := range scripted.execs {
		if !strings.Contains(q, "REORGANIZE PARTITION p_future INTO") {
			t.Fatalf("unexpected DDL: %s", q)
		}
		if !strings.Contains(q, "PARTITION p_future VALUES LESS THAN MAXVALUE") {
			t.Fatalf("DDL must keep the catch-all: %s", q)
		}
	}
	// The boundary must be the first day of the FOLLOWING month, matching the
	// schema's TO_DAYS('<next month>-01') layout.
	if !strings.Contains(scripted.execs[0], "VALUES LESS THAN (TO_DAYS(?))") {
		t.Fatalf("boundary must be bound as a parameter: %s", scripted.execs[0])
	}
}

func TestEnsureIsNoOpWhenAlreadyCovered(t *testing.T) {
	now := time.Now()
	db, scripted := newPartitionTestDB(t, []partitionRow{
		monthRow(pname(now), 10),
		monthRow(pname(now.AddDate(0, 1, 0)), 10),
		monthRow(pname(now.AddDate(0, 2, 0)), 10),
		futureRow(0),
	})
	c := newCronWithDB(db)

	created, err := c.ensureClickLogsPartitionsAhead(2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(created) != 0 {
		t.Fatalf("created = %v, want none", created)
	}
	if len(scripted.execs) != 0 {
		t.Fatalf("no DDL expected, got %v", scripted.execs)
	}
}

func TestEnsureFailsWhenFuturePartitionMissing(t *testing.T) {
	now := time.Now()
	db, scripted := newPartitionTestDB(t, []partitionRow{
		monthRow(pname(now), 10),
	})
	c := newCronWithDB(db)

	_, err := c.ensureClickLogsPartitionsAhead(2)
	if err == nil {
		t.Fatal("expected an error when p_future is missing")
	}
	if !strings.Contains(err.Error(), "p_future") {
		t.Fatalf("error should name p_future: %v", err)
	}
	if len(scripted.execs) != 0 {
		t.Fatal("no DDL must run when the catch-all is missing")
	}
}

func TestEnsureKeepsPartialProgressAndReportsFailedMonth(t *testing.T) {
	now := time.Now()
	db, scripted := newPartitionTestDB(t, []partitionRow{
		monthRow(pname(now), 10),
		futureRow(0),
	})
	// Fail the second ALTER (simulates a DDL lock timeout / disk full).
	scripted.execFunc = func(query string, args []driver.Value) error {
		if !strings.Contains(query, pname(now.AddDate(0, 1, 0))) {
			return errors.New("Lock wait timeout exceeded; try restarting transaction")
		}
		return nil
	}
	c := newCronWithDB(db)

	created, err := c.ensureClickLogsPartitionsAhead(2)
	if err == nil {
		t.Fatal("expected the ALTER failure to propagate")
	}
	if len(created) != 1 || created[0] != pname(now.AddDate(0, 1, 0)) {
		t.Fatalf("created = %v, want the first month only", created)
	}
	var pce *partitionCreateError
	if !errors.As(err, &pce) {
		t.Fatalf("error type = %T, want *partitionCreateError", err)
	}
	if pce.month != pname(now.AddDate(0, 2, 0)) {
		t.Fatalf("failed month = %q, want %q", pce.month, pname(now.AddDate(0, 2, 0)))
	}
	if !strings.Contains(err.Error(), "Lock wait timeout") {
		t.Fatalf("original error must be wrapped: %v", err)
	}
}

func TestEnsurePartitionsWithStatusSurfacesAlertAfterFailure(t *testing.T) {
	now := time.Now()
	db, scripted := newPartitionTestDB(t, []partitionRow{
		monthRow(pname(now), 10),
		futureRow(0),
	})
	scripted.execFunc = func(query string, args []driver.Value) error {
		return errors.New("ALTER TABLE failed")
	}
	c := newCronWithDB(db)

	// Two failing runs cross the alert threshold, mirroring the daily cron.
	if _, err := c.EnsurePartitionsWithStatus(2); err == nil {
		t.Fatal("expected the first run to fail")
	}
	res, err := c.EnsurePartitionsWithStatus(2)
	if err == nil {
		t.Fatal("expected the second run to fail")
	}
	if res == nil || res.Status == nil {
		t.Fatal("result must carry the post-run status even on failure")
	}
	if res.FailedMonth == "" || res.Error == "" {
		t.Fatalf("failed month / error must be reported: %+v", res)
	}
	if res.Status.AlertLevel != "critical" || !res.Status.Failing {
		t.Fatalf("level=%q failing=%v alerts=%v", res.Status.AlertLevel, res.Status.Failing, res.Status.Alerts)
	}
	if res.Status.ConsecutiveFailures != 2 {
		t.Fatalf("consecutive failures = %d, want 2", res.Status.ConsecutiveFailures)
	}
	if res.Status.LastRunAt == nil {
		t.Fatal("a failing run must still record a run timestamp")
	}
}

func TestEnsurePartitionsWithStatusNoOpIsHealthy(t *testing.T) {
	now := time.Now()
	db, scripted := newPartitionTestDB(t, []partitionRow{
		monthRow(pname(now), 10),
		monthRow(pname(now.AddDate(0, 1, 0)), 10),
		monthRow(pname(now.AddDate(0, 2, 0)), 10),
		futureRow(0),
	})
	c := newCronWithDB(db)

	res, err := c.EnsurePartitionsWithStatus(2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Created) != 0 {
		t.Fatalf("created = %v, want none", res.Created)
	}
	if res.Status == nil || !res.Status.Healthy || res.Status.AlertLevel != "ok" {
		t.Fatalf("status = %+v", res.Status)
	}
	if len(scripted.execs) != 0 {
		t.Fatal("no-op must not issue DDL")
	}
}

// Reaching N months ahead is an explicit operator override and must create every
// missing month up to it.
func TestEnsurePartitionsWithStatusExtendsHorizon(t *testing.T) {
	now := time.Now()
	db, _ := newPartitionTestDB(t, []partitionRow{
		monthRow(pname(now), 10),
		monthRow(pname(now.AddDate(0, 1, 0)), 10),
		monthRow(pname(now.AddDate(0, 2, 0)), 10),
		futureRow(0),
	})
	c := newCronWithDB(db)

	res, err := c.EnsurePartitionsWithStatus(4)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Created) != 2 {
		t.Fatalf("created = %v, want 2 months", res.Created)
	}
	if res.Status.BehindMonths != 0 {
		t.Fatalf("behind = %d after catch-up", res.Status.BehindMonths)
	}
}

func TestPreviewDoesNotRunDDL(t *testing.T) {
	now := time.Now()
	db, scripted := newPartitionTestDB(t, []partitionRow{
		monthRow(pname(now), 10),
		futureRow(0),
	})
	c := newCronWithDB(db)

	res, err := c.PreviewEnsurePartitions(2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.DryRun {
		t.Fatal("preview must be flagged as a dry run")
	}
	if len(res.Created) != 2 {
		t.Fatalf("planned = %v, want 2 months", res.Created)
	}
	if len(scripted.execs) != 0 {
		t.Fatal("preview must not issue DDL")
	}
}

func TestEnsureWithoutDBReturnsError(t *testing.T) {
	c := &CronService{}
	if _, err := c.ensureClickLogsPartitionsAhead(2); err == nil {
		t.Fatal("expected an error without a database")
	}
	// Inspect must degrade gracefully instead of panicking.
	st := c.InspectClickLogsPartitions()
	if st == nil || !st.Failing {
		t.Fatalf("status = %+v", st)
	}
}

// A cron-window run must record its outcome so the next monitor poll can see it
// even when the ALTER failed.
func TestCronTaskRecordsFailureForObservability(t *testing.T) {
	now := time.Now()
	db, scripted := newPartitionTestDB(t, []partitionRow{
		monthRow(pname(now), 10),
		futureRow(0),
	})
	scripted.execFunc = func(query string, args []driver.Value) error {
		return errors.New("ddl lock")
	}
	c := newCronWithDB(db)

	c.ensureClickLogsPartitions()

	lastRun, lastErr, failures := c.partitionSnapshotValues()
	if lastRun.IsZero() {
		t.Fatal("the cron run must record a run time")
	}
	if lastErr == "" || failures != 1 {
		t.Fatalf("lastErr=%q failures=%d", lastErr, failures)
	}
	st := c.InspectClickLogsPartitions()
	if st.LastError == "" || st.LastRunAt == nil {
		t.Fatalf("status must expose the failure: %+v", st)
	}
}

func TestNormaliseAheadMonths(t *testing.T) {
	if got := normaliseAheadMonths(0); got != partitionAheadMonths {
		t.Fatalf("0 → %d, want %d", got, partitionAheadMonths)
	}
	if got := normaliseAheadMonths(-3); got != partitionAheadMonths {
		t.Fatalf("-3 → %d, want %d", got, partitionAheadMonths)
	}
	if got := normaliseAheadMonths(4); got != 4 {
		t.Fatalf("4 → %d", got)
	}
	if got := normaliseAheadMonths(999); got != partitionMaxAheadMonths {
		t.Fatalf("999 → %d, want %d", got, partitionMaxAheadMonths)
	}
}

func containsSubstring(list []string, sub string) bool {
	for _, v := range list {
		if strings.Contains(v, sub) {
			return true
		}
	}
	return false
}

var _ = context.Background
var _ = zap.NewNop

// ---------------------------------------------------------------------------
// Scripted driver wiring
//
// Everything below is test-only: production code never sees the fake driver, so
// `go build ./...` is unaffected.
// ---------------------------------------------------------------------------

const fakeDriverName = "partitionfake"

var registerFakeDriverOnce = sync.OnceFunc(func() {
	sql.Register(fakeDriverName, fakeDriverImpl{})
})

var silentLogger = logger.Default.LogMode(logger.Silent)

// fakeDialector satisfies gorm.Dialector. Only connection handling matters here
// because the service issues raw SQL (no AutoMigrate, no schema mapping).
type fakeDialector struct{}

func (fakeDialector) Name() string { return "mysql" }

func (fakeDialector) Initialize(db *gorm.DB) error {
	conn, err := sql.Open(fakeDriverName, "scripted")
	if err != nil {
		return err
	}
	if db.Config != nil {
		if existing, ok := db.Config.ConnPool.(*sql.DB); ok && existing != nil {
			_ = existing.Close()
		}
		db.Config.ConnPool = conn
	}
	// gorm builds the base Statement from Config.ConnPool right after
	// Initialize returns, so the pool must be visible there too.
	if db.Statement != nil {
		db.Statement.ConnPool = conn
	}
	// gorm's raw/row callbacks are not registered for a dialector that is not
	// MySQL's, so wire the standard set explicitly (tests issue raw SQL).
	callbacks.RegisterDefaultCallbacks(db, &callbacks.Config{})
	return nil
}

func (fakeDialector) Migrator(*gorm.DB) gorm.Migrator { return nil }

func (fakeDialector) DataTypeOf(*schema.Field) string { return "" }

func (fakeDialector) DefaultValueOf(*schema.Field) clause.Expression { return nil }

func (fakeDialector) BindVarTo(writer clause.Writer, _ *gorm.Statement, _ interface{}) {
	writer.WriteByte('?')
}

func (fakeDialector) QuoteTo(writer clause.Writer, s string) {
	writer.WriteByte('`')
	writer.WriteString(s)
	writer.WriteByte('`')
}

func (fakeDialector) Explain(sql string, _ ...interface{}) string { return sql }

type fakeDriverImpl struct{}

func (fakeDriverImpl) Open(dsn string) (driver.Conn, error) {
	db := takeFakeDB()
	if db == nil {
		return nil, errors.New("fake driver: no scripted database")
	}
	return &fakeConn{db: db}, nil
}

func nullableString(v *string) driver.Value {
	if v == nil {
		return nil
	}
	return *v
}

func nullableInt(v *int64) driver.Value {
	if v == nil {
		return nil
	}
	return *v
}

func timeNowForTest() time.Time { return time.Now() }

func deref(s *string) string {
	if s == nil {
		return "<nil>"
	}
	return *s
}

// ---------------------------------------------------------------------------
// Partition name whitelist
//
// The ALTER TABLE in ensureClickLogsPartitionsAhead interpolates the partition
// name, because MySQL does not accept a placeholder for an identifier. That
// makes the validator the only thing standing between the DDL and a caller
// supplied string, so it gets its own coverage.
// ---------------------------------------------------------------------------

func TestValidatePartitionNameAcceptsMonthPartitions(t *testing.T) {
	for _, name := range []string{"p202601", "p202609", "p209912", "p000001"} {
		if err := validatePartitionName(name); err != nil {
			t.Fatalf("validatePartitionName(%q) = %v, want nil", name, err)
		}
	}
}

func TestValidatePartitionNameRejectsInjectionAndJunk(t *testing.T) {
	bad := []string{
		"",
		"p",
		"p2026",
		"p2026011",
		"p20260a",
		"p202601 `",
		"p202601 VALUES LESS THAN MAXVALUE",
		"p202601; DROP TABLE click_logs",
		"click_logs",
		"p_future",
		"p-20260",
	}
	for _, name := range bad {
		if err := validatePartitionName(name); err == nil {
			t.Fatalf("validatePartitionName(%q) = nil, want error", name)
		}
	}
}

// TestEnsureValidatesEveryInterpolatedPartitionName runs the real DDL path and
// asserts each executed statement still only contains a whitelisted name.
func TestEnsureValidatesEveryInterpolatedPartitionName(t *testing.T) {
	now := time.Now()
	db, scripted := newPartitionTestDB(t, []partitionRow{
		monthRow(pname(now), 10),
		futureRow(0),
	})
	scripted.execFunc = func(query string, args []driver.Value) error { return nil }

	created, err := newCronWithDB(db).ensureClickLogsPartitionsAhead(2)
	if err != nil {
		t.Fatalf("ensureClickLogsPartitionsAhead: %v", err)
	}
	if _, err := time.Parse("200601", strings.TrimPrefix(created[0], "p")); err != nil {
		t.Fatalf("created name %q is not a month", created[0])
	}
	for _, q := range scripted.execs {
		for _, f := range strings.Fields(q) {
			if strings.HasPrefix(f, "p") && f != "p_future" && f != "PARTITION" {
				if err := validatePartitionName(strings.TrimRight(f, ",")); err != nil {
					t.Fatalf("statement contains a non-whitelisted identifier: %q", f)
				}
			}
		}
	}
}
