package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"pulsecheck/internal/config"

	_ "github.com/go-sql-driver/mysql"
	_ "modernc.org/sqlite"
)

const (
	EmailPending = "pending"
	EmailSent    = "sent"
	EmailFailed  = "failed"
	EmailSkipped = "skipped"
)

var (
	ErrUnknownService = errors.New("unknown service")
	ErrBadCategory    = errors.New("invalid category")
	ErrDescription    = errors.New("description too long")
	ErrEmail          = errors.New("invalid email")
	ErrNotFound       = errors.New("report not found")
)

// Category is a fixed problem type shown on the report form.
type Category struct {
	ID    string
	Label string
}

var categoryList = []Category{
	{ID: "down", Label: "Can't access it"},
	{ID: "errors", Label: "Errors"},
	{ID: "slow", Label: "Slow"},
	{ID: "login", Label: "Login or authentication"},
	{ID: "other", Label: "Other"},
}

// Categories is the report form list, in display order.
func Categories() []Category {
	out := make([]Category, len(categoryList))
	copy(out, categoryList)
	return out
}

// ValidCategory reports whether id is one of the fixed problem types.
func ValidCategory(id string) bool {
	for _, c := range categoryList {
		if c.ID == id {
			return true
		}
	}
	return false
}

// CategoryLabel returns the visitor-facing name for a category id.
func CategoryLabel(id string) string {
	for _, c := range categoryList {
		if c.ID == id {
			return c.Label
		}
	}
	return id
}

// SignalLevel classifies a report count against the configured thresholds.
func SignalLevel(count, amber, red int) (class, label string) {
	switch {
	case count >= red:
		return "high", "High report volume"
	case count >= amber:
		return "elevated", "Elevated reports"
	default:
		return "normal", "Normal signal level"
	}
}

// BucketStart floors t to the UTC bucket that contains it.
func BucketStart(t time.Time, minutes int) int64 {
	if minutes <= 0 {
		minutes = 15
	}
	sec := int64(minutes) * 60
	u := t.UTC().Unix()
	return u - u%sec
}

// Store is the SQLite or MySQL database.
type Store struct {
	db            *sql.DB
	dialect       string
	bucketMinutes int
}

// Open connects, migrates, and seeds services when the component table is empty.
// MySQL connection attempts are retried so a database container can finish starting.
func Open(ctx context.Context, cfg config.Config) (*Store, error) {
	attempts := 1
	if cfg.Driver == "mysql" {
		attempts = 15
	}
	var last error
	var db *sql.DB
	for i := 0; i < attempts; i++ {
		opened, err := openDB(cfg)
		if err == nil {
			pingCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
			err = opened.PingContext(pingCtx)
			cancel()
			if err == nil {
				db = opened
				break
			}
			opened.Close()
		}
		last = err
		if i == attempts-1 {
			return nil, fmt.Errorf("database: %w", last)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}

	minutes := cfg.BucketMinutes
	if minutes == 0 {
		minutes = 15
	}
	s := &Store{db: db, dialect: cfg.Driver, bucketMinutes: minutes}
	if err := s.migrate(ctx); err != nil {
		db.Close()
		return nil, err
	}
	if err := s.seed(ctx, cfg.Services); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func openDB(cfg config.Config) (*sql.DB, error) {
	switch cfg.Driver {
	case "sqlite":
		dsn, err := sqliteDSN(cfg.SQLitePath)
		if err != nil {
			return nil, err
		}
		db, err := sql.Open("sqlite", dsn)
		if err != nil {
			return nil, err
		}
		db.SetMaxOpenConns(1)
		return db, nil
	case "mysql":
		db, err := sql.Open("mysql", cfg.DatabaseURL)
		if err != nil {
			return nil, err
		}
		db.SetMaxOpenConns(10)
		db.SetMaxIdleConns(5)
		db.SetConnMaxLifetime(5 * time.Minute)
		return db, nil
	default:
		return nil, fmt.Errorf("unsupported database driver %q", cfg.Driver)
	}
}

func sqliteDSN(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o750); err != nil {
		return "", err
	}
	// modernc.org/sqlite accepts a file URI plus repeated _pragma parameters.
	u := url.URL{Scheme: "file", Path: filepath.ToSlash(abs)}
	return u.String() + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)", nil
}

// Close releases the database.
func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER NOT NULL PRIMARY KEY)`); err != nil {
		return fmt.Errorf("migration table: %w", err)
	}
	var v int
	if err := s.db.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&v); err != nil {
		return fmt.Errorf("migration version: %w", err)
	}
	if v >= 1 {
		return nil
	}
	script := sqliteSchema
	if s.dialect == "mysql" {
		script = mysqlSchema
	}
	if err := execScript(ctx, s.db, script); err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx, `INSERT INTO schema_migrations (version) VALUES (1)`); err != nil {
		return fmt.Errorf("record migration: %w", err)
	}
	return nil
}

func execScript(ctx context.Context, db *sql.DB, script string) error {
	for _, stmt := range strings.Split(script, ";") {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("migrate: %w", err)
		}
	}
	return nil
}

// seed inserts SERVICES only when no components exist yet.
// See .claude/docs/landmines.md.
func (s *Store) seed(ctx context.Context, services []config.Service) error {
	var n int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM components`).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	for i, svc := range services {
		if _, err := s.db.ExecContext(ctx, `INSERT INTO components (slug, name, sort_order, active) VALUES (?, ?, ?, 1)`, svc.Slug, svc.Name, i); err != nil {
			return fmt.Errorf("seed %s: %w", svc.Slug, err)
		}
	}
	return nil
}

// Submission is one visitor report.
type Submission struct {
	Slug         string
	Category     string
	Description  string
	Email        string
	IPHash       string
	Now          time.Time
	DedupeWindow time.Duration
	EmailStatus  string
}

// Submitted is a stored report, or a duplicate that was not stored.
type Submitted struct {
	Duplicate     bool
	ID            int64
	ComponentName string
	Category      string
	Description   string
	Email         string
	CreatedAt     time.Time
}

// Submit stores a report and increments its aggregate, or reports a duplicate.
// The dedupe window is fixed to the clock and is safe across concurrent writers.
func (s *Store) Submit(ctx context.Context, in Submission) (Submitted, error) {
	if !ValidCategory(in.Category) {
		return Submitted{}, ErrBadCategory
	}
	desc := strings.TrimSpace(in.Description)
	if utf8.RuneCountInString(desc) > 2000 {
		return Submitted{}, ErrDescription
	}
	email := strings.TrimSpace(in.Email)
	if email != "" && !validEmail(email) {
		return Submitted{}, ErrEmail
	}
	if strings.TrimSpace(in.IPHash) == "" {
		return Submitted{}, fmt.Errorf("missing ip hash")
	}
	now := in.Now.UTC()
	if in.Now.IsZero() {
		now = time.Now().UTC()
	}
	window := in.DedupeWindow
	if window <= 0 {
		window = 10 * time.Minute
	}
	status := in.EmailStatus
	if status == "" {
		status = EmailSkipped
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Submitted{}, err
	}
	defer tx.Rollback()

	var componentID int64
	var name string
	err = tx.QueryRowContext(ctx, `SELECT id, name FROM components WHERE slug = ? AND active = 1`, in.Slug).Scan(&componentID, &name)
	if errors.Is(err, sql.ErrNoRows) {
		return Submitted{}, ErrUnknownService
	}
	if err != nil {
		return Submitted{}, err
	}

	allowed, err := s.claimWindow(ctx, tx, in.IPHash, componentID, now.Unix(), int64(window/time.Second))
	if err != nil {
		return Submitted{}, err
	}
	if !allowed {
		return Submitted{Duplicate: true}, nil
	}

	res, err := tx.ExecContext(ctx, `
		INSERT INTO reports (component_id, category, description, reporter_email, ip_hash, email_status, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		componentID, in.Category, nullString(desc), nullString(email), in.IPHash, status, now.Unix())
	if err != nil {
		return Submitted{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Submitted{}, err
	}
	if err := s.bumpAggregate(ctx, tx, componentID, in.Category, BucketStart(now, s.bucketMinutes), 1); err != nil {
		return Submitted{}, err
	}
	if err := tx.Commit(); err != nil {
		return Submitted{}, err
	}
	return Submitted{
		ID:            id,
		ComponentName: name,
		Category:      in.Category,
		Description:   desc,
		Email:         email,
		CreatedAt:     now,
	}, nil
}

// claimWindow records that this hash reported this component at now.
// A zero row count means the previous report is still inside the window.
func (s *Store) claimWindow(ctx context.Context, tx *sql.Tx, ipHash string, componentID, now, windowSec int64) (bool, error) {
	q := `INSERT INTO report_windows (ip_hash, component_id, last_at) VALUES (?, ?, ?)
ON CONFLICT(ip_hash, component_id) DO UPDATE SET last_at = excluded.last_at
WHERE report_windows.last_at <= excluded.last_at - ?`
	if s.dialect == "mysql" {
		q = `INSERT INTO report_windows (ip_hash, component_id, last_at) VALUES (?, ?, ?) AS new
ON DUPLICATE KEY UPDATE last_at = IF(report_windows.last_at <= new.last_at - ?, new.last_at, report_windows.last_at)`
	}
	res, err := tx.ExecContext(ctx, q, ipHash, componentID, now, windowSec)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

func (s *Store) bumpAggregate(ctx context.Context, tx *sql.Tx, componentID int64, category string, bucket int64, delta int) error {
	if delta > 0 {
		q := `INSERT INTO aggregates (component_id, category, bucket_start, report_count) VALUES (?, ?, ?, 1)
ON CONFLICT(component_id, category, bucket_start) DO UPDATE SET report_count = report_count + 1`
		if s.dialect == "mysql" {
			q = `INSERT INTO aggregates (component_id, category, bucket_start, report_count) VALUES (?, ?, ?, 1)
ON DUPLICATE KEY UPDATE report_count = report_count + 1`
		}
		_, err := tx.ExecContext(ctx, q, componentID, category, bucket)
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE aggregates SET report_count = report_count - 1 WHERE component_id = ? AND category = ? AND bucket_start = ? AND report_count > 0`, componentID, category, bucket); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `DELETE FROM aggregates WHERE component_id = ? AND category = ? AND bucket_start = ? AND report_count <= 0`, componentID, category, bucket)
	return err
}

// SetEmailStatus updates delivery state after the report is already stored.
func (s *Store) SetEmailStatus(ctx context.Context, id int64, status, errMsg string) error {
	if len(errMsg) > 400 {
		errMsg = errMsg[:400]
	}
	_, err := s.db.ExecContext(ctx, `UPDATE reports SET email_status = ?, email_error = ? WHERE id = ?`, status, nullString(errMsg), id)
	return err
}

// Component is an active service visitors can select.
type Component struct {
	Slug string
	Name string
}

// Point is one public chart bucket. Counts are summed across categories.
type Point struct {
	Start int64
	Count int
}

// ServiceCount is the public total for one service.
type ServiceCount struct {
	Slug     string
	Name     string
	LastHour int
	LastDay  int
}

// Dash is the public aggregate view. It never includes report text.
type Dash struct {
	Components []Component
	FilterSlug string
	FilterName string
	Points     []Point
	Services   []ServiceCount
}

// Dashboard returns 24 hours of aggregate buckets.
// filterSlug empty means all services. An unknown slug returns ErrUnknownService.
func (s *Store) Dashboard(ctx context.Context, filterSlug string, now time.Time) (Dash, error) {
	now = now.UTC()
	comps, err := s.components(ctx)
	if err != nil {
		return Dash{}, err
	}
	var filterName string
	if filterSlug != "" {
		found := false
		for _, c := range comps {
			if c.Slug == filterSlug {
				found = true
				filterName = c.Name
				break
			}
		}
		if !found {
			return Dash{}, ErrUnknownService
		}
	}

	step := int64(s.bucketMinutes) * 60
	end := BucketStart(now, s.bucketMinutes)
	start := end - 24*60*60 + step

	rows, err := s.db.QueryContext(ctx, `
		SELECT c.slug, a.bucket_start, SUM(a.report_count)
		FROM aggregates a
		JOIN components c ON c.id = a.component_id
		WHERE c.active = 1 AND a.bucket_start >= ?
		GROUP BY c.slug, a.bucket_start`, start)
	if err != nil {
		return Dash{}, err
	}
	defer rows.Close()

	counts := map[string]map[int64]int{}
	for rows.Next() {
		var slug string
		var bucket int64
		var n int
		if err := rows.Scan(&slug, &bucket, &n); err != nil {
			return Dash{}, err
		}
		if counts[slug] == nil {
			counts[slug] = map[int64]int{}
		}
		counts[slug][bucket] = n
	}
	if err := rows.Err(); err != nil {
		return Dash{}, err
	}

	var points []Point
	for t := start; t <= end; t += step {
		var n int
		if filterSlug == "" {
			for _, byBucket := range counts {
				n += byBucket[t]
			}
		} else {
			n = counts[filterSlug][t]
		}
		points = append(points, Point{Start: t, Count: n})
	}

	hourCut := now.Add(-time.Hour).Unix()
	services := make([]ServiceCount, 0, len(comps))
	for _, c := range comps {
		sc := ServiceCount{Slug: c.Slug, Name: c.Name}
		for t := start; t <= end; t += step {
			n := counts[c.Slug][t]
			sc.LastDay += n
			if t >= hourCut {
				sc.LastHour += n
			}
		}
		services = append(services, sc)
	}
	return Dash{
		Components: comps,
		FilterSlug: filterSlug,
		FilterName: filterName,
		Points:     points,
		Services:   services,
	}, nil
}

func (s *Store) components(ctx context.Context) ([]Component, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT slug, name FROM components WHERE active = 1 ORDER BY sort_order, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Component
	for rows.Next() {
		var c Component
		if err := rows.Scan(&c.Slug, &c.Name); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// PrivateReport is an admin row. It is never returned by public handlers.
type PrivateReport struct {
	ID          int64
	Component   string
	Category    string
	Description string
	Email       string
	EmailStatus string
	CreatedAt   time.Time
}

// ListReports returns the newest private reports.
func (s *Store) ListReports(ctx context.Context, limit int) ([]PrivateReport, error) {
	if limit < 1 || limit > 200 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT r.id, c.name, r.category, COALESCE(r.description, ''), COALESCE(r.reporter_email, ''), r.email_status, r.created_at
		FROM reports r
		JOIN components c ON c.id = r.component_id
		ORDER BY r.id DESC
		LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PrivateReport
	for rows.Next() {
		var r PrivateReport
		var created int64
		if err := rows.Scan(&r.ID, &r.Component, &r.Category, &r.Description, &r.Email, &r.EmailStatus, &created); err != nil {
			return nil, err
		}
		r.CreatedAt = time.Unix(created, 0).UTC()
		out = append(out, r)
	}
	return out, rows.Err()
}

// DeleteReport removes a private report and decrements its public bucket.
func (s *Store) DeleteReport(ctx context.Context, id int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var componentID, created int64
	var category string
	err = tx.QueryRowContext(ctx, `SELECT component_id, category, created_at FROM reports WHERE id = ?`, id).Scan(&componentID, &category, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM reports WHERE id = ?`, id); err != nil {
		return err
	}
	bucket := BucketStart(time.Unix(created, 0).UTC(), s.bucketMinutes)
	if err := s.bumpAggregate(ctx, tx, componentID, category, bucket, -1); err != nil {
		return err
	}
	return tx.Commit()
}

// AllowLogin applies a fixed 15-minute window of 8 attempts for an IP hash.
func (s *Store) AllowLogin(ctx context.Context, ipHash string, now time.Time) (bool, error) {
	const windowSec int64 = 15 * 60
	const limit = 8
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()

	var attempts int
	var windowStart int64
	err = tx.QueryRowContext(ctx, `SELECT attempts, window_start FROM login_attempts WHERE ip_hash = ?`, ipHash).Scan(&attempts, &windowStart)
	nowU := now.UTC().Unix()
	switch {
	case errors.Is(err, sql.ErrNoRows):
		if _, err := tx.ExecContext(ctx, `INSERT INTO login_attempts (ip_hash, attempts, window_start) VALUES (?, 1, ?)`, ipHash, nowU); err != nil {
			return false, err
		}
	case err != nil:
		return false, err
	case nowU-windowStart >= windowSec:
		if _, err := tx.ExecContext(ctx, `UPDATE login_attempts SET attempts = 1, window_start = ? WHERE ip_hash = ?`, nowU, ipHash); err != nil {
			return false, err
		}
	case attempts >= limit:
		return false, tx.Commit()
	default:
		if _, err := tx.ExecContext(ctx, `UPDATE login_attempts SET attempts = attempts + 1 WHERE ip_hash = ?`, ipHash); err != nil {
			return false, err
		}
	}
	return true, tx.Commit()
}

// Ping checks that the database still answers.
func (s *Store) Ping(ctx context.Context) error { return s.db.PingContext(ctx) }

func nullString(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func validEmail(s string) bool {
	if len(s) < 3 || len(s) > 254 || strings.ContainsAny(s, " \r\n\t") {
		return false
	}
	at := strings.IndexByte(s, '@')
	if at <= 0 || at >= len(s)-1 {
		return false
	}
	return !strings.Contains(s[at+1:], "@")
}
