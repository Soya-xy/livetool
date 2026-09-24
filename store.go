package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

func OpenStore(file string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
		return nil, fmt.Errorf("create data directory: %w", err)
	}
	db, err := sql.Open("sqlite", file)
	if err != nil {
		return nil, fmt.Errorf("open sqlite database: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetConnMaxLifetime(0)
	store := &Store{db: db}
	if _, err := db.Exec(`PRAGMA busy_timeout=5000; PRAGMA journal_mode=WAL; PRAGMA foreign_keys=ON;`); err != nil {
		db.Close()
		return nil, fmt.Errorf("configure sqlite database: %w", err)
	}
	if err := store.initSchema(context.Background()); err != nil {
		db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) initSchema(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS rules (
			id TEXT PRIMARY KEY,
			json TEXT NOT NULL,
			updated_at INTEGER NOT NULL
		);
		CREATE TABLE IF NOT EXISTS danmaku_records (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			source TEXT NOT NULL,
			room_id TEXT,
			kind TEXT NOT NULL,
			user_id TEXT,
			user_name TEXT,
			text TEXT,
			gift_name TEXT,
			gift_count INTEGER,
			gift_value REAL,
			repeat_count INTEGER,
			ts INTEGER NOT NULL,
			created_at INTEGER NOT NULL,
			matched_rule_id TEXT,
			matched_rule_name TEXT,
			action_result TEXT NOT NULL DEFAULT 'none',
			fail_reason TEXT,
			raw_json TEXT
		);
		CREATE INDEX IF NOT EXISTS idx_danmaku_ts ON danmaku_records(ts);
		CREATE INDEX IF NOT EXISTS idx_danmaku_source_ts ON danmaku_records(source, ts);
		CREATE INDEX IF NOT EXISTS idx_danmaku_kind_ts ON danmaku_records(kind, ts);
		CREATE INDEX IF NOT EXISTS idx_danmaku_user ON danmaku_records(user_name);
		CREATE TABLE IF NOT EXISTS logs (
			id INTEGER PRIMARY KEY,
			level TEXT NOT NULL,
			category TEXT NOT NULL,
			message TEXT NOT NULL,
			detail TEXT,
			ts INTEGER NOT NULL
		);
		CREATE TABLE IF NOT EXISTS settings (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		);
	`)
	if err != nil {
		return fmt.Errorf("initialize sqlite schema: %w", err)
	}
	return nil
}

func (s *Store) ListRules(ctx context.Context) ([]Rule, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT json FROM rules ORDER BY json_extract(json, '$.priority') DESC, updated_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]Rule, 0)
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var rule Rule
		if err := json.Unmarshal([]byte(raw), &rule); err != nil {
			continue
		}
		result = append(result, rule)
	}
	return result, rows.Err()
}

func (s *Store) SaveRule(ctx context.Context, rule Rule) (Rule, error) {
	now := nowMillis()
	if rule.CreatedAt == 0 {
		rule.CreatedAt = now
	}
	rule.UpdatedAt = now
	data, err := json.Marshal(rule)
	if err != nil {
		return Rule{}, err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO rules(id,json,updated_at) VALUES(?,?,?) ON CONFLICT(id) DO UPDATE SET json=excluded.json, updated_at=excluded.updated_at`, rule.ID, string(data), now)
	return rule, err
}

func (s *Store) RemoveRule(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM rules WHERE id=?`, id)
	return err
}

func (s *Store) ClearRules(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM rules`)
	return err
}

func (s *Store) InsertRecord(ctx context.Context, record DanmakuRecord) (int64, error) {
	result, err := s.db.ExecContext(ctx, `INSERT INTO danmaku_records
		(source,room_id,kind,user_id,user_name,text,gift_name,gift_count,gift_value,repeat_count,ts,created_at,matched_rule_id,matched_rule_name,action_result,fail_reason,raw_json)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		record.Source, nullable(record.RoomID), string(record.Kind), nullable(record.UserID), nullable(record.UserName), nullable(record.Text),
		nullable(record.GiftName), nullableInt(record.GiftCount), nullableFloat(record.GiftValue), nullableInt(record.RepeatCount), record.TS, record.CreatedAt,
		nullable(record.MatchedRuleID), nullable(record.MatchedRuleName), record.ActionResult, nullable(record.FailReason), nullable(record.RawJSON))
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (s *Store) UpdateRecordResult(ctx context.Context, id int64, outcome RuleOutcome) error {
	_, err := s.db.ExecContext(ctx, `UPDATE danmaku_records SET matched_rule_id=?,matched_rule_name=?,action_result=?,fail_reason=? WHERE id=?`,
		nullable(outcome.RuleID), nullable(outcome.RuleName), outcome.Result, nullable(outcome.Reason), id)
	return err
}

func (s *Store) QueryRecords(ctx context.Context, filter DanmakuFilter) ([]DanmakuRecord, error) {
	where, args := recordWhere(filter)
	limit := filter.Limit
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	offset := max(filter.Offset, 0)
	args = append(args, limit, offset)
	rows, err := s.db.QueryContext(ctx, `SELECT id,source,room_id,kind,user_id,user_name,text,gift_name,gift_count,gift_value,repeat_count,ts,created_at,matched_rule_id,matched_rule_name,action_result,fail_reason,raw_json FROM danmaku_records `+where+` ORDER BY ts DESC,id DESC LIMIT ? OFFSET ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]DanmakuRecord, 0)
	for rows.Next() {
		record, err := scanRecord(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, record)
	}
	return result, rows.Err()
}

func (s *Store) CountRecords(ctx context.Context, filter DanmakuFilter) (int, error) {
	where, args := recordWhere(filter)
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM danmaku_records `+where, args...).Scan(&count)
	return count, err
}

func (s *Store) ClearRecords(ctx context.Context, from, to int64) (int64, error) {
	conditions := make([]string, 0, 2)
	args := make([]any, 0, 2)
	if from != 0 {
		conditions = append(conditions, "ts>=?")
		args = append(args, from)
	}
	if to != 0 {
		conditions = append(conditions, "ts<=?")
		args = append(args, to)
	}
	query := "DELETE FROM danmaku_records"
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	result, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (s *Store) CleanupRetention(ctx context.Context, days, maxRows int) (int64, error) {
	if days < 1 {
		days = 1
	}
	if maxRows < 1 {
		maxRows = 1
	}
	before, err := s.CountRecords(ctx, DanmakuFilter{})
	if err != nil {
		return 0, err
	}
	cutoff := time.Now().Add(-time.Duration(days) * 24 * time.Hour).UnixMilli()
	if _, err := s.db.ExecContext(ctx, `DELETE FROM danmaku_records WHERE ts<?`, cutoff); err != nil {
		return 0, err
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM danmaku_records WHERE id IN (SELECT id FROM danmaku_records ORDER BY ts ASC,id ASC LIMIT MAX((SELECT COUNT(*) FROM danmaku_records)-?,0))`, maxRows); err != nil {
		return 0, err
	}
	after, err := s.CountRecords(ctx, DanmakuFilter{})
	return int64(before - after), err
}

func (s *Store) SaveLog(ctx context.Context, entry LogEntry) (int64, error) {
	result, err := s.db.ExecContext(ctx, `INSERT INTO logs(level,category,message,detail,ts) VALUES(?,?,?,?,?)`, entry.Level, entry.Category, entry.Message, nullable(entry.Detail), entry.TS)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (s *Store) ListLogs(ctx context.Context, limit int) ([]LogEntry, error) {
	if limit < 1 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id,level,category,message,detail,ts FROM logs ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]LogEntry, 0)
	for rows.Next() {
		var item LogEntry
		var detail sql.NullString
		if err := rows.Scan(&item.ID, &item.Level, &item.Category, &item.Message, &detail, &item.TS); err != nil {
			return nil, err
		}
		item.Detail = detail.String
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *Store) GetSetting(ctx context.Context, key string, fallback any, target any) error {
	var raw string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key=?`, key).Scan(&raw)
	if err == sql.ErrNoRows {
		return json.Unmarshal(mustJSON(fallback), target)
	}
	if err != nil {
		return err
	}
	if err := json.Unmarshal([]byte(raw), target); err != nil {
		return json.Unmarshal(mustJSON(fallback), target)
	}
	return nil
}

func (s *Store) SetSetting(ctx context.Context, key string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, string(data))
	return err
}

func recordWhere(filter DanmakuFilter) (string, []any) {
	conditions := make([]string, 0, 9)
	args := make([]any, 0, 9)
	if filter.From != 0 {
		conditions = append(conditions, "ts>=?")
		args = append(args, filter.From)
	}
	if filter.To != 0 {
		conditions = append(conditions, "ts<=?")
		args = append(args, filter.To)
	}
	if filter.Source != "" {
		conditions = append(conditions, "source=?")
		args = append(args, filter.Source)
	}
	if filter.Kind != "" {
		conditions = append(conditions, "kind=?")
		args = append(args, filter.Kind)
	}
	if filter.UserName != "" {
		conditions = append(conditions, "user_name LIKE ?")
		args = append(args, "%"+filter.UserName+"%")
	}
	if filter.Keyword != "" {
		conditions = append(conditions, "(text LIKE ? OR gift_name LIKE ?)")
		args = append(args, "%"+filter.Keyword+"%", "%"+filter.Keyword+"%")
	}
	if filter.ActionResult != "" {
		conditions = append(conditions, "action_result=?")
		args = append(args, filter.ActionResult)
	}
	if filter.Matched == "yes" {
		conditions = append(conditions, "matched_rule_id IS NOT NULL")
	} else if filter.Matched == "no" {
		conditions = append(conditions, "matched_rule_id IS NULL")
	}
	if len(conditions) == 0 {
		return "", args
	}
	return "WHERE " + strings.Join(conditions, " AND "), args
}

type rowScanner interface{ Scan(dest ...any) error }

func scanRecord(row rowScanner) (DanmakuRecord, error) {
	var record DanmakuRecord
	var roomID, userID, userName, text, giftName, matchedID, matchedName, failReason, rawJSON sql.NullString
	var giftCount, repeatCount sql.NullInt64
	var giftValue sql.NullFloat64
	err := row.Scan(&record.ID, &record.Source, &roomID, &record.Kind, &userID, &userName, &text, &giftName, &giftCount, &giftValue, &repeatCount, &record.TS, &record.CreatedAt, &matchedID, &matchedName, &record.ActionResult, &failReason, &rawJSON)
	record.RoomID, record.UserID, record.UserName = roomID.String, userID.String, userName.String
	record.Text, record.GiftName, record.MatchedRuleID, record.MatchedRuleName = text.String, giftName.String, matchedID.String, matchedName.String
	record.FailReason, record.RawJSON = failReason.String, rawJSON.String
	if giftCount.Valid {
		record.GiftCount = int(giftCount.Int64)
	}
	if repeatCount.Valid {
		record.RepeatCount = int(repeatCount.Int64)
	}
	if giftValue.Valid {
		record.GiftValue = giftValue.Float64
	}
	return record, err
}

func nullable(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func nullableInt(value int) any {
	if value == 0 {
		return nil
	}
	return value
}

func nullableFloat(value float64) any {
	if value == 0 {
		return nil
	}
	return value
}

func mustJSON(value any) []byte {
	data, _ := json.Marshal(value)
	return data
}
