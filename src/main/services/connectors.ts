import { randomUUID } from 'node:crypto'
import type { ConnectorStatus, LiveEvent, Platform } from '@shared/types'
import { delay } from '@main/core/rule-engine'

type EventListener = (event: LiveEvent) => void
type StatusListener = (status: ConnectorStatus) => void

export class ConnectorManager {
  private statusValue: ConnectorStatus = {
    platform: 'simulator', state: 'disconnected', mode: 'simulator', reconnects: 0, dropped: 0,
  }
  private readonly eventListeners = new Set<EventListener>()
  private readonly statusListeners = new Set<StatusListener>()
  private readonly dedupe = new Map<string, number>()
  private readonly buffered = new Map<string, LiveEvent[]>()

  onEvent(listener: EventListener): () => void { this.eventListeners.add(listener); return () => this.eventListeners.delete(listener) }
  onStatus(listener: StatusListener): () => void { this.statusListeners.add(listener); return () => this.statusListeners.delete(listener) }

  async connect(platform: Platform, roomId: string): Promise<ConnectorStatus> {
    this.setStatus({ platform, roomId, state: 'connecting', mode: 'simulator', reconnects: this.statusValue.reconnects, dropped: 0 })
    await delay(120)
    this.setStatus({ platform, roomId, state: 'connected', mode: 'simulator', reconnects: 0, dropped: 0, lastEventAt: undefined })
    return this.statusValue
  }

  async disconnect(): Promise<ConnectorStatus> {
    this.setStatus({ ...this.statusValue, state: 'disconnected' })
    return this.statusValue
  }

  getStatus(): ConnectorStatus { return { ...this.statusValue } }

  async publish(input: Partial<LiveEvent> & Pick<LiveEvent, 'kind'>): Promise<LiveEvent> {
    const event: LiveEvent = {
      id: input.id ?? randomUUID(),
      source: input.source ?? this.statusValue.platform,
      roomId: input.roomId ?? this.statusValue.roomId,
      kind: input.kind,
      user: input.user ?? { id: 'sim-user', name: '模拟观众' },
      gift: input.gift,
      text: input.text,
      count: input.count,
      timestamp: input.timestamp ?? Date.now(),
      raw: input.raw,
    }
    const key = dedupeKey(event)
    const lastSeen = this.dedupe.get(key) ?? 0
    if (event.timestamp - lastSeen < 3000) return event
    this.dedupe.set(key, event.timestamp)
    this.pruneDedupe(event.timestamp)
    const platform = event.source
    const queue = this.buffered.get(platform) ?? []
    if (queue.length >= 1000) {
      if (event.kind === 'gift') {
        const index = queue.findIndex((item) => item.kind !== 'gift')
        if (index >= 0) queue.splice(index, 1)
        else { this.statusValue.dropped += 1; return event }
      } else {
        this.statusValue.dropped += 1
        return event
      }
    }
    queue.push(event)
    this.buffered.set(platform, queue)
    const next = queue.shift()
    if (next) {
      this.statusValue.lastEventAt = next.timestamp
      for (const listener of this.eventListeners) listener(next)
    }
    this.emitStatus()
    return event
  }

  private setStatus(status: ConnectorStatus): void { this.statusValue = { ...status }; this.emitStatus() }
  private emitStatus(): void { for (const listener of this.statusListeners) listener(this.getStatus()) }
  private pruneDedupe(now: number): void { for (const [key, ts] of this.dedupe) if (now - ts > 30_000) this.dedupe.delete(key) }
}

function dedupeKey(event: LiveEvent): string {
  return [event.source, event.user?.id ?? event.user?.name ?? '', event.kind, event.gift?.name ?? '', event.text ?? '', Math.floor(event.timestamp / 1000)].join('|')
}
