import { contextBridge, ipcRenderer } from 'electron'
import type { AppSettings, ConnectorStatus, DanmakuFilter, DanmakuRecord, ElectronApi, LicenseCardCreateInput, LicenseCardQuery, LicenseCardStatus, LiveEvent, LogEntry, OverlaySettings, OverlayType, OverlayWindowMode, OverlayWidgetPayload, Platform, Rule } from '@shared/types'

const api: ElectronApi = {
  rules: {
    list: () => ipcRenderer.invoke('rules:list'),
    save: (rule: Rule) => ipcRenderer.invoke('rules:save', rule),
    remove: (id: string) => ipcRenderer.invoke('rules:remove', id),
    clear: () => ipcRenderer.invoke('rules:clear'),
    clone: (id: string) => ipcRenderer.invoke('rules:clone', id),
  },
  danmaku: {
    query: (filter: DanmakuFilter) => ipcRenderer.invoke('danmaku:query', filter),
    count: (filter?: DanmakuFilter) => ipcRenderer.invoke('danmaku:count', filter),
    export: (filter: DanmakuFilter, format: 'json' | 'csv' | 'txt') => ipcRenderer.invoke('danmaku:export', { filter, format }),
    clear: (filter?: { from?: number; to?: number }) => ipcRenderer.invoke('danmaku:clear', filter),
    onAppend: (callback: (record: DanmakuRecord) => void) => subscribe('danmaku:append', callback),
  },
  conn: {
    connect: (platform: Platform, roomId: string) => ipcRenderer.invoke('conn:connect', platform, roomId),
    disconnect: () => ipcRenderer.invoke('conn:disconnect'),
    status: () => ipcRenderer.invoke('conn:status'),
    simulate: (event: Partial<LiveEvent> & Pick<LiveEvent, 'kind'>) => ipcRenderer.invoke('conn:simulate', event),
    onStatus: (callback: (status: ConnectorStatus) => void) => subscribe('conn:status', callback),
  },
  overlay: {
    open: (type: 'green' | 'slot') => ipcRenderer.invoke('overlay:open', type),
    close: (type: 'green' | 'slot') => ipcRenderer.invoke('overlay:close', type),
    status: () => ipcRenderer.invoke('overlay:status'),
    setMode: (type: OverlayType, mode: OverlayWindowMode) => ipcRenderer.invoke('overlay:setMode', type, mode),
    toggleOpacity: (type: OverlayType) => ipcRenderer.invoke('overlay:toggleOpacity', type),
    updateSettings: (type: 'green' | 'slot', settings: Partial<OverlaySettings>) => ipcRenderer.invoke('overlay:updateSettings', type, settings),
    playVideo: (payload) => ipcRenderer.invoke('overlay:playVideo', payload),
    drop: (payload) => ipcRenderer.invoke('overlay:drop', payload),
    slot: (payload) => ipcRenderer.invoke('overlay:slot', payload),
    widget: (payload: OverlayWidgetPayload) => ipcRenderer.invoke('overlay:widget', payload),
    removeWidget: (featureId) => ipcRenderer.invoke('overlay:removeWidget', featureId),
    onMessage: (callback) => subscribe('overlay:message', callback),
    onStatus: (callback) => subscribe('overlay:status', callback),
  },
  audio: {
    play: (payload) => ipcRenderer.invoke('audio:play', payload),
    stop: () => ipcRenderer.invoke('audio:stop'),
    onMessage: (callback) => subscribe('audio:message', callback),
  },
  input: {
    run: (action) => ipcRenderer.invoke('input:run', action),
    findImage: (image, threshold) => ipcRenderer.invoke('input:findImage', image, threshold),
  },
  serial: {
    ports: () => ipcRenderer.invoke('serial:ports'),
    pulse: (action) => ipcRenderer.invoke('serial:pulse', action),
    stopAll: () => ipcRenderer.invoke('serial:stopAll'),
  },
  obs: {
    connect: (url, password) => ipcRenderer.invoke('obs:connect', url, password),
    command: (command, args) => ipcRenderer.invoke('obs:command', command, args),
    startVirtualCamera: () => ipcRenderer.invoke('obs:startVirtualCamera'),
    stopVirtualCamera: () => ipcRenderer.invoke('obs:stopVirtualCamera'),
    status: () => ipcRenderer.invoke('obs:status'),
  },
  features: {
    test: (featureId) => ipcRenderer.invoke('features:test', featureId),
    show: (featureId) => ipcRenderer.invoke('features:show', featureId),
    increment: (featureId, key, amount) => ipcRenderer.invoke('features:increment', featureId, key, amount),
  },
  auth: {
    status: () => ipcRenderer.invoke('auth:status'),
    login: (payload) => ipcRenderer.invoke('auth:login', payload),
    logout: () => ipcRenderer.invoke('auth:logout'),
    setSafeCode: (code: string) => ipcRenderer.invoke('auth:setSafeCode', code),
    unbind: (safeCode: string) => ipcRenderer.invoke('auth:unbind', safeCode),
    adminStatus: () => ipcRenderer.invoke('auth:adminStatus'),
    adminLogin: (payload) => ipcRenderer.invoke('auth:adminLogin', payload),
    adminLogout: () => ipcRenderer.invoke('auth:adminLogout'),
    listCards: (query: LicenseCardQuery) => ipcRenderer.invoke('auth:listCards', query),
    createCards: (input: LicenseCardCreateInput) => ipcRenderer.invoke('auth:createCards', input),
    setCardStatus: (id: string, status: Extract<LicenseCardStatus, 'active' | 'disabled' | 'revoked'>) => ipcRenderer.invoke('auth:setCardStatus', id, status),
    resetCardBindings: (id: string) => ipcRenderer.invoke('auth:resetCardBindings', id),
    cardDetail: (id: string) => ipcRenderer.invoke('auth:cardDetail', id),
    extendCard: (id: string, days: number) => ipcRenderer.invoke('auth:extendCard', id, days),
    auditLog: (limit?: number) => ipcRenderer.invoke('auth:auditLog', limit),
  },
  config: {
    export: () => ipcRenderer.invoke('config:export'),
    import: () => ipcRenderer.invoke('config:import'),
  },
  diagnostics: {
    logs: (limit?: number) => ipcRenderer.invoke('diagnostics:logs', limit),
    assets: () => ipcRenderer.invoke('diagnostics:assets'),
    settings: () => ipcRenderer.invoke('diagnostics:settings'),
    saveSettings: (settings: Partial<AppSettings>) => ipcRenderer.invoke('diagnostics:saveSettings', settings),
    onLog: (callback: (log: LogEntry) => void) => subscribe('log:append', callback),
  },
  window: {
    minimize: () => ipcRenderer.invoke('window:minimize'),
    maximize: () => ipcRenderer.invoke('window:maximize'),
    close: () => ipcRenderer.invoke('window:close'),
  },
}

contextBridge.exposeInMainWorld('api', api)

function subscribe<T>(channel: string, callback: (payload: T) => void): () => void {
  const listener = (_event: Electron.IpcRendererEvent, payload: T) => callback(payload)
  ipcRenderer.on(channel, listener)
  return () => ipcRenderer.removeListener(channel, listener)
}
