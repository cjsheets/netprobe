package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/chadsheets/netprobe/internal/model"
	_ "modernc.org/sqlite"
)

type Store struct{ DB *sql.DB }

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(8)
	for _, q := range []string{"PRAGMA journal_mode=WAL", "PRAGMA synchronous=NORMAL", "PRAGMA busy_timeout=5000", "PRAGMA foreign_keys=ON"} {
		if _, err = db.Exec(q); err != nil {
			db.Close()
			return nil, fmt.Errorf("configure sqlite: %w", err)
		}
	}
	s := &Store{DB: db}
	if err = s.Migrate(context.Background()); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.DB.Close() }

var migrations = []string{
	`CREATE TABLE IF NOT EXISTS schema_migrations(version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS observations(
 id INTEGER PRIMARY KEY AUTOINCREMENT, ts TEXT NOT NULL, target TEXT NOT NULL, probe_type TEXT NOT NULL,
 success INTEGER NOT NULL, latency_ms REAL, error_category TEXT NOT NULL DEFAULT '', error_message TEXT NOT NULL DEFAULT '',
 destination TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT '', interface TEXT NOT NULL DEFAULT '', route TEXT NOT NULL DEFAULT '');
CREATE INDEX IF NOT EXISTS observations_ts ON observations(ts);
CREATE INDEX IF NOT EXISTS observations_target_ts ON observations(target, ts);
CREATE TABLE IF NOT EXISTS gaps(id INTEGER PRIMARY KEY AUTOINCREMENT, start_ts TEXT NOT NULL, end_ts TEXT NOT NULL, kind TEXT NOT NULL, details TEXT NOT NULL DEFAULT '');
CREATE TABLE IF NOT EXISTS markers(id INTEGER PRIMARY KEY AUTOINCREMENT, ts TEXT NOT NULL, message TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS incidents(id INTEGER PRIMARY KEY AUTOINCREMENT, start_ts TEXT NOT NULL, end_ts TEXT, scope TEXT NOT NULL, explanation TEXT NOT NULL, affected_targets TEXT NOT NULL, probe_types TEXT NOT NULL, packet_loss REAL NOT NULL, latency_change_ms REAL NOT NULL DEFAULT 0);
CREATE TABLE IF NOT EXISTS rollups_minute(
 bucket_ts TEXT NOT NULL, target TEXT NOT NULL, probe_type TEXT NOT NULL, count INTEGER NOT NULL, successful_count INTEGER NOT NULL,
 packet_loss REAL NOT NULL, min_ms REAL, median_ms REAL, max_ms REAL, mean_ms REAL, jitter_ms REAL, p90_ms REAL, p95_ms REAL, p99_ms REAL,
 PRIMARY KEY(bucket_ts,target,probe_type));`,
	`CREATE TABLE IF NOT EXISTS collector_state(key TEXT PRIMARY KEY, value TEXT NOT NULL);`,
	`CREATE UNIQUE INDEX IF NOT EXISTS incidents_start_unique ON incidents(start_ts);`,
	`ALTER TABLE incidents ADD COLUMN markers TEXT NOT NULL DEFAULT '[]';`,
}

func (s *Store) Migrate(ctx context.Context) error {
	if _, err := s.DB.ExecContext(ctx, "CREATE TABLE IF NOT EXISTS schema_migrations(version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL)"); err != nil {
		return err
	}
	for i, m := range migrations {
		v := i + 1
		var n int
		_ = s.DB.QueryRowContext(ctx, "SELECT count(*) FROM schema_migrations WHERE version=?", v).Scan(&n)
		if n > 0 {
			continue
		}
		tx, e := s.DB.BeginTx(ctx, nil)
		if e != nil {
			return e
		}
		if _, e = tx.ExecContext(ctx, m); e == nil {
			_, e = tx.ExecContext(ctx, "INSERT INTO schema_migrations(version,applied_at) VALUES(?,?)", v, time.Now().UTC().Format(time.RFC3339Nano))
		}
		if e != nil {
			tx.Rollback()
			return fmt.Errorf("migration %d: %w", v, e)
		}
		if e = tx.Commit(); e != nil {
			return e
		}
	}
	return nil
}

func (s *Store) InsertObservation(ctx context.Context, o model.Observation) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO observations(ts,target,probe_type,success,latency_ms,error_category,error_message,destination,source,interface,route) VALUES(?,?,?,?,?,?,?,?,?,?,?)`, o.Timestamp.UTC().Format(time.RFC3339Nano), o.Target, o.ProbeType, o.Success, o.LatencyMS, o.ErrorCategory, o.ErrorMessage, o.Destination, o.Source, o.Interface, o.Route)
	return err
}

func (s *Store) AddMarker(ctx context.Context, at time.Time, msg string) error {
	_, e := s.DB.ExecContext(ctx, "INSERT INTO markers(ts,message) VALUES(?,?)", at.UTC().Format(time.RFC3339Nano), msg)
	return e
}
func (s *Store) AddGap(ctx context.Context, g model.Gap) error {
	_, e := s.DB.ExecContext(ctx, "INSERT INTO gaps(start_ts,end_ts,kind,details) VALUES(?,?,?,?)", g.Start.UTC().Format(time.RFC3339Nano), g.End.UTC().Format(time.RFC3339Nano), g.Kind, g.Details)
	return e
}

func (s *Store) Observations(ctx context.Context, from, to time.Time) ([]model.Observation, error) {
	rows, e := s.DB.QueryContext(ctx, `SELECT id,ts,target,probe_type,success,latency_ms,error_category,error_message,destination,source,interface,route FROM observations WHERE ts>=? AND ts<=? ORDER BY ts,target`, from.UTC().Format(time.RFC3339Nano), to.UTC().Format(time.RFC3339Nano))
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	var out []model.Observation
	for rows.Next() {
		var o model.Observation
		var ts string
		var latency sql.NullFloat64
		if e = rows.Scan(&o.ID, &ts, &o.Target, &o.ProbeType, &o.Success, &latency, &o.ErrorCategory, &o.ErrorMessage, &o.Destination, &o.Source, &o.Interface, &o.Route); e != nil {
			return nil, e
		}
		o.Timestamp, _ = time.Parse(time.RFC3339Nano, ts)
		if latency.Valid {
			o.LatencyMS = &latency.Float64
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

func (s *Store) Markers(ctx context.Context, from, to time.Time) ([]model.Marker, error) {
	rows, e := s.DB.QueryContext(ctx, "SELECT id,ts,message FROM markers WHERE ts>=? AND ts<=? ORDER BY ts", from.UTC().Format(time.RFC3339Nano), to.UTC().Format(time.RFC3339Nano))
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	var out []model.Marker
	for rows.Next() {
		var m model.Marker
		var ts string
		if e = rows.Scan(&m.ID, &ts, &m.Message); e != nil {
			return nil, e
		}
		m.Timestamp, _ = time.Parse(time.RFC3339Nano, ts)
		out = append(out, m)
	}
	return out, rows.Err()
}
func (s *Store) Gaps(ctx context.Context, from, to time.Time) ([]model.Gap, error) {
	rows, e := s.DB.QueryContext(ctx, "SELECT id,start_ts,end_ts,kind,details FROM gaps WHERE end_ts>=? AND start_ts<=? ORDER BY start_ts", from.UTC().Format(time.RFC3339Nano), to.UTC().Format(time.RFC3339Nano))
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	var out []model.Gap
	for rows.Next() {
		var g model.Gap
		var a, b string
		if e = rows.Scan(&g.ID, &a, &b, &g.Kind, &g.Details); e != nil {
			return nil, e
		}
		g.Start, _ = time.Parse(time.RFC3339Nano, a)
		g.End, _ = time.Parse(time.RFC3339Nano, b)
		out = append(out, g)
	}
	return out, rows.Err()
}

func (s *Store) Rollups(ctx context.Context, from, to time.Time) ([]model.Rollup, error) {
	rows, e := s.DB.QueryContext(ctx, `SELECT bucket_ts,target,probe_type,count,successful_count,packet_loss,min_ms,median_ms,max_ms,mean_ms,jitter_ms,p90_ms,p95_ms,p99_ms FROM rollups_minute WHERE bucket_ts>=? AND bucket_ts<=? ORDER BY bucket_ts,target`, from.UTC().Format(time.RFC3339), to.UTC().Format(time.RFC3339))
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	var out []model.Rollup
	for rows.Next() {
		var r model.Rollup
		var ts string
		var vals [8]sql.NullFloat64
		if e = rows.Scan(&ts, &r.Target, &r.ProbeType, &r.Count, &r.SuccessfulCount, &r.PacketLoss, &vals[0], &vals[1], &vals[2], &vals[3], &vals[4], &vals[5], &vals[6], &vals[7]); e != nil {
			return nil, e
		}
		r.Bucket, _ = time.Parse(time.RFC3339, ts)
		ptr := func(v sql.NullFloat64) *float64 {
			if !v.Valid {
				return nil
			}
			x := v.Float64
			return &x
		}
		r.MinMS = ptr(vals[0])
		r.MedianMS = ptr(vals[1])
		r.MaxMS = ptr(vals[2])
		r.MeanMS = ptr(vals[3])
		r.JitterMS = ptr(vals[4])
		r.P90MS = ptr(vals[5])
		r.P95MS = ptr(vals[6])
		r.P99MS = ptr(vals[7])
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) SaveIncidents(ctx context.Context, incidents []model.Incident) error {
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, "DELETE FROM incidents"); e != nil {
		tx.Rollback()
		return e
	}
	for _, in := range incidents {
		a, _ := json.Marshal(in.AffectedTargets)
		p, _ := json.Marshal(in.ProbeTypes)
		mk, _ := json.Marshal(in.Markers)
		var end any
		if in.End != nil {
			end = in.End.UTC().Format(time.RFC3339Nano)
		}
		if _, e = tx.ExecContext(ctx, `INSERT INTO incidents(start_ts,end_ts,scope,explanation,affected_targets,probe_types,packet_loss,latency_change_ms,markers) VALUES(?,?,?,?,?,?,?,?,?)`, in.Start.UTC().Format(time.RFC3339Nano), end, in.Scope, in.Explanation, string(a), string(p), in.PacketLoss, in.LatencyChangeMS, string(mk)); e != nil {
			tx.Rollback()
			return e
		}
	}
	return tx.Commit()
}

func (s *Store) ReplaceIncidentsFrom(ctx context.Context, from time.Time, incidents []model.Incident) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	keep := make([]string, 0, len(incidents))
	for _, in := range incidents {
		start := in.Start.UTC().Format(time.RFC3339Nano)
		keep = append(keep, start)
		a, _ := json.Marshal(in.AffectedTargets)
		p, _ := json.Marshal(in.ProbeTypes)
		mk, _ := json.Marshal(in.Markers)
		var end any
		if in.End != nil {
			end = in.End.UTC().Format(time.RFC3339Nano)
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO incidents(start_ts,end_ts,scope,explanation,affected_targets,probe_types,packet_loss,latency_change_ms,markers) VALUES(?,?,?,?,?,?,?,?,?) ON CONFLICT(start_ts) DO UPDATE SET end_ts=excluded.end_ts,scope=excluded.scope,explanation=excluded.explanation,affected_targets=excluded.affected_targets,probe_types=excluded.probe_types,packet_loss=excluded.packet_loss,latency_change_ms=excluded.latency_change_ms,markers=excluded.markers`, start, end, in.Scope, in.Explanation, string(a), string(p), in.PacketLoss, in.LatencyChangeMS, string(mk))
		if err != nil {
			tx.Rollback()
			return err
		}
	}
	query := "DELETE FROM incidents WHERE start_ts >= ?"
	args := []any{from.UTC().Format(time.RFC3339Nano)}
	if len(keep) > 0 {
		query += " AND start_ts NOT IN (" + strings.TrimRight(strings.Repeat("?,", len(keep)), ",") + ")"
		for _, v := range keep {
			args = append(args, v)
		}
	}
	if _, err = tx.ExecContext(ctx, query, args...); err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit()
}

func (s *Store) Incidents(ctx context.Context, from, to time.Time) ([]model.Incident, error) {
	rows, e := s.DB.QueryContext(ctx, "SELECT id,start_ts,end_ts,scope,explanation,affected_targets,probe_types,packet_loss,latency_change_ms,markers FROM incidents WHERE start_ts<=? AND COALESCE(end_ts,start_ts)>=? ORDER BY start_ts", to.UTC().Format(time.RFC3339Nano), from.UTC().Format(time.RFC3339Nano))
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	var out []model.Incident
	for rows.Next() {
		var in model.Incident
		var a, b, st, mk string
		var et sql.NullString
		if e = rows.Scan(&in.ID, &st, &et, &in.Scope, &in.Explanation, &a, &b, &in.PacketLoss, &in.LatencyChangeMS, &mk); e != nil {
			return nil, e
		}
		in.Start, _ = time.Parse(time.RFC3339Nano, st)
		if et.Valid {
			x, _ := time.Parse(time.RFC3339Nano, et.String)
			in.End = &x
		}
		_ = json.Unmarshal([]byte(a), &in.AffectedTargets)
		_ = json.Unmarshal([]byte(b), &in.ProbeTypes)
		_ = json.Unmarshal([]byte(mk), &in.Markers)
		out = append(out, in)
	}
	return out, rows.Err()
}

func (s *Store) LastObservationTime(ctx context.Context) (time.Time, error) {
	var v sql.NullString
	e := s.DB.QueryRowContext(ctx, "SELECT max(ts) FROM observations").Scan(&v)
	if e != nil || !v.Valid {
		return time.Time{}, e
	}
	return time.Parse(time.RFC3339Nano, v.String)
}
func (s *Store) Counts(ctx context.Context) (obs, inc, gaps, markers int64, err error) {
	for _, x := range []struct {
		q string
		p *int64
	}{{"SELECT count(*) FROM observations", &obs}, {"SELECT count(*) FROM incidents", &inc}, {"SELECT count(*) FROM gaps", &gaps}, {"SELECT count(*) FROM markers", &markers}} {
		if err = s.DB.QueryRowContext(ctx, x.q).Scan(x.p); err != nil {
			return
		}
	}
	return
}

func (s *Store) RollupAndRetain(ctx context.Context, rawRetention, rollupRetention time.Duration) error {
	cut := time.Now().UTC().Add(-rawRetention).Truncate(time.Minute)
	var oldestText sql.NullString
	if e := s.DB.QueryRowContext(ctx, `SELECT min(ts) FROM observations WHERE ts<?`, cut.Format(time.RFC3339Nano)).Scan(&oldestText); e != nil {
		return e
	}
	if !oldestText.Valid {
		_, e := s.DB.ExecContext(ctx, "DELETE FROM rollups_minute WHERE bucket_ts < ?", time.Now().UTC().Add(-rollupRetention).Format(time.RFC3339Nano))
		return e
	}
	oldest, e := time.Parse(time.RFC3339Nano, oldestText.String)
	if e != nil {
		return e
	}
	batchEnd := oldest.UTC().Truncate(time.Minute).Add(5 * time.Minute)
	if batchEnd.After(cut) {
		batchEnd = cut
	}
	rows, e := s.DB.QueryContext(ctx, `SELECT ts,target,probe_type,success,latency_ms FROM observations WHERE ts<? ORDER BY ts`, batchEnd.Format(time.RFC3339Nano))
	if e != nil {
		return e
	}
	defer rows.Close()
	type agg struct {
		bucket, target, probe string
		count, ok             int
		vals                  []float64
	}
	m := map[string]*agg{}
	for rows.Next() {
		var ts, t, p string
		var ok bool
		var v sql.NullFloat64
		if e = rows.Scan(&ts, &t, &p, &ok, &v); e != nil {
			return e
		}
		parsed, parseErr := time.Parse(time.RFC3339Nano, ts)
		if parseErr != nil {
			return parseErr
		}
		bucket := parsed.UTC().Truncate(time.Minute).Format(time.RFC3339)
		k := bucket + "\x00" + t + "\x00" + p
		if m[k] == nil {
			m[k] = &agg{bucket: bucket, target: t, probe: p}
		}
		a := m[k]
		a.count++
		if ok {
			a.ok++
			if v.Valid {
				a.vals = append(a.vals, v.Float64)
			}
		}
	}
	if e = rows.Close(); e != nil {
		return e
	}
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	for _, a := range m {
		ordered := append([]float64(nil), a.vals...)
		sort.Float64s(a.vals)
		var min, med, max, mean, jitter, p90, p95, p99 any
		if len(a.vals) > 0 {
			mn, md, mx, av, _ := stats(a.vals)
			_, _, _, _, jt := stats(ordered)
			min, med, max, mean, jitter = mn, md, mx, av, jt
			p90 = percentile(a.vals, .90)
			p95 = percentile(a.vals, .95)
			p99 = percentile(a.vals, .99)
		}
		loss := 1.0 - float64(a.ok)/float64(a.count)
		_, e = tx.ExecContext(ctx, `INSERT OR REPLACE INTO rollups_minute VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, a.bucket, a.target, a.probe, a.count, a.ok, loss, min, med, max, mean, jitter, p90, p95, p99)
		if e != nil {
			tx.Rollback()
			return e
		}
	}
	if _, e = tx.ExecContext(ctx, "DELETE FROM observations WHERE ts < ?", batchEnd.Format(time.RFC3339Nano)); e == nil {
		_, e = tx.ExecContext(ctx, "DELETE FROM rollups_minute WHERE bucket_ts < ?", time.Now().UTC().Add(-rollupRetention).Format(time.RFC3339Nano))
	}
	if e != nil {
		tx.Rollback()
		return e
	}
	return tx.Commit()
}
func percentile(v []float64, p float64) float64 {
	if len(v) == 0 {
		return 0
	}
	return v[int(math.Round(p*float64(len(v)-1)))]
}
func stats(v []float64) (float64, float64, float64, float64, float64) {
	var sum, diff float64
	for i, x := range v {
		sum += x
		if i > 0 {
			diff += math.Abs(x - v[i-1])
		}
	}
	return v[0], percentile(v, .5), v[len(v)-1], sum / float64(len(v)), diff / math.Max(1, float64(len(v)-1))
}

func EscapeLike(s string) string { return strings.NewReplacer("%", "\\%", "_", "\\_").Replace(s) }
