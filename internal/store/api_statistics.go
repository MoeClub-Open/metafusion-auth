package store

import (
	"context"
	_ "embed"
	"time"
)

//go:embed audit_cleanup.sql
var auditCleanupSchema string

type APIStatistic struct {
	Day      string `json:"day"`
	Service  string `json:"service"`
	Method   string `json:"method"`
	Route    string `json:"route"`
	Status   int    `json:"status"`
	Requests int64  `json:"requests"`
	TotalMS  int64  `json:"total_ms"`
	MaxMS    int64  `json:"max_ms"`
}

// The date window is bounded by the HTTP handler; rows are daily aggregates.
func (s *Store) APIStatistics(ctx context.Context, from, to time.Time, service string) ([]APIStatistic, error) {
	items := []APIStatistic{}
	rows, err := s.DB.QueryContext(ctx, `SELECT day::text,service,method,route,status,requests,total_ms,max_ms
	 FROM audit.api_statistics WHERE day >= $1 AND day <= $2 AND ($3='' OR service=$3)
	 ORDER BY day DESC,service,requests DESC,method,route,status`, from, to, service)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var item APIStatistic
		if err := rows.Scan(&item.Day, &item.Service, &item.Method, &item.Route, &item.Status, &item.Requests, &item.TotalMS, &item.MaxMS); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) PreviewLogCleanup(ctx context.Context, kind string, before time.Time, service string) (int64, error) {
	query := `SELECT count(*) FROM audit.audit_log WHERE occurred_at < $1 AND ($2='' OR service=$2)`
	if kind == "statistics" {
		query = `SELECT count(*) FROM audit.api_statistics WHERE day < ($1::timestamptz AT TIME ZONE 'UTC')::date AND ($2='' OR service=$2)`
	}
	var count int64
	err := s.DB.QueryRowContext(ctx, query, before, service).Scan(&count)
	return count, err
}

func (s *Store) CleanupLogs(ctx context.Context, kind string, before time.Time, service string) (int64, error) {
	var removed int64
	err := s.DB.QueryRowContext(ctx, `SELECT audit.cleanup_logs($1,$2,$3)`, kind, before, service).Scan(&removed)
	return removed, err
}
