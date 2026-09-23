import type { AppSettings, ConnectorStatus, DanmakuFilter, DanmakuRecord, ElectronApi, LiveEvent, LogEntry, OverlaySettings, OverlayType, OverlayWindowMode, OverlayWindowStatus, Platform, Rule } from '@shared/types'
import { createDefaultFeatureSettings } from '@shared/features'

export const api: ElectronApi = window.api ? withPlainArgs(window.api) : createBrowserApi()

// 渲染进程会把 Vue 响应式代理（reactive/ref 里的对象）直接当参数传给 IPC，而 contextBridge 与 IPC
// 的结构化克隆都不支持 Proxy，会抛 “An object could not be cloned.”。跨进程前统一还原成纯数据。
// 注意：不能直接用 Proxy 包住桥接对象（它的属性是只读且不可配置的，get 陷阱会违反代理不变量）。
function withPlainArgs<T extends object>(source: T): T {
  const wrapped: Record<string, unknown> = {}
  for (const key of Object.getOwnPropertyNames(source)) {
    const value: unknown = (source as Record<string, unknown>)[key]
    if (typeof value === 'function') wrapped[key] = (...args: unknown[]) => (value as (...rest: unknown[]) => unknown)(...args.map(toPlain))
    else if (value && typeof value === 'object') wrapped[key] = withPlainArgs(value as object)
    else wrapped[key] = value
  }
  return wrapped as T
}

function toPlain(value: unknown): unknown {
  if (value === null || typeof value !== 'object') return value
  if (value instanceof Date) return new Date(value.getTime())
  if (Array.isArray(value)) return value.map(toPlain)
  const plain: Record<string, unknown> = {}
  for (const key of Object.keys(value)) plain[key] = toPlain((value as Record<string, unknown>)[key])
  return plain
}

function createBrowserApi(): ElectronApi {
  const listeners = new EventTarget()
  const state = {
    rules: read<Rule[]>('replica.rules', []),
    records: read<DanmakuRecord[]>('replica.records', []),
    settings: read<AppSettings>('replica.settings', defaultSettings()),
    overlayStatus: createBrowserOverlayStatus(),
    logs: [] as LogEntry[],
    status: { platform: 'simulator', state: 'connected', mode: 'simulator', reconnects: 0, dropped: 0 } as ConnectorStatus,
  }
  const emit = <T>(type: string, payload: T) => listeners.dispatchEvent(new CustomEvent(type, { detail: payload }))
  const sub = <T>(type: string, callback: (payload: T) => void) => {
    const handler = (event: Event) => callback((event as CustomEvent<T>).detail)
    listeners.addEventListener(type, handler)
    return () => listeners.removeEventListener(type, handler)
  }
  return {
    rules: {
      list: async () => state.rules,
      save: async (rule) => { state.rules = [...state.rules.filter((item) => item.id !== rule.id), rule]; write('replica.rules', state.rules); return rule },
      remove: async (id) => { state.rules = state.rules.filter((rule) => rule.id !== id); write('replica.rules', state.rules) },
      clear: async () => { state.rules = []; write('replica.rules', state.rules) },
      clone: async (id) => { const source = state.rules.find((rule) => rule.id === id)!; const clone = { ...structuredClone(source), id: crypto.randomUUID(), name: `${source.name} - 副本`, enabled: false }; state.rules.push(clone); write('replica.rules', state.rules); return clone },
    },
    danmaku: {
      query: async (filter) => queryRecords(state.records, filter),
      count: async (filter = {}) => queryRecords(state.records, filter).length,
      export: async (filter, format) => ({ content: format === 'json' ? JSON.stringify(queryRecords(state.records, filter), null, 2) : format === 'csv' ? toCsv(queryRecords(state.records, filter)) : queryRecords(state.records, filter).map((record) => `${new Date(record.ts).toLocaleString()} ${record.userName ?? ''} ${record.text ?? record.giftName ?? ''}`).join('\n') }),
      clear: async (range) => { const before = state.records.length; state.records = state.records.filter((record) => (range?.from !== undefined && record.ts < range.from) || (range?.to !== undefined && record.ts > range.to)); write('replica.records', state.records); return before - state.records.length },
      onAppend: (callback) => sub('danmaku:append', callback),
    },
    conn: {
      connect: async (platform, roomId) => { state.status = { ...state.status, platform, roomId, state: 'connected' }; emit('conn:status', state.status); return state.status },
      disconnect: async () => { state.status = { ...state.status, state: 'disconnected' }; emit('conn:status', state.status); return state.status },
      status: async () => state.status,
      simulate: async (input) => { const event: LiveEvent = { id: crypto.randomUUID(), source: input.source ?? state.status.platform, roomId: input.roomId ?? state.status.roomId, kind: input.kind, user: input.user ?? { name: '模拟观众' }, gift: input.gift, text: input.text, count: input.count, timestamp: Date.now() }; const record = recordFromEvent(event); state.records.unshift(record); write('replica.records', state.records); emit('danmaku:append', record); return event },
      onStatus: (callback) => sub('conn:status', callback),
    },
    overlay: {
      open: async (type) => { const status = state.overlayStatus.find((item) => item.type === type)!; status.visible = true; emit('overlay:status', status); return state.overlayStatus },
      close: async (type) => { const status = state.overlayStatus.find((item) => item.type === type)!; status.visible = false; emit('overlay:status', status); return state.overlayStatus },
      status: async () => state.overlayStatus,
      setMode: async (type, mode) => { const status = state.overlayStatus.find((item) => item.type === type)!; const size = overlaySize(type, mode, status); status.mode = mode; status.visible = true; status.width = size.width; status.height = size.height; emit('overlay:status', status); return status },
      toggleOpacity: async (type) => { const status = state.overlayStatus.find((item) => item.type === type)!; if (type === 'green') status.opacity = status.opacity <= 0.3 ? 1 : 0.25; else { status.backgroundTransparent = !status.backgroundTransparent; state.settings.overlaySlot.backgroundTransparent = status.backgroundTransparent }; emit('overlay:status', status); return status },
      updateSettings: async (type, patch) => { const key = type === 'green' ? 'overlayGreen' : 'overlaySlot'; state.settings[key] = { ...state.settings[key], ...patch }; const status = state.overlayStatus.find((item) => item.type === type)!; status.width = state.settings[key].width; status.height = state.settings[key].height; status.opacity = state.settings[key].opacity; status.backgroundTransparent = state.settings[key].backgroundTransparent; emit('overlay:status', status); return state.settings[key] },
      playVideo: async () => undefined, drop: async () => undefined, slot: async () => undefined, widget: async () => undefined, removeWidget: async () => undefined,
      onMessage: (callback) => sub('overlay:message', callback),
      onStatus: (callback) => sub('overlay:status', callback),
    },
    audio: { play: async () => undefined, stop: async () => undefined, onMessage: (callback) => sub('audio:message', callback) },
    input: { run: async () => ({ ok: true, message: '浏览器演示模式' }), findImage: async () => ({ ok: false, message: '浏览器演示模式不执行查图' }) },
    serial: { ports: async () => [{ path: 'VIRTUAL-COM1', virtual: true }], pulse: async () => ({ ok: true, message: '浏览器演示模式' }), stopAll: async () => undefined },
    obs: { connect: async () => ({ ok: true, message: '浏览器演示模式' }), command: async () => ({ ok: true, message: '浏览器演示模式' }), startVirtualCamera: async () => ({ ok: false, message: '浏览器演示模式不支持虚拟摄像头' }), stopVirtualCamera: async () => ({ ok: true, message: '浏览器演示模式' }), status: async () => ({ connected: false, message: '浏览器演示模式' }) },
    features: { test: async () => ({ ok: true, message: '浏览器演示模式' }), show: async () => ({ ok: true, message: '浏览器演示模式' }), increment: async (_featureId, _key, amount) => amount },
    auth: {
      status: async () => ({ loggedIn: true, mode: 'local', platform: 'simulator', features: ['overlay', 'slot', 'serial'] }),
      login: async ({ local }) => local
        ? ({ loggedIn: true, message: '浏览器本地演示模式' })
        : ({ loggedIn: false, message: '浏览器演示模式不连接远程授权服务' }),
      logout: async () => undefined,
      setSafeCode: async () => ({ ok: false, message: '请在桌面应用中设置安全码' }),
      unbind: async () => ({ ok: false, message: '请在桌面应用中解绑设备' }),
      adminStatus: async () => ({ loggedIn: false }),
      adminLogin: async () => ({ ok: false, message: '浏览器演示模式不支持卡密后台' }),
      adminLogout: async () => undefined,
      listCards: async (query) => ({ items: [], total: 0, page: query.page ?? 1, pageSize: query.pageSize ?? 20 }),
      createCards: async () => { throw new Error('卡密后台仅在桌面应用中可用') },
      setCardStatus: async () => { throw new Error('卡密后台仅在桌面应用中可用') },
      resetCardBindings: async () => { throw new Error('卡密后台仅在桌面应用中可用') },
      cardDetail: async () => { throw new Error('卡密后台仅在桌面应用中可用') },
      extendCard: async () => { throw new Error('卡密后台仅在桌面应用中可用') },
      auditLog: async () => [],
    },
    config: { export: async () => ({ ok: true, message: '浏览器演示模式' }), import: async () => ({ ok: false, message: '请在 Electron 中导入配置' }) },
    diagnostics: { logs: async () => state.logs, assets: async () => [], settings: async () => state.settings, saveSettings: async (patch) => { state.settings = { ...state.settings, ...patch }; write('replica.settings', state.settings); return state.settings }, onLog: (callback) => sub('log:append', callback) },
    window: { minimize: async () => undefined, maximize: async () => undefined, close: async () => undefined },
  }
}

function queryRecords(records: DanmakuRecord[], filter: DanmakuFilter): DanmakuRecord[] {
  return records
    .filter((record) => filter.from === undefined || record.ts >= filter.from)
    .filter((record) => filter.to === undefined || record.ts <= filter.to)
    .filter((record) => !filter.source || record.source === filter.source)
    .filter((record) => !filter.kind || record.kind === filter.kind)
    .filter((record) => !filter.userName || record.userName?.includes(filter.userName))
    .filter((record) => !filter.keyword || `${record.text ?? ''}${record.giftName ?? ''}`.includes(filter.keyword))
    .filter((record) => !filter.actionResult || record.actionResult === filter.actionResult)
    .filter((record) => !filter.matched || (filter.matched === 'yes' ? Boolean(record.matchedRuleId) : !record.matchedRuleId))
    .slice(filter.offset ?? 0, (filter.offset ?? 0) + (filter.limit ?? 100))
}

function recordFromEvent(event: LiveEvent): DanmakuRecord {
  return { id: Date.now(), source: event.source, roomId: event.roomId, kind: event.kind, userId: event.user?.id, userName: event.user?.name, text: event.text, giftName: event.gift?.name, giftCount: event.gift?.count, repeatCount: event.count, ts: event.timestamp, createdAt: Date.now(), actionResult: 'none' }
}

function toCsv(records: DanmakuRecord[]): string {
  const header = ['时间', '平台', '类型', '昵称', '内容', '命中规则', '结果']
  const rows = records.map((record) => [new Date(record.ts).toLocaleString(), record.source, record.kind, record.userName ?? '', record.text ?? record.giftName ?? '', record.matchedRuleName ?? '', record.actionResult])
  return [header, ...rows].map((row) => row.map((value) => `"${String(value).replaceAll('"', '""')}"`).join(',')).join('\n')
}

function defaultSettings(): AppSettings { return { version: '7.6.3-replica', devMode: true, logLevel: 'info', retentionDays: 30, retentionMaxRows: 200000, storeRaw: false, hotkey: 'Ctrl + Shift + 1-9', audioVolume: 0.8, assetsRoot: 'resources', overlayGreen: { visible: false, alwaysOnTop: true, opacity: 1, backgroundTransparent: false, width: 960, height: 540, laneCount: 3, background: '#00ff00' }, overlaySlot: { visible: false, alwaysOnTop: true, opacity: 1, backgroundTransparent: true, width: 960, height: 540, laneCount: 1, background: '#000000' }, platform: 'simulator', roomId: 'demo-room', authServerUrl: 'http://127.0.0.1:8787', obsUrl: 'ws://127.0.0.1:4455', obsPassword: '', autoStart: false, features: createDefaultFeatureSettings() } }

function createBrowserOverlayStatus(): OverlayWindowStatus[] {
  return [
    { type: 'green', role: 'ylm', title: '阿比整蛊 - 绿幕窗口 【禁止最小化】（按Tab键可以管理视频列表）', visible: false, width: 960, height: 540, mode: 'green', fullscreen: false, opacity: 1, backgroundTransparent: false },
    { type: 'slot', role: 'yapp', title: '阿比整蛊 - 组件窗口 【禁止最小化】 快捷键切换透明度 Ctrl + F1', visible: false, width: 960, height: 540, mode: 'landscape-16-9', fullscreen: false, opacity: 1, backgroundTransparent: true },
  ]
}

function overlaySize(type: OverlayType, mode: OverlayWindowMode, current: OverlayWindowStatus): { width: number; height: number } {
  if (mode === 'green') return { width: current.width || 960, height: current.height || 540 }
  if (mode === 'landscape-4-3') return { width: 800, height: 600 }
  if (mode === 'portrait-9-16') return { width: 540, height: 960 }
  if (mode === 'fullscreen') return { width: current.width, height: current.height }
  return type === 'slot' ? { width: 960, height: 540 } : { width: 960, height: 540 }
}
function read<T>(key: string, fallback: T): T { try { const value = localStorage.getItem(key); return value ? JSON.parse(value) as T : fallback } catch { return fallback } }
function write<T>(key: string, value: T): void { localStorage.setItem(key, JSON.stringify(value)) }
