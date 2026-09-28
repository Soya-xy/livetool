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

/** 素材选择器支持的素材类别。 */
export type AssetKind = 'image' | 'video' | 'audio'

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
  | (ActionBase & { kind: 'serial'; port: string; baud: number; onBytes: number[]; offBytes: number[]; pulseMs: number })
  | (ActionBase & { kind: 'obs'; command: string; args?: Record<string, string> })
  // ── 与原版动作菜单 1:1 对齐的类型 ──
  /** 等待：阻塞指定时长后继续下一条动作。 */
  | (ActionBase & { kind: 'sleep' })
  /** 砸图片：把礼物图片砸向屏幕指定位置。 */
  | (ActionBase & { kind: 'showimage'; image: string; count?: number; size?: number; x?: number; y?: number })
  /** 随机盲盒：从素材池按权重抽一个。 */
  | (ActionBase & { kind: 'randombox'; pool?: string[]; weights?: number[] })
  /** 手机玩法：在绿幕窗口播放手机端玩法视频。 */
  | (ActionBase & { kind: 'app'; appAction?: string; path: string; loop?: boolean })
  /** 加速度：按到达数量把队列加速完成。 */
  | (ActionBase & { kind: 'gospeed'; targetCount?: number; speed?: number })
  /** OBS场景滤镜：切换滤镜显隐，可选到点自动隐藏。 */
  | (ActionBase & { kind: 'obs_filter'; filterName: string; visible?: boolean; autoHide?: boolean })
  /** 内置事件：触发原版的内置事件表条目（事件 ID 见 shared/builtinEvents.ts）。 */
  | (ActionBase & { kind: 'rule'; ruleEvent: string; ruleEventTimeoutMs?: number; killProcessName?: string; runExePath?: string })
  /** 屏幕锁链：给锁链层数加/减若干层。 */
  | (ActionBase & { kind: 'tielian'; delta?: number; effect?: string; countMin?: number; countMax?: number })
  /** 垃圾掉落：掉落垃圾，堆满垃圾桶。 */
  | (ActionBase & { kind: 'trash'; count?: number; image?: string; bin?: string })

export interface RuleTrigger {
  kinds: EventKind[]
  giftNames?: string[]
  keywords?: string[]
  keywordMode?: 'exact' | 'contains' | 'regex'
  minCount?: number
  users?: string[]
  source?: string[]
}

/** 触发限制：同一用户在该秒数内只触发一次；0 表示不限制。 */
export interface RuleTriggerLimits {
  giftSecond?: number
  textSecond?: number
  likeSecond?: number
  enterSecond?: number
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
  /** 置顶：固定在控制中心列表最前。 */
  pinned?: boolean
  /** 玩法热键：形如 `Ctrl + Shift + F`。 */
  hotkey?: string
  /** 重复执行次数：整条动作列表重复执行的轮数，默认 1。 */
  repeatCount?: number
  /** 立即执行：跳过排队，匹配后立刻开始。 */
  nowait?: boolean
  /** 等待时长（毫秒）：排队执行时动作开始前先等这么久，立即执行时不生效。 */
  waitMs?: number
  triggerLimits?: RuleTriggerLimits
  /** 播放声音：规则命中时播放配置的声音素材。 */
  playSound?: boolean
  /** 同时播放声音：允许多条声音叠放，而不是排队。 */
  soundSimultaneous?: boolean
  /** 播放间隔（毫秒）：两条声音之间的最小间隔，默认 800。 */
  soundIntervalMs?: number
  /** 累计动作时间：把动作耗时计入队列等待时间。 */
  countActionTime?: boolean
  /** 不参与加速：开启后「加速度」对本条玩法无效。 */
  noAcceleration?: boolean
  /** 盲盒概率：数值越大越容易抽中，默认 1。 */
  boxWeight?: number
}

/** 控制中心底部状态：全局开启/停止、暂停/继续与固定热键。 */
export interface RulesRuntimeState {
  enabled: boolean
  paused: boolean
  /** Ctrl + Shift + 1-9 调试快捷键当前是否可用。 */
  debugKeys: boolean
  openKey: string
  closeKey: string
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

/** 连接状态：取值与原版 webcastState 的 state 一致（connect/reConnect/success/error/break/timeout）。 */
export type ConnectorState = 'disconnected' | 'connecting' | 'connected' | 'reconnecting' | 'error' | 'break' | 'timeout'

export type ConnectorMode = 'simulator' | 'websocket' | 'polling' | 'relay'

export interface ConnectorStatus {
  platform: string
  roomId?: string
  state: ConnectorState
  mode: ConnectorMode
  reconnects: number
  dropped: number
  lastError?: string
  lastEventAt?: number
}

/** 组件窗里某个组件的位置与大小（拖动 / 拉伸后保存）。 */
export interface WidgetLayout {
  x: number
  y: number
  w: number
  h: number
}

export interface OverlaySettings {
  visible: boolean
  alwaysOnTop: boolean
  opacity: number
  /** 组件窗口专用：true = 完全透明底板，false = 不透明深色底板。绿幕窗口不使用。 */
  backgroundTransparent: boolean
  /** 组件窗口专用：在角落显示帧率与长帧统计，用于定位拖动 / 拉伸卡顿。 */
  showPerf?: boolean
  width: number
  height: number
  laneCount: number
  background: string
}

export type OverlayType = 'green' | 'slot'

export type OverlayWidgetKind = 'acceleration' | 'health' | 'timer' | 'sticker' | 'lock' | 'reply' | 'notice' | 'speech' | 'countdown' | 'trash' | 'menu'

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
  backgroundTransparent: boolean
  showPerf?: boolean
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
  /** 禁用组刷：同一用户 1.2 秒内连送同款礼物只执行 1 次操作；关闭时每条礼物消息都会执行。 */
  isDisableGiftGroup: boolean
  /** 操作顺序执行：同一时间只执行一条玩法的动作列表。 */
  isOrder: boolean
  /** 禁用OBS连接：开启后所有 OBS 调用都会被拒绝。 */
  isDisableOBS: boolean
  /** 调试延时（毫秒）：调试快捷键与玩法热键触发前的等待时间。 */
  debugSleepMs: number
  /** 鼠标移动步长：上下 / 左右；0 表示一次到位。 */
  moveTopBottomStep: number
  moveLeftRightStep: number
}

export type LicenseEntitlement = 'overlay' | 'slot' | 'serial'

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

export interface OperationResult {
  ok: boolean
  message: string
  path?: string
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

export interface LiveToolApi {
  rules: {
    list: () => Promise<Rule[]>
    save: (rule: Rule) => Promise<Rule>
    remove: (id: string) => Promise<void>
    clear: () => Promise<void>
    /** 克隆玩法；count 为克隆份数（1–50）。 */
    clone: (id: string, count?: number) => Promise<Rule[]>
    /** 置顶 / 取消置顶。 */
    setPinned: (id: string, pinned: boolean) => Promise<Rule>
    /** 立即执行该玩法的动作列表（调试与玩法热键使用）。 */
    trigger: (id: string) => Promise<OperationResult>
    runtimeState: () => Promise<RulesRuntimeState>
    setEnabled: (enabled: boolean) => Promise<RulesRuntimeState>
    setPaused: (paused: boolean) => Promise<RulesRuntimeState>
    onState: (cb: (state: RulesRuntimeState) => void) => () => void
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
    ready: (type: 'green' | 'slot') => Promise<void>
    setMode: (type: OverlayType, mode: OverlayWindowMode) => Promise<OverlayWindowStatus>
    toggleOpacity: (type: OverlayType) => Promise<OverlayWindowStatus>
    updateSettings: (type: 'green' | 'slot', settings: Partial<OverlaySettings>) => Promise<OverlaySettings>
    playVideo: (payload: { path: string; lane?: number; durationMs?: number; loop?: boolean; chroma?: ChromaConfig }) => Promise<void>
    drop: (payload: { image: string; count?: number; gravity?: number; bounce?: number; durationMs?: number; maxVisible?: number }) => Promise<void>
    slot: (payload: { theme?: string; pool?: string[]; weights?: number[]; images?: string[]; durationMs?: number; audioPath?: string }) => Promise<void>
    widget: (payload: OverlayWidgetPayload) => Promise<void>
    removeWidget: (featureId: FeatureId) => Promise<void>
    decrementScreenLock: () => Promise<number>
    widgetLayouts: () => Promise<Record<string, WidgetLayout>>
    saveWidgetLayout: (featureId: string, layout: WidgetLayout) => Promise<OperationResult>
    onMessage: (cb: (message: { target: 'green' | 'slot'; type: string; payload: unknown }) => void) => () => void
    onStatus: (cb: (status: OverlayWindowStatus) => void) => () => void
  }
  audio: {
    ready: () => Promise<void>
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
    selectLockMedia: () => Promise<string>
    menuGift: (giftName: string) => Promise<{ ok: boolean; message: string }>
    giftIcon: (name: string) => Promise<string>
  }
  auth: {
    status: () => Promise<LicenseAuthStatus>
    developmentModeAvailable: () => Promise<boolean>
    login: (payload: { platform: Platform; code: string; local: boolean }) => Promise<{ loggedIn: boolean; message: string }>
    logout: () => Promise<void>
    setSafeCode: (code: string) => Promise<{ ok: boolean; message: string }>
    unbind: (safeCode: string) => Promise<{ ok: boolean; message: string }>
  }
  config: {
    export: () => Promise<{ ok: boolean; path?: string; message: string }>
    import: () => Promise<{ ok: boolean; message: string; rules?: Rule[] }>
  }
  diagnostics: {
    logs: (limit?: number) => Promise<LogEntry[]>
    assets: () => Promise<string[]>
    /** 上传素材：选择本地文件并复制进素材目录，返回相对路径；取消时返回空字符串。 */
    importAsset: (kind: AssetKind) => Promise<string>
    settings: () => Promise<AppSettings>
    saveSettings: (settings: Partial<AppSettings>) => Promise<AppSettings>
    onLog: (cb: (log: LogEntry) => void) => () => void
  }
  window: {
    minimize: () => Promise<void>
    maximize: () => Promise<void>
    close: () => Promise<void>
  }
  updater: {
    check: () => Promise<OperationResult>
    install: () => Promise<OperationResult>
    restart: () => Promise<OperationResult>
  }
}
