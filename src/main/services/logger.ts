import { appendFile, mkdir } from 'node:fs/promises'
import { join } from 'node:path'
import type { LogEntry } from '@shared/types'

type LogListener = (entry: LogEntry) => void

export class AppLogger {
  private readonly listeners = new Set<LogListener>()
  private nextId = Date.now() * 1000

  constructor(private readonly logDir: string) {}

  on(listener: LogListener): () => void {
    this.listeners.add(listener)
    return () => this.listeners.delete(listener)
  }

  write(level: LogEntry['level'], category: string, message: string, detail?: string): LogEntry {
    const entry: LogEntry = {
      id: this.nextId++,
      level,
      category,
      message: redact(message),
      detail: detail ? redact(detail) : undefined,
      ts: Date.now(),
    }
    for (const listener of this.listeners) listener(entry)
    void this.persist(entry)
    return entry
  }

  info(message: string, detail?: string): LogEntry { return this.write('info', 'app', message, detail) }
  warn(message: string, detail?: string): LogEntry { return this.write('warn', 'app', message, detail) }
  error(message: string, detail?: string): LogEntry { return this.write('error', 'app', message, detail) }
  debug(message: string, detail?: string): LogEntry { return this.write('debug', 'app', message, detail) }

  private async persist(entry: LogEntry): Promise<void> {
    try {
      await mkdir(this.logDir, { recursive: true })
      await appendFile(join(this.logDir, 'app.log'), `${JSON.stringify(entry)}\n`, 'utf8')
    } catch {
      // A logging failure must never interrupt event processing.
    }
  }
}

export function redact(value: string): string {
  return value
    .replace(/(cookie|authorization|token|password|kami|card|卡密)\s*[:=]\s*[^\s,;]+/gi, '$1=***')
    .replace(/Bearer\s+[A-Za-z0-9._~-]+/gi, 'Bearer ***')
}
