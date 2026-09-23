import type { FeatureId, FeatureSettings, FeatureValue } from './features'

export type Platform =
  | 'douyin'
  | 'kuaishou'
  | 'shipinhao'
  | 'bilibili'
  | 'tiktok'
  | 'douyu'
  | 'xiaohongshu'
  | 'simulator'

export type EventKind = 'gift' | 'chat' | 'like' | 'follow' | 'enter' | 'system'

export interface LiveEvent {
  id: string
  source: Platform | string
  roomId?: string
  kind: EventKind
  user?: { id?: string; name?: string }
  gift?: { id?: string; name: string; icon?: string; count: number; value?: number }
  text?: string
  count?: number
  timestamp: number
  raw?: unknown
}

export type WindowTarget = {
  title?: string
  className?: string
  processName?: string
}

export type KeyStep =
  | { op: 'down' | 'up' | 'tap'; key: string }
  | { op: 'wait'; ms: number }

export type MouseStep =
  | { op: 'move' | 'click' | 'down' | 'up'; button?: 'left' | 'right' | 'middle'; x?: number; y?: number }
  | { op: 'scroll'; delta: number }
  | { op: 'wait'; ms: number }

export interface ChromaConfig {
  enabled: boolean
  color: string
  similarity: number
  smoothness: number
}

export interface ActionBase {
  id: string
  delayMs?: number
  repeat?: number
  durationMs?: number
}

export type Action =
  | (ActionBase & { kind: 'key'; target?: WindowTarget; steps: KeyStep[] })
  | (ActionBase & { kind: 'mouse'; target?: WindowTarget; steps: MouseStep[] })
  | (ActionBase & { kind: 'video'; path: string; lane?: number; loop?: boolean; chroma?: ChromaConfig })
  | (ActionBase & { kind: 'audio'; path: string; volume?: number; interrupt?: boolean; loop?: boolean })
  | (ActionBase & { kind: 'drop'; image: string; count?: number; gravity?: number; bounce?: number; durationMs?: number })
  | (ActionBase & { kind: 'slot'; theme?: string; pool?: string[]; weights?: number[] })
  | (ActionBase & { kind: 'serial'; port: string; baud: number; onBytes: number[]; offBytes: number[]; pulseMs: number })
  | (ActionBase & { kind: 'obs'; command: string; args?: Record<string, string> })

export interface RuleTrigger {
  kinds: EventKind[]
  giftNames?: string[]
  keywords?: string[]
  keywordMode?: 'exact' | 'contains' | 'regex'
  minCount?: number
  users?: string[]
  source?: string[]
}

export interface Rule {
  id: string
  name: string
  enabled: boolean
  priority: number
  trigger: RuleTrigger
  cooldownMs?: number
  probability?: number
  concurrency: 'queue' | 'parallel' | 'replace' | 'exclusive'
  actions: Action[]
  createdAt?: number
  updatedAt?: number
}

export type ActionResult = 'ok' | 'skipped' | 'failed' | 'none'

export interface DanmakuRecord {
  id: number
  source: string
  roomId?: string
  kind: EventKind
  userId?: string
  userName?: string
  text?: string
  giftName?: string
  giftCount?: number
  giftValue?: number
  repeatCount?: number
  ts: number
  createdAt: number
  matchedRuleId?: string
  matchedRuleName?: string
  actionResult: ActionResult
  failReason?: string
  rawJson?: string
}

export interface DanmakuFilter {
  from?: number
  to?: number
  source?: string
  kind?: EventKind | ''
  userName?: string
  keyword?: string
  actionResult?: ActionResult | ''
  matched?: 'yes' | 'no' | ''
  limit?: number
  offset?: number
}

export type ConnectorState = 'disconnected' | 'connecting' | 'connected' | 'reconnecting' | 'error'

export interface ConnectorStatus {
  platform: string
  roomId?: string
  state: ConnectorState
  mode: 'simulator' | 'websocket' | 'polling' | 'relay'
  reconnects: number
  dropped: number
  lastError?: string
  lastEventAt?: number
}

export interface OverlaySettings {
  visible: boolean
  alwaysOnTop: boolean
  opacity: number
  width: number
  height: number
  laneCount: number
  background: string
}

export type OverlayType = 'green' | 'slot'

export type OverlayWidgetKind = 'acceleration' | 'health' | 'timer' | 'woodfish' | 'counter' | 'wheel' | 'sticker' | 'lock' | 'mosquito' | 'reply' | 'notice' | 'speech'

export interface OverlayWidgetPayload {
  featureId: FeatureId
  kind: OverlayWidgetKind
  title: string
  values: Record<string, FeatureValue>
  data?: Record<string, FeatureValue>
}

export type OverlayWindowMode = 'green' | 'landscape-16-9' | 'landscape-4-3' | 'portrait-9-16' | 'fullscreen'

export interface OverlayWindowStatus {
  type: OverlayType
  role: 'ylm' | 'yapp'
  title: string
  visible: boolean
  width: number
  height: number
  mode: OverlayWindowMode
  fullscreen: boolean
  opacity: number
}

export interface AppSettings {
  version: string
  devMode: boolean
  logLevel: 'error' | 'warn' | 'info' | 'debug'
  retentionDays: number
  retentionMaxRows: number
  storeRaw: boolean
  hotkey: string
  audioVolume: number
  assetsRoot: string
  overlayGreen: OverlaySettings
  overlaySlot: OverlaySettings
  platform: Platform
  roomId: string
  authServerUrl: string
  obsUrl: string
  obsPassword: string
  autoStart: boolean
  features: FeatureSettings
}

export type LicenseEntitlement = 'overlay' | 'slot' | 'serial'
export type LicenseCardStatus = 'pending' | 'active' | 'disabled' | 'revoked' | 'expired'

export interface LicenseCardSummary {
  id: string
  suffix: string
  status: LicenseCardStatus
  createdAt: number
  activatedAt?: number
  expiresAt?: number
  durationDays: number
  maxDevices: number
  boundDevices: number
  platforms: Platform[]
  features: LicenseEntitlement[]
  note: string
}

export interface LicenseCardDetail extends LicenseCardSummary {
  bindings: Array<{ fingerprint: string; boundAt: number }>
}

export interface LicenseAuditEntry {
  id: string
  action: string
  cardId?: string
  at: number
  detail?: string
}

export interface LicenseCardCreateInput {
  count: number
  durationDays: number
  maxDevices: number
  platforms: Platform[]
  features: LicenseEntitlement[]
  note?: string
}

export interface LicenseCardQuery {
  query?: string
  status?: LicenseCardStatus | 'pending' | ''
  page?: number
  pageSize?: number
}

export interface LicenseCardPage {
  items: LicenseCardSummary[]
  total: number
  page: number
  pageSize: number
}

export interface LicenseAuthStatus {
  loggedIn: boolean
  mode: 'local' | 'remote'
  platform: string
  expiresAt?: string
  features: LicenseEntitlement[]
  safeCodeSet?: boolean
}

export interface LogEntry {
  id: number
  level: 'error' | 'warn' | 'info' | 'debug'
  category: string
  message: string
  detail?: string
  ts: number
}

export interface EventOutcome {
  ruleId?: string
  ruleName?: string
  result: ActionResult
  reason?: string
}

export interface ExportConfig {
  schema_version: 1
  app_version: string
  rules: Rule[]
  assets: string[]
  overlay: {
    green: OverlaySettings
    slot: OverlaySettings
  }
  slot: { theme: string }
  connectors: { platform: Platform; roomId: string }
  danmaku: {
    retention_days: number
    retention_max_rows: number
    store_raw: boolean
  }
  features: FeatureSettings
}

export interface ElectronApi {
  rules: {
    list: () => Promise<Rule[]>
    save: (rule: Rule) => Promise<Rule>
    remove: (id: string) => Promise<void>
    clear: () => Promise<void>
    clone: (id: string) => Promise<Rule>
  }
  danmaku: {
    query: (filter: DanmakuFilter) => Promise<DanmakuRecord[]>
    count: (filter?: DanmakuFilter) => Promise<number>
    export: (filter: DanmakuFilter, format: 'json' | 'csv' | 'txt') => Promise<{ path?: string; content?: string }>
    clear: (filter?: { from?: number; to?: number }) => Promise<number>
    onAppend: (cb: (record: DanmakuRecord) => void) => () => void
  }
  conn: {
    connect: (platform: Platform, roomId: string) => Promise<ConnectorStatus>
    disconnect: () => Promise<ConnectorStatus>
    status: () => Promise<ConnectorStatus>
    simulate: (event: Partial<LiveEvent> & Pick<LiveEvent, 'kind'>) => Promise<LiveEvent>
    onStatus: (cb: (status: ConnectorStatus) => void) => () => void
  }
  overlay: {
    open: (type: 'green' | 'slot') => Promise<OverlayWindowStatus[]>
    close: (type: 'green' | 'slot') => Promise<OverlayWindowStatus[]>
    status: () => Promise<OverlayWindowStatus[]>
    setMode: (type: OverlayType, mode: OverlayWindowMode) => Promise<OverlayWindowStatus>
    toggleOpacity: (type: OverlayType) => Promise<OverlayWindowStatus>
    updateSettings: (type: 'green' | 'slot', settings: Partial<OverlaySettings>) => Promise<OverlaySettings>
    playVideo: (payload: { path: string; lane?: number; durationMs?: number; loop?: boolean; chroma?: ChromaConfig }) => Promise<void>
    drop: (payload: { image: string; count?: number; gravity?: number; bounce?: number; durationMs?: number; maxVisible?: number }) => Promise<void>
    slot: (payload: { theme?: string; pool?: string[]; weights?: number[]; images?: string[]; durationMs?: number; audioPath?: string }) => Promise<void>
    widget: (payload: OverlayWidgetPayload) => Promise<void>
    removeWidget: (featureId: FeatureId) => Promise<void>
    onMessage: (cb: (message: { target: 'green' | 'slot'; type: string; payload: unknown }) => void) => () => void
    onStatus: (cb: (status: OverlayWindowStatus) => void) => () => void
  }
  audio: {
    play: (payload: { path: string; volume?: number; interrupt?: boolean; loop?: boolean }) => Promise<void>
    stop: () => Promise<void>
    onMessage: (cb: (message: { type: string; payload: { path?: string; volume?: number; interrupt?: boolean; loop?: boolean } }) => void) => () => void
  }
  input: {
    run: (action: Extract<Action, { kind: 'key' | 'mouse' }>) => Promise<{ ok: boolean; message: string }>
    findImage: (image: string, threshold: number) => Promise<{ ok: boolean; message: string; confidence?: number }>
  }
  serial: {
    ports: () => Promise<Array<{ path: string; manufacturer?: string; virtual: boolean }>>
    pulse: (action: Extract<Action, { kind: 'serial' }>) => Promise<{ ok: boolean; message: string }>
    stopAll: () => Promise<void>
  }
  obs: {
    connect: (url: string, password?: string) => Promise<{ ok: boolean; message: string }>
    command: (command: string, args?: Record<string, string>) => Promise<{ ok: boolean; message: string }>
    startVirtualCamera: () => Promise<{ ok: boolean; message: string }>
    stopVirtualCamera: () => Promise<{ ok: boolean; message: string }>
    status: () => Promise<{ connected: boolean; url?: string; message: string; virtualCameraActive?: boolean }>
  }
  features: {
    test: (featureId: FeatureId) => Promise<{ ok: boolean; message: string }>
    show: (featureId: FeatureId) => Promise<{ ok: boolean; message: string }>
    increment: (featureId: FeatureId, key: string, amount: number) => Promise<number>
  }
  auth: {
    status: () => Promise<LicenseAuthStatus>
    login: (payload: { platform: Platform; code: string; local: boolean }) => Promise<{ loggedIn: boolean; message: string }>
    logout: () => Promise<void>
    setSafeCode: (code: string) => Promise<{ ok: boolean; message: string }>
    unbind: (safeCode: string) => Promise<{ ok: boolean; message: string }>
    adminStatus: () => Promise<{ loggedIn: boolean; expiresAt?: string }>
    adminLogin: (payload: { serverUrl: string; username: string; password: string }) => Promise<{ ok: boolean; message: string; expiresAt?: string }>
    adminLogout: () => Promise<void>
    listCards: (query: LicenseCardQuery) => Promise<LicenseCardPage>
    createCards: (input: LicenseCardCreateInput) => Promise<Array<{ id: string; code: string; suffix: string }>>
    setCardStatus: (id: string, status: Extract<LicenseCardStatus, 'active' | 'disabled' | 'revoked'>) => Promise<void>
    resetCardBindings: (id: string) => Promise<void>
    cardDetail: (id: string) => Promise<LicenseCardDetail>
    extendCard: (id: string, days: number) => Promise<string | undefined>
    auditLog: (limit?: number) => Promise<LicenseAuditEntry[]>
  }
  config: {
    export: () => Promise<{ ok: boolean; path?: string; message: string }>
    import: () => Promise<{ ok: boolean; message: string; rules?: Rule[] }>
  }
  diagnostics: {
    logs: (limit?: number) => Promise<LogEntry[]>
    assets: () => Promise<string[]>
    settings: () => Promise<AppSettings>
    saveSettings: (settings: Partial<AppSettings>) => Promise<AppSettings>
    onLog: (cb: (log: LogEntry) => void) => () => void
  }
  window: {
    minimize: () => Promise<void>
    maximize: () => Promise<void>
    close: () => Promise<void>
  }
}
