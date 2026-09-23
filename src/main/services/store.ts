import { existsSync } from 'node:fs'
import { mkdir, readFile, rename, writeFile } from 'node:fs/promises'
import { createRequire } from 'node:module'
import { dirname, join } from 'node:path'
import initSqlJs, { type Database } from 'sql.js'
import type { DanmakuFilter, DanmakuRecord, LogEntry, Rule } from '@shared/types'

type SqlRow = Record<string, unknown>
const localRequire = createRequire(import.meta.url)

export class SqliteStore {
  private db!: Database

  async init(filePath: string): Promise<void> {
    await mkdir(dirname(filePath), { recursive: true })
    const wasmPath = this.resolveWasmPath()
    const SQL = await initSqlJs({ locateFile: () => wasmPath })
    if (existsSync(filePath)) {
      const bytes = await readFile(filePath)
      this.db = new SQL.Database(new Uint8Array(bytes))
    } else {
      this.db = new SQL.Database()
    }
    this.db.run('PRAGMA journal_mode = WAL;')
    this.db.run(`
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
    await this.persist(filePath)
  }

  async listRules(): Promise<Rule[]> {
    return this.all('SELECT json FROM rules ORDER BY json_extract(json, \'$.priority\') DESC, updated_at ASC')
      .map((row) => JSON.parse(String(row.json)) as Rule)
  }

  async saveRule(rule: Rule): Promise<Rule> {
    const updated = { ...rule, updatedAt: Date.now(), createdAt: rule.createdAt ?? Date.now() }
    this.run('INSERT OR REPLACE INTO rules(id, json, updated_at) VALUES (?, ?, ?)', [updated.id, JSON.stringify(updated), updated.updatedAt])
    return updated
  }

  async removeRule(id: string): Promise<void> {
    this.run('DELETE FROM rules WHERE id = ?', [id])
  }

  async clearRules(): Promise<void> {
    this.db.run('DELETE FROM rules')
  }

  insertRecord(record: Omit<DanmakuRecord, 'id'>): number {
    this.run(
      `INSERT INTO danmaku_records
        (source, room_id, kind, user_id, user_name, text, gift_name, gift_count, gift_value, repeat_count, ts, created_at, matched_rule_id, matched_rule_name, action_result, fail_reason, raw_json)
       VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
      [
        record.source,
        record.roomId ?? null,
        record.kind,
        record.userId ?? null,
        record.userName ?? null,
        record.text ?? null,
        record.giftName ?? null,
        record.giftCount ?? null,
        record.giftValue ?? null,
        record.repeatCount ?? null,
        record.ts,
        record.createdAt,
        record.matchedRuleId ?? null,
        record.matchedRuleName ?? null,
        record.actionResult,
        record.failReason ?? null,
        record.rawJson ?? null,
      ],
    )
    const row = this.all('SELECT last_insert_rowid() AS id')[0]
    return Number(row?.id ?? 0)
  }

  updateRecordResult(id: number, result: Pick<DanmakuRecord, 'matchedRuleId' | 'matchedRuleName' | 'actionResult' | 'failReason'>): void {
    this.run('UPDATE danmaku_records SET matched_rule_id = ?, matched_rule_name = ?, action_result = ?, fail_reason = ? WHERE id = ?', [
      result.matchedRuleId ?? null,
      result.matchedRuleName ?? null,
      result.actionResult,
      result.failReason ?? null,
      id,
    ])
  }

  queryRecords(filter: DanmakuFilter = {}): DanmakuRecord[] {
    const { where, params } = buildRecordWhere(filter)
    const limit = Math.min(Math.max(filter.limit ?? 100, 1), 500)
    const offset = Math.max(filter.offset ?? 0, 0)
    return this.all(
      `SELECT id, source, room_id AS roomId, kind, user_id AS userId, user_name AS userName,
        text, gift_name AS giftName, gift_count AS giftCount, gift_value AS giftValue,
        repeat_count AS repeatCount, ts, created_at AS createdAt, matched_rule_id AS matchedRuleId,
        matched_rule_name AS matchedRuleName, action_result AS actionResult, fail_reason AS failReason,
        raw_json AS rawJson FROM danmaku_records ${where} ORDER BY ts DESC, id DESC LIMIT ? OFFSET ?`,
      [...params, limit, offset],
    ).map((row) => normalizeRecord(row))
  }

  countRecords(filter: DanmakuFilter = {}): number {
    const { where, params } = buildRecordWhere(filter)
    const row = this.all(`SELECT COUNT(*) AS count FROM danmaku_records ${where}`, params)[0]
    return Number(row?.count ?? 0)
  }

  clearRecords(range: { from?: number; to?: number } = {}): number {
    const conditions: string[] = []
    const params: unknown[] = []
    if (range.from !== undefined) { conditions.push('ts >= ?'); params.push(range.from) }
    if (range.to !== undefined) { conditions.push('ts <= ?'); params.push(range.to) }
    const before = this.countRecords()
    this.run(`DELETE FROM danmaku_records${conditions.length ? ` WHERE ${conditions.join(' AND ')}` : ''}`, params)
    return before - this.countRecords()
  }

  cleanupRetention(days: number, maxRows: number): number {
    const cutoff = Date.now() - Math.max(days, 1) * 24 * 60 * 60 * 1000
    const before = this.countRecords()
    this.run('DELETE FROM danmaku_records WHERE ts < ?', [cutoff])
    const excess = Math.max(0, this.countRecords() - Math.max(maxRows, 1))
    if (excess) this.run('DELETE FROM danmaku_records WHERE id IN (SELECT id FROM danmaku_records ORDER BY ts ASC, id ASC LIMIT ?)', [excess])
    return before - this.countRecords()
  }

  saveLog(entry: LogEntry): void {
    this.run('INSERT OR REPLACE INTO logs(id, level, category, message, detail, ts) VALUES (?, ?, ?, ?, ?, ?)', [entry.id, entry.level, entry.category, entry.message, entry.detail ?? null, entry.ts])
  }

  listLogs(limit = 100): LogEntry[] {
    return this.all('SELECT id, level, category, message, detail, ts FROM logs ORDER BY id DESC LIMIT ?', [Math.min(Math.max(limit, 1), 500)]) as unknown as LogEntry[]
  }

  getSetting<T>(key: string, fallback: T): T {
    const row = this.all('SELECT value FROM settings WHERE key = ?', [key])[0]
    if (!row) return fallback
    try { return JSON.parse(String(row.value)) as T } catch { return fallback }
  }

  setSetting<T>(key: string, value: T): void {
    this.run('INSERT OR REPLACE INTO settings(key, value) VALUES (?, ?)', [key, JSON.stringify(value)])
  }

  async persist(filePath: string): Promise<void> {
    if (!this.db) return
    const bytes = this.db.export()
    const tempPath = `${filePath}.tmp`
    await writeFile(tempPath, bytes)
    await rename(tempPath, filePath)
  }

  private all(sql: string, params: unknown[] = []): SqlRow[] {
    const statement = this.db.prepare(sql)
    statement.bind(params as any)
    const rows: SqlRow[] = []
    while (statement.step()) rows.push(statement.getAsObject() as SqlRow)
    statement.free()
    return rows
  }

  private run(sql: string, params: unknown[] = []): void {
    const statement = this.db.prepare(sql)
    statement.run(params as any)
    statement.free()
  }

  private resolveWasmPath(): string {
    try {
      const resolved = localRequire.resolve('sql.js/dist/sql-wasm.wasm')
      return resolved
    } catch {
      return join(process.resourcesPath, 'sql.js', 'sql-wasm.wasm')
    }
  }
}

function buildRecordWhere(filter: DanmakuFilter): { where: string; params: unknown[] } {
  const clauses: string[] = []
  const params: unknown[] = []
  if (filter.from !== undefined) { clauses.push('ts >= ?'); params.push(filter.from) }
  if (filter.to !== undefined) { clauses.push('ts <= ?'); params.push(filter.to) }
  if (filter.source) { clauses.push('source = ?'); params.push(filter.source) }
  if (filter.kind) { clauses.push('kind = ?'); params.push(filter.kind) }
  if (filter.userName) { clauses.push('user_name LIKE ?'); params.push(`%${filter.userName}%`) }
  if (filter.keyword) { clauses.push('(text LIKE ? OR gift_name LIKE ?)'); params.push(`%${filter.keyword}%`, `%${filter.keyword}%`) }
  if (filter.actionResult) { clauses.push('action_result = ?'); params.push(filter.actionResult) }
  if (filter.matched === 'yes') clauses.push('matched_rule_id IS NOT NULL')
  if (filter.matched === 'no') clauses.push('matched_rule_id IS NULL')
  return { where: clauses.length ? `WHERE ${clauses.join(' AND ')}` : '', params }
}

function normalizeRecord(row: SqlRow): DanmakuRecord {
  return {
    id: Number(row.id), source: String(row.source), roomId: nullableString(row.roomId), kind: row.kind as DanmakuRecord['kind'],
    userId: nullableString(row.userId), userName: nullableString(row.userName), text: nullableString(row.text),
    giftName: nullableString(row.giftName), giftCount: nullableNumber(row.giftCount), giftValue: nullableNumber(row.giftValue),
    repeatCount: nullableNumber(row.repeatCount), ts: Number(row.ts), createdAt: Number(row.createdAt),
    matchedRuleId: nullableString(row.matchedRuleId), matchedRuleName: nullableString(row.matchedRuleName),
    actionResult: (row.actionResult as DanmakuRecord['actionResult']) ?? 'none', failReason: nullableString(row.failReason), rawJson: nullableString(row.rawJson),
  }
}

function nullableString(value: unknown): string | undefined { return value === null || value === undefined ? undefined : String(value) }
function nullableNumber(value: unknown): number | undefined { return value === null || value === undefined ? undefined : Number(value) }
