import { Events } from '@wailsio/runtime'
import * as Backend from '../../bindings/livetool/appservice'
import type * as Models from '../../bindings/livetool/models'
import type { LiveToolApi, OperationResult, Platform } from '@shared/types'

function result<T>(call: PromiseLike<unknown>): Promise<T> {
  return Promise.resolve(call).then((value) => value as T)
}

function subscribe<T>(name: string, callback: (payload: T) => void): () => void {
  return Events.On(name, (event) => callback(event.data as T))
}

function backendPlatform(value: Platform): Models.Platform {
  return value as unknown as Models.Platform
}

export const api: LiveToolApi = {
  rules: {
    list: () => result(Backend.RulesList()),
    save: (rule) => result(Backend.RulesSave(rule as unknown as Models.Rule)),
    remove: (id) => result(Backend.RulesRemove(id)),
    clear: () => result(Backend.RulesClear()),
    clone: (id) => result(Backend.RulesClone(id)),
  },
  danmaku: {
    query: (filter) => result(Backend.DanmakuQuery(filter as Models.DanmakuFilter)),
    count: (filter = {}) => result(Backend.DanmakuCount(filter as Models.DanmakuFilter)),
    export: (filter, format) => result(Backend.DanmakuExport(filter as Models.DanmakuFilter, format)),
    clear: (filter) => result(Backend.DanmakuClear({ from: filter?.from ?? 0, to: filter?.to ?? 0 })),
    onAppend: (callback) => subscribe('danmaku:append', callback),
  },
  conn: {
    connect: (platform, roomId) => result(Backend.ConnConnect(backendPlatform(platform), roomId)),
    disconnect: () => result(Backend.ConnDisconnect()),
    status: () => result(Backend.ConnStatus()),
    simulate: (event) => result(Backend.ConnSimulate(event as Models.LiveEvent)),
    onStatus: (callback) => subscribe('conn:status', callback),
  },
  overlay: {
    open: (kind) => result(Backend.OverlayOpen(kind)),
    close: (kind) => result(Backend.OverlayClose(kind)),
    status: () => result(Backend.OverlayStatus()),
    ready: (kind) => result(Backend.OverlayReady(kind)),
    setMode: (kind, mode) => result(Backend.OverlaySetMode(kind, mode)),
    toggleOpacity: (kind) => result(Backend.OverlayToggleOpacity(kind)),
    updateSettings: (kind, settings) => result(Backend.OverlayUpdateSettings(kind, settings)),
    playVideo: (payload) => result(Backend.OverlayPlayVideo(payload)),
    drop: (payload) => result(Backend.OverlayDrop(payload)),
    slot: (payload) => result(Backend.OverlaySlot(payload)),
    widget: (payload) => result(Backend.OverlayWidget(payload as unknown as Models.OverlayWidgetPayload)),
    removeWidget: (featureId) => result(Backend.OverlayRemoveWidget(featureId as unknown as Models.FeatureID)),
    decrementScreenLock: () => result(Backend.OverlayDecrementScreenLock()),
    onMessage: (callback) => subscribe('overlay:message', callback),
    onStatus: (callback) => subscribe('overlay:status', callback),
  },
  audio: {
    ready: () => result(Backend.AudioReady()),
    play: (payload) => result(Backend.AudioPlay(payload)),
    stop: () => result(Backend.AudioStop()),
    onMessage: (callback) => subscribe('audio:message', callback),
  },
  input: {
    run: (action) => result(Backend.InputRun(action as unknown as Models.Action)),
    findImage: (image, threshold) => result(Backend.InputFindImage(image, threshold)),
  },
  serial: {
    ports: () => result(Backend.SerialPorts()),
    pulse: (action) => result(Backend.SerialPulse(action as unknown as Models.Action)),
    stopAll: () => result(Backend.SerialStopAll()),
  },
  obs: {
    connect: (address, password) => result(Backend.OBSConnect(address, password ?? '')),
    command: (command, args) => result(Backend.OBSCommand(command, args ?? {})),
    startVirtualCamera: () => result(Backend.OBSStartVirtualCamera()),
    stopVirtualCamera: () => result(Backend.OBSStopVirtualCamera()),
    status: () => result(Backend.OBSStatus()),
  },
  features: {
    test: (id) => result(Backend.FeatureTest(id as unknown as Models.FeatureID)),
    show: (id) => result(Backend.FeatureShow(id as unknown as Models.FeatureID)),
    increment: (id, key, amount) => result(Backend.FeatureIncrement(id as unknown as Models.FeatureID, key, amount)),
    selectLockMedia: () => result(Backend.FeatureSelectLockMedia()),
  },
  auth: {
    status: () => result(Backend.AuthStatus()),
    developmentModeAvailable: () => result(Backend.AuthDevelopmentModeAvailable()),
    login: (payload) => result(Backend.AuthLogin({ ...payload, platform: backendPlatform(payload.platform) })),
    logout: () => result(Backend.AuthLogout()),
    setSafeCode: (code) => result(Backend.AuthSetSafeCode(code)),
    unbind: (safeCode) => result(Backend.AuthUnbind(safeCode)),
  },
  config: {
    export: () => result(Backend.ConfigExport()),
    import: () => result(Backend.ConfigImport()),
  },
  diagnostics: {
    logs: (limit) => result(Backend.DiagnosticsLogs(limit ?? 100)),
    assets: () => result(Backend.DiagnosticsAssets()),
    settings: () => result(Backend.DiagnosticsSettings()),
    saveSettings: (settings) => result(Backend.DiagnosticsSaveSettings(settings)),
    onLog: (callback) => subscribe('log:append', callback),
  },
  window: {
    minimize: async () => Backend.WindowMinimize(),
    maximize: async () => Backend.WindowMaximize(),
    close: async () => Backend.WindowClose(),
  },
  updater: {
    check: () => result<OperationResult>(Backend.UpdaterCheck()),
    install: () => result<OperationResult>(Backend.UpdaterInstall()),
    restart: () => result<OperationResult>(Backend.UpdaterRestart()),
  },
}
