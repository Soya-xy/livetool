import { app, BrowserWindow, dialog, globalShortcut, ipcMain, session } from 'electron'
import { existsSync } from 'node:fs'
import { mkdir, readFile, readdir, writeFile } from 'node:fs/promises'
import { basename, extname, isAbsolute, join, relative, resolve, sep } from 'node:path'
import { randomUUID } from 'node:crypto'
import { createRequire } from 'node:module'
import { fileURLToPath, pathToFileURL } from 'node:url'
import type {
  Action,
  AppSettings,
  DanmakuFilter,
  DanmakuRecord,
  ExportConfig,
  LicenseAuthStatus,
  LicenseCardCreateInput,
  LicenseCardDetail,
  LicenseCardQuery,
  LicenseCardStatus,
  LicenseEntitlement,
  LiveEvent,
  OverlaySettings,
  OverlayType,
  OverlayWidgetPayload,
  OverlayWindowMode,
  Platform,
  Rule,
} from '@shared/types'
import type { FeatureConfig, FeatureId, FeatureValue } from '@shared/features'
import { createDefaultFeatureSettings } from '@shared/features'
import { RuleEngine } from './core/rule-engine'
import { ActionExecutor } from './services/action-executor'
import { ConnectorManager } from './services/connectors'
import { AppLogger } from './services/logger'
import { SqliteStore } from './services/store'
import { WindowManager } from './services/windows'
import { ObsWebSocketClient } from './services/obs-websocket'
import { LicenseClient, normalizeServerUrl } from './services/license-client'
import { featureEntitlement } from '@shared/license'

const __filename = fileURLToPath(import.meta.url)
const __dirname = resolve(__filename, '..')
const APP_VERSION = '7.6.3-replica'
const SUPPORTED_PLATFORMS: Platform[] = ['simulator', 'douyin', 'kuaishou', 'shipinhao', 'bilibili', 'tiktok', 'douyu', 'xiaohongshu']

let mainWindow: BrowserWindow | undefined
let store: SqliteStore
let logger: AppLogger
let connectors: ConnectorManager
let windows: WindowManager
let engine: RuleEngine
let actionExecutor: ActionExecutor
let settings: AppSettings
let dbPath = ''
let installId = ''
let persistTimer: ReturnType<typeof setTimeout> | undefined
let recordQueue = Promise.resolve()
const featureCooldowns = new Map<FeatureId, number>()
let auth: LicenseAuthStatus = { loggedIn: false, mode: 'remote', platform: 'simulator', features: [] }
let authSessionExpiresAt = 0
const licenseClient = new LicenseClient()
const obsClient = new ObsWebSocketClient()
const optionalRequire = createRequire(import.meta.url)

const defaultSettings: AppSettings = {
  version: APP_VERSION,
  devMode: true,
  logLevel: 'info',
  retentionDays: 30,
  retentionMaxRows: 200000,
  storeRaw: false,
  hotkey: 'Ctrl + Shift + 1-9',
  audioVolume: 0.8,
  assetsRoot: '',
  overlayGreen: { visible: false, alwaysOnTop: true, opacity: 1, backgroundTransparent: false, width: 960, height: 540, laneCount: 3, background: '#00ff00' },
  overlaySlot: { visible: false, alwaysOnTop: true, opacity: 1, backgroundTransparent: true, width: 960, height: 540, laneCount: 1, background: '#000000' },
  platform: 'simulator',
  roomId: 'demo-room',
  authServerUrl: 'http://127.0.0.1:8787',
  obsUrl: 'ws://127.0.0.1:4455',
  obsPassword: '',
  autoStart: false,
  features: createDefaultFeatureSettings(),
}

void bootstrap()

async function bootstrap(): Promise<void> {
  await app.whenReady()
  app.setName('阿比整蛊复刻版')
  const userData = app.getPath('userData')
  const dataDir = join(userData, 'data')
  const logDir = join(userData, 'logs')
  const configDir = join(userData, 'configs')
  await Promise.all([mkdir(dataDir, { recursive: true }), mkdir(logDir, { recursive: true }), mkdir(configDir, { recursive: true })])
  dbPath = join(dataDir, 'app.db')
  logger = new AppLogger(logDir)
  store = new SqliteStore()
  try {
    await store.init(dbPath)
  } catch (error) {
    logger.error('SQLite 初始化失败，将退出应用', error instanceof Error ? error.message : String(error))
    dialog.showErrorBox('初始化失败', '本地数据存储初始化失败，请检查应用数据目录权限。')
    app.quit()
    return
  }
  logger.on((entry) => store.saveLog(entry))
  logger.on((entry) => push('log:append', entry))
  settings = mergeSettings(store.getSetting('settings', defaultSettings))
  settings.assetsRoot = normalizeAssetsRoot(settings.assetsRoot)
  installId = store.getSetting('authorizationInstallId', '')
  if (!installId) {
    installId = randomUUID()
    store.setSetting('authorizationInstallId', installId)
    await store.persist(dbPath)
  }
  if (!app.isPackaged) auth = { loggedIn: true, mode: 'local', platform: settings.platform, features: ['overlay', 'slot', 'serial'] }
  windows = new WindowManager(join(__dirname, '../preload/index.cjs'))
  windows.updateSettings('green', settings.overlayGreen)
  windows.updateSettings('slot', settings.overlaySlot)
  windows.onMessage((message) => push('overlay:message', message))
  windows.onStatus((status) => push('overlay:status', status))
  connectors = new ConnectorManager()
  connectors.onStatus((status) => push('conn:status', status))
  actionExecutor = new ActionExecutor(windows, logger)
  engine = new RuleEngine({
    executeAction: (action, event, rule) => {
      const entitlement = actionEntitlement(action)
      if (!isAuthorized(entitlement)) return Promise.resolve({ ok: false, message: entitlement ? `当前授权未包含 ${entitlement} 权限` : '请先登录授权或进入本地开发模式' })
      return actionExecutor.execute(resolveActionPaths(action), event, rule)
    },
    log: (level, message, detail) => logger.write(level, 'rule', message, detail),
  })
  connectors.onEvent((event) => { recordQueue = recordQueue.then(() => handleEvent(event)).catch((error) => { logger.error('事件处理失败', String(error)) }) })
  registerIpc()
  createMainWindow()
  registerDebugHotkeys()
  registerWindowHotkeys()
  const removedByRetention = store.cleanupRetention(settings.retentionDays, settings.retentionMaxRows)
  if (removedByRetention) logger.info(`[retention] removed=${removedByRetention}`)
  requestPersist()
  logger.info('应用已启动', `version=${APP_VERSION} mode=local-simulator`)
}

function createMainWindow(): void {
  mainWindow = new BrowserWindow({
    width: 1280,
    height: 820,
    minWidth: 1080,
    minHeight: 720,
    frame: false,
    show: false,
    backgroundColor: '#f4f7fb',
    webPreferences: { preload: join(__dirname, '../preload/index.cjs'), sandbox: true, contextIsolation: true, nodeIntegration: false },
  })
  windows.attachMainWindow(mainWindow)
  mainWindow.once('ready-to-show', () => mainWindow?.show())
  mainWindow.on('closed', () => { mainWindow = undefined })
  mainWindow.webContents.on('preload-error', (_event, preloadPath, error) => logger.error(`[preload] ${preloadPath}`, error.message))
  mainWindow.webContents.setWindowOpenHandler(() => ({ action: 'deny' }))
  mainWindow.webContents.on('will-navigate', (event, url) => {
    if (!url.startsWith('file:') && !url.startsWith('http://127.0.0.1:')) event.preventDefault()
  })
  const rendererUrl = process.env.ELECTRON_RENDERER_URL
  if (rendererUrl) void mainWindow.loadURL(`${rendererUrl}#/control`)
  else void mainWindow.loadFile(join(__dirname, '../renderer/index.html'), { hash: '/control' })
}

function registerDebugHotkeys(): void {
  if (!settings.devMode) return
  for (let index = 1; index <= 9; index += 1) {
    const accelerator = `CommandOrControl+Shift+${index}`
    const registered = globalShortcut.register(accelerator, () => {
      void debugRuleByIndex(index - 1)
    })
    if (!registered) logger.warn(`[hotkey] 无法注册 ${accelerator}`)
  }
}

function registerWindowHotkeys(): void {
  const registered = globalShortcut.register('CommandOrControl+F1', () => {
    void windows.toggleOpacity('slot')
  })
  if (!registered) logger.warn('[hotkey] 无法注册 CommandOrControl+F1（组件窗口透明度）')
}

async function debugRuleByIndex(index: number): Promise<void> {
  const rule = (await store.listRules())[index]
  if (!rule) { logger.debug(`[hotkey] 未找到第 ${index + 1} 条规则`); return }
  const kind = rule.trigger.kinds[0] ?? 'gift'
  await connectors.publish(kind === 'gift'
    ? { kind, gift: { name: rule.trigger.giftNames?.[0] ?? rule.trigger.keywords?.[0] ?? '啤酒', count: Math.max(1, rule.trigger.minCount ?? 1) }, user: { name: '快捷键调试' } }
    : { kind, text: rule.trigger.keywords?.[0] ?? '调试弹幕', user: { name: '快捷键调试' } })
  logger.info(`[hotkey] sent rule=${rule.name}`)
}

function registerIpc(): void {
  handle('rules:list', () => store.listRules())
  handle('rules:save', async (_event, rule: Rule) => { const saved = await store.saveRule(normalizeRule(rule)); requestPersist(); return saved })
  handle('rules:remove', async (_event, id: string) => { await store.removeRule(id); requestPersist() })
  handle('rules:clear', async () => { await store.clearRules(); requestPersist() })
  handle('rules:clone', async (_event, id: string) => {
    const found = (await store.listRules()).find((rule) => rule.id === id)
    if (!found) throw new Error('规则不存在')
    const clone = await store.saveRule({ ...structuredClone(found), id: randomUUID(), name: `${found.name} - 副本`, enabled: false })
    requestPersist()
    return clone
  })

  handle('danmaku:query', (_event, filter: DanmakuFilter) => store.queryRecords(filter))
  handle('danmaku:count', (_event, filter?: DanmakuFilter) => store.countRecords(filter))
  handle('danmaku:export', async (_event, payload: { filter: DanmakuFilter; format: 'json' | 'csv' | 'txt' }) => exportRecords(payload.filter, payload.format))
  handle('danmaku:clear', (_event, range?: { from?: number; to?: number }) => { const count = store.clearRecords(range); requestPersist(); logger.info(`[danmaku:store] cleared ${count} records`); return count })

  handle('conn:connect', async (_event, platform: Platform, roomId: string) => { settings.platform = platform; settings.roomId = roomId; saveSettings(); return connectors.connect(platform, roomId) })
  handle('conn:disconnect', () => connectors.disconnect())
  handle('conn:status', () => connectors.getStatus())
  handle('conn:simulate', (_event, input: Partial<LiveEvent> & Pick<LiveEvent, 'kind'>) => connectors.publish(input))

  handle('overlay:open', async (_event, type: OverlayType) => { requireEntitlement(type === 'green' ? 'overlay' : 'slot'); await windows.open(type); return windows.getStatus() })
  handle('overlay:close', async (_event, type: OverlayType) => { await windows.close(type); return windows.getStatus() })
  handle('overlay:status', () => windows.getStatus())
  handle('overlay:setMode', (_event, type: OverlayType, mode: OverlayWindowMode) => { requireEntitlement(type === 'green' ? 'overlay' : 'slot'); return windows.setMode(type, mode) })
  handle('overlay:toggleOpacity', async (_event, type: OverlayType) => {
    requireEntitlement(type === 'green' ? 'overlay' : 'slot')
    const status = await windows.toggleOpacity(type)
    // 组件窗口的底板开关要跟着设置落盘，重启后保持；绿幕窗口沿用原有行为。
    if (type === 'slot') { settings.overlaySlot = windows.getSettings('slot'); saveSettings() }
    return status
  })
  handle('overlay:updateSettings', (_event, type: 'green' | 'slot', patch: Partial<OverlaySettings>) => {
    requireEntitlement(type === 'green' ? 'overlay' : 'slot')
    const updated = windows.updateSettings(type, patch)
    if (type === 'green') settings.overlayGreen = updated
    else settings.overlaySlot = updated
    saveSettings()
    return updated
  })
  handle('overlay:playVideo', (_event, payload) => { requireEntitlement('overlay'); return windows.playVideo({ ...payload, path: resolveAssetPath(payload.path) }) })
  handle('overlay:drop', (_event, payload) => { requireEntitlement('overlay'); return windows.drop({ ...payload, image: resolveAssetPath(payload.image) }) })
  handle('overlay:slot', (_event, payload) => {
    requireEntitlement('slot')
    return windows.slot({
    ...payload,
    images: payload.images?.map((image: string) => resolveAssetPath(image)),
    audioPath: payload.audioPath ? resolveAssetPath(payload.audioPath) : undefined,
    })
  })
  handle('overlay:widget', (_event, payload: OverlayWidgetPayload) => {
    requireEntitlement(featureEntitlement(payload.featureId))
    const values = { ...payload.values }
    for (const key of ['imagePath', 'audioPath']) {
      if (typeof values[key] === 'string' && values[key]) values[key] = resolveAssetPath(values[key] as string)
    }
    return windows.widget({ ...payload, values })
  })
  handle('overlay:removeWidget', (_event, featureId: FeatureId) => windows.removeWidget(featureId))
  handle('features:increment', (_event, featureId: FeatureId, key: string, amount: number) => {
    requireEntitlement('slot')
    if (featureId !== 'electronic-woodfish' || key !== 'currentMerit') throw new Error('不允许更新此功能状态')
    const config = settings.features[featureId]
    const current = typeof config.values[key] === 'number' ? config.values[key] as number : 0
    const increment = Number.isFinite(amount) ? Math.max(0, Math.floor(amount)) : 0
    const value = Math.max(0, current + increment)
    config.values[key] = value
    saveSettings()
    return value
  })
  handle('audio:play', (_event, payload) => { requireEntitlement('slot'); return windows.playAudio({ ...payload, path: resolveAssetPath(payload.path) }) })
  handle('audio:stop', () => windows.stopAudio())

  handle('input:run', (_event, action: Extract<Action, { kind: 'key' | 'mouse' }>) => {
    logger.info(`[input-helper] ${action.kind} action recorded in safe simulator mode`)
    return { ok: true, message: '安全模拟模式：已记录动作，未向系统注入键鼠事件' }
  })
  handle('input:findImage', (_event, image: string, threshold: number) => ({ ok: false, message: `未执行系统查图：${basename(image)} threshold=${threshold}`, confidence: 0 }))
  handle('serial:ports', async () => {
    const virtual = [{ path: 'VIRTUAL-COM1', manufacturer: 'Replica simulator', virtual: true }]
    try {
      const serialport = optionalRequire('serialport') as { SerialPort: { list: () => Promise<Array<{ path: string; manufacturer?: string }>> } }
      const ports = await serialport.SerialPort.list()
      return [...virtual, ...ports.map((port) => ({ path: port.path, manufacturer: port.manufacturer, virtual: false }))]
    } catch { return virtual }
  })
  handle('serial:pulse', (_event, action: Extract<Action, { kind: 'serial' }>) => {
    requireEntitlement('serial')
    logger.info(`[serial] virtual pulse port=${action.port} duration=${action.pulseMs}ms`)
    return { ok: true, message: '虚拟串口模式：脉冲已记录，未写入物理设备' }
  })
  handle('serial:stopAll', () => logger.info('[serial] emergency stop: all outputs released'))

  handle('obs:connect', async (_event, url: string, password?: string) => {
    requireEntitlement('overlay')
    try {
      settings.obsUrl = url
      if (password !== undefined && password !== '***') settings.obsPassword = password
      saveSettings()
      const status = await obsClient.connect(url, password === undefined || password === '***' ? settings.obsPassword : password)
      logger.info(`[obs] connected url=${url}`)
      return { ok: true, message: status.message }
    } catch (error) {
      logger.warn('[obs] connection failed', error instanceof Error ? error.message : String(error))
      return { ok: false, message: error instanceof Error ? error.message : String(error) }
    }
  })
  handle('obs:command', async (_event, command: string, args?: Record<string, string>) => {
    requireEntitlement('overlay')
    try {
      await obsClient.command(command, args)
      if (command === 'StartVirtualCam') logger.info('[obs] virtual camera started')
      if (command === 'StopVirtualCam') logger.info('[obs] virtual camera stopped')
      return { ok: true, message: obsClient.status().message }
    } catch (error) {
      return { ok: false, message: error instanceof Error ? error.message : String(error) }
    }
  })
  handle('obs:startVirtualCamera', async () => {
    requireEntitlement('overlay')
    try {
      if (!obsClient.status().connected) await obsClient.connect(settings.obsUrl, settings.obsPassword)
      const status = await obsClient.startVirtualCamera()
      logger.info('[obs] virtual camera started')
      return { ok: true, message: status.message }
    } catch (error) {
      return { ok: false, message: error instanceof Error ? error.message : String(error) }
    }
  })
  handle('obs:stopVirtualCamera', async () => {
    requireEntitlement('overlay')
    try {
      const status = await obsClient.stopVirtualCamera()
      logger.info('[obs] virtual camera stopped')
      return { ok: true, message: status.message }
    } catch (error) {
      return { ok: false, message: error instanceof Error ? error.message : String(error) }
    }
  })
  handle('obs:status', () => obsClient.status())

  handle('features:test', async (_event, featureId: FeatureId) => {
    requireEntitlement(featureEntitlement(featureId))
    const config = settings.features[featureId]
    if (!config?.enabled) return { ok: false, message: '请先启用此功能' }
    if (featureId === 'danmaku-assistant') {
      const keyword = featureValue(config, 'keywords', '666, 欧皇').split(/[,，]/).map((item) => item.trim()).find(Boolean) ?? '666'
      featureCooldowns.delete(featureId)
      const event: LiveEvent = { id: randomUUID(), source: 'simulator', kind: 'chat', user: { name: '测试观众' }, text: keyword, timestamp: Date.now() }
      await runFeatureRules(event)
      return { ok: true, message: `已测试关键词回复：${keyword}` }
    }
    if (featureId === 'voice-broadcast') {
      const event: LiveEvent = { id: randomUUID(), source: 'simulator', kind: 'chat', user: { name: '测试观众' }, text: '这是一条测试弹幕', timestamp: Date.now() }
      await executeFeature(featureId, config, event, '触发')
      return { ok: true, message: '已发送测试语音播报' }
    }
    if (featureId === 'virtual-camera') {
      if (!obsClient.status().connected) await obsClient.connect(settings.obsUrl, settings.obsPassword)
      const status = obsClient.status().virtualCameraActive
        ? await obsClient.stopVirtualCamera()
        : await obsClient.startVirtualCamera()
      return { ok: true, message: status.message }
    }
    const event: LiveEvent = {
      id: randomUUID(), source: 'simulator', kind: 'gift', user: { name: '功能测试' },
      gift: { name: '测试礼物', count: 1 }, timestamp: Date.now(),
    }
    await executeFeature(featureId, config, event, '触发')
    return { ok: true, message: `${featureId} 已在组件窗口/绿幕窗口执行` }
  })
  handle('features:show', async (_event, featureId: FeatureId) => {
    requireEntitlement(featureEntitlement(featureId))
    const config = settings.features[featureId]
    if (!config?.enabled) return { ok: false, message: '请先启用此功能' }
    const shown = await showFeaturePreview(featureId, config)
    return { ok: true, message: shown ? '组件已显示在 yapp 组件窗口' : '功能已启用，等待对应事件触发' }
  })

  handle('auth:status', async () => {
    if (auth.mode === 'remote' && auth.loggedIn) {
      try {
        const session = await licenseClient.status(settings.authServerUrl)
        auth.expiresAt = session.expiresAt
        auth.features = session.features
        auth.safeCodeSet = session.safeCodeSet
        authSessionExpiresAt = Date.parse(session.sessionExpiresAt)
      } catch {
        auth = { loggedIn: false, mode: 'remote', platform: auth.platform, features: [] }
        authSessionExpiresAt = 0
      }
    }
    return { ...auth, features: [...auth.features] }
  })
  handle('auth:login', async (_event, payload: { platform: Platform; code: string; local: boolean }) => {
    if (!payload || !SUPPORTED_PLATFORMS.includes(payload.platform) || typeof payload.local !== 'boolean') return { loggedIn: false, message: '登录参数无效' }
    authSessionExpiresAt = 0
    auth = { loggedIn: false, mode: 'remote', platform: payload.platform, features: [] }
    if (payload.local) {
      if (app.isPackaged) return { loggedIn: false, message: '正式版本不开放本地开发模式' }
      await licenseClient.logout().catch(() => undefined)
      authSessionExpiresAt = Number.POSITIVE_INFINITY
      auth = { loggedIn: true, mode: 'local', platform: payload.platform, features: ['overlay', 'slot', 'serial'] }
      logger.info(`[auth] local development mode platform=${payload.platform}`)
      return { loggedIn: true, message: '已进入本地开发模式' }
    }
    try {
      const session = await licenseClient.login(settings.authServerUrl, payload.code, payload.platform, installId)
      authSessionExpiresAt = Date.parse(session.sessionExpiresAt)
      auth = { loggedIn: true, mode: 'remote', platform: payload.platform, expiresAt: session.expiresAt, features: session.features, safeCodeSet: session.safeCodeSet }
      logger.info(`[auth] remote license accepted platform=${payload.platform} features=${session.features.join(',')}`)
      return { loggedIn: true, message: '卡密验证成功，已建立短期授权会话' }
    } catch (error) {
      logger.warn('[auth] remote license rejected', error instanceof Error ? error.message : 'unknown error')
      return { loggedIn: false, message: error instanceof Error ? error.message : '卡密验证失败' }
    }
  })
  handle('auth:logout', async () => {
    await licenseClient.logout().catch(() => undefined)
    authSessionExpiresAt = 0
    auth = { loggedIn: false, mode: 'remote', platform: auth.platform, features: [] }
    logger.info('[auth] logged out')
  })
  handle('auth:setSafeCode', async (_event, code: string) => {
    try {
      await licenseClient.setSafeCode(settings.authServerUrl, code)
      auth.safeCodeSet = true
      return { ok: true, message: '安全码已保存' }
    } catch (error) { return { ok: false, message: error instanceof Error ? error.message : '安全码设置失败' } }
  })
  handle('auth:unbind', async (_event, safeCode: string) => {
    try {
      await licenseClient.unbind(settings.authServerUrl, safeCode)
      auth = { loggedIn: false, mode: 'remote', platform: auth.platform, features: [] }
      authSessionExpiresAt = 0
      return { ok: true, message: '本机设备绑定已解除' }
    } catch (error) { return { ok: false, message: error instanceof Error ? error.message : '设备解绑失败' } }
  })
  handle('auth:adminStatus', () => licenseClient.adminStatus(settings.authServerUrl))
  handle('auth:adminLogin', async (_event, payload: { serverUrl: string; username: string; password: string }) => {
    const serverUrl = normalizeServerUrl(payload.serverUrl)
    const result = await licenseClient.adminLogin(serverUrl, payload.username, payload.password)
    settings.authServerUrl = serverUrl
    saveSettings()
    logger.info('[auth-admin] administrator logged in')
    return { ok: true, message: '管理员登录成功', expiresAt: result.expiresAt }
  })
  handle('auth:adminLogout', () => licenseClient.adminLogout())
  handle('auth:listCards', (_event, query: LicenseCardQuery) => licenseClient.listCards(settings.authServerUrl, query))
  handle('auth:createCards', (_event, input: LicenseCardCreateInput) => licenseClient.createCards(settings.authServerUrl, input))
  handle('auth:setCardStatus', (_event, id: string, status: Extract<LicenseCardStatus, 'active' | 'disabled' | 'revoked'>) => licenseClient.setCardStatus(settings.authServerUrl, id, status))
  handle('auth:resetCardBindings', (_event, id: string) => licenseClient.resetCardBindings(settings.authServerUrl, id))
  handle('auth:cardDetail', (_event, id: string) => licenseClient.cardDetail(settings.authServerUrl, id))
  handle('auth:extendCard', (_event, id: string, days: number) => licenseClient.extendCard(settings.authServerUrl, id, days))
  handle('auth:auditLog', (_event, limit?: number) => licenseClient.auditLog(settings.authServerUrl, limit))

  handle('config:export', () => exportConfig())
  handle('config:import', () => importConfig())
  handle('diagnostics:logs', (_event, limit?: number) => store.listLogs(limit))
  handle('diagnostics:assets', async () => {
    try { return await listAssets(settings.assetsRoot) } catch { return [] }
  })
  handle('diagnostics:settings', () => redactSettings())
  handle('diagnostics:saveSettings', (_event, patch: Partial<AppSettings>) => { settings = mergeSettings({ ...settings, ...patch }); saveSettings(); return redactSettings() })
  handle('window:minimize', () => mainWindow?.minimize())
  handle('window:maximize', () => { if (!mainWindow) return; if (mainWindow.isMaximized()) mainWindow.unmaximize(); else mainWindow.maximize() })
  handle('window:close', () => mainWindow?.close())
}

async function handleEvent(event: LiveEvent): Promise<void> {
  const recordId = store.insertRecord(toRecord(event))
  requestPersist()
  const initial = { ...toRecord(event), id: recordId }
  push('danmaku:append', initial)
  logger.info(`[${event.source}] ${event.kind} user=${event.user?.name ?? '匿名用户'} text=${event.text ?? event.gift?.name ?? ''}`)
  const rules = await store.listRules()
  const outcome = await engine.process(event, rules)
  await runFeatureRules(event)
  store.updateRecordResult(recordId, {
    matchedRuleId: outcome.ruleId,
    matchedRuleName: outcome.ruleName,
    actionResult: outcome.result,
    failReason: outcome.reason,
  })
  const finished = { ...initial, matchedRuleId: outcome.ruleId, matchedRuleName: outcome.ruleName, actionResult: outcome.result, failReason: outcome.reason }
  push('danmaku:append', finished)
  requestPersist()
}

async function runFeatureRules(event: LiveEvent): Promise<void> {
  for (const [featureId, config] of Object.entries(settings.features) as Array<[FeatureId, FeatureConfig]>) {
    if (!config?.enabled || !isAuthorized(featureEntitlement(featureId))) continue
    try {
      if (event.kind === 'gift' && event.gift?.name) {
        const giftName = event.gift.name.trim().toLocaleLowerCase()
        const enabledRules = config.giftRules?.filter((item) => item.enabled) ?? []
        const rule = enabledRules.find((item) => item.giftName.trim() !== '*' && item.giftName.trim().toLocaleLowerCase() === giftName)
          ?? enabledRules.find((item) => item.giftName.trim() === '*')
        if (rule) {
          await executeFeature(featureId, config, event, rule.action, rule.values)
          logger.info(`[feature:${featureId}] gift=${event.gift.name} action=${rule.action}`)
        }
      }
      if (event.kind === 'chat' && event.text?.trim()) {
        if (featureId === 'danmaku-assistant') await runDanmakuAssistant(config, event)
        if (featureId === 'voice-broadcast' && featureValue(config, 'chatEnabled', true)) await executeFeature(featureId, config, event, '触发')
      }
    } catch (error) {
      logger.error(`[feature:${featureId}] 执行失败`, error instanceof Error ? error.message : String(error))
    }
  }
}

async function runDanmakuAssistant(config: FeatureConfig, event: LiveEvent): Promise<void> {
  const keywords = splitFeatureList(featureValue(config, 'keywords', '666, 欧皇'))
  const mode: string = featureValue(config, 'keywordMode', 'contains')
  const text = event.text?.trim() ?? ''
  const matched = keywords.some((keyword) => mode === 'exact' ? text.toLocaleLowerCase() === keyword.toLocaleLowerCase() : text.toLocaleLowerCase().includes(keyword.toLocaleLowerCase()))
  if (!matched) return
  const featureId: FeatureId = 'danmaku-assistant'
  const now = Date.now()
  const cooldownMs = featureValue(config, 'cooldownMs', 3000)
  if (now - (featureCooldowns.get(featureId) ?? 0) < cooldownMs) return
  featureCooldowns.set(featureId, now)
  await executeFeature(featureId, config, event, '触发')
}

async function showFeaturePreview(featureId: FeatureId, config: FeatureConfig): Promise<boolean> {
  if (windows.hasEventWidget(featureId)) {
    await windows.open('slot')
    return true
  }
  const values = config.values
  switch (featureId) {
    case 'speed-curve':
    case 'speed-iba':
      await windows.widget({ featureId, kind: 'acceleration', title: featureId === 'speed-curve' ? '加速度 · 曲线队列' : '加速度 · IB 批处理', values, data: { preview: true } })
      return true
    case 'frying-pan': {
      const maxValue = featureValue(config, 'maxValue', 100)
      const value = typeof values.currentValue === 'number' ? values.currentValue : maxValue
      await windows.widget({ featureId, kind: 'health', title: featureValue(config, 'displayContent', '煮播血条'), values, data: { preview: true, value, maxValue } })
      return true
    }
    case 'impact-gift':
      await windows.widget({ featureId, kind: 'notice', title: '砸礼物', values, data: { preview: true, text: '组件已启用 · 礼物效果将在绿幕窗口播放' } })
      return true
    case 'danmaku-assistant':
      await windows.widget({ featureId, kind: 'reply', title: '弹幕助手', values, data: { preview: true, text: featureValue(config, 'replyTemplate', '收到 {user}') } })
      return true
    case 'live-clock':
      await windows.widget({ featureId, kind: 'timer', title: featureValue(config, 'displayContent', '直播倒计时'), values, data: { preview: true, seconds: featureValue(config, 'durationSeconds', 60) } })
      return true
    case 'electronic-woodfish': {
      const count = typeof values.currentMerit === 'number' ? values.currentMerit : 0
      await windows.widget({ featureId, kind: 'woodfish', title: '电子木鱼', values, data: { preview: true, count } })
      return true
    }
    case 'lottery':
      await windows.widget({ featureId, kind: 'wheel', title: '大转盘', values, data: { preview: true } })
      return true
    case 'counter': {
      const value = typeof values.currentValue === 'number' ? values.currentValue : featureValue(config, 'initialValue', 0)
      await windows.widget({ featureId, kind: 'counter', title: featureValue(config, 'displayContent', '礼物计数'), values, data: { preview: true, value } })
      return true
    }
    case 'gift-screen':
      await windows.widget({ featureId, kind: 'notice', title: '礼物飘屏', values, data: { preview: true, text: '组件已启用 · 礼物素材将在绿幕窗口飘屏' } })
      return true
    case 'gift-pool': {
      const selected = splitFeatureList(featureValue(config, 'pool', '啤酒, 小心心, 平底锅'))[0] ?? '等待礼物'
      const isImage = /\.(png|jpe?g|gif|webp|svg)$/i.test(selected)
      await windows.widget({ featureId, kind: 'sticker', title: '礼物咖', values, data: { preview: true, sticker: isImage ? basename(selected) : selected, imagePath: isImage ? pathToFileURL(resolveAssetPath(selected)).href : '' } })
      return true
    }
    case 'screen-lock':
      await windows.widget({ featureId, kind: 'lock', title: '屏幕锁键', values, data: { preview: true, locked: false } })
      return true
    case 'mosquito-slap':
      await windows.widget({ featureId, kind: 'mosquito', title: '拍蚊子', values: { ...values, imagePath: pathToFileURL(resolveAssetPath(featureValue(config, 'imagePath', 'idle.png'))).href }, data: { preview: true, durationMs: featureValue(config, 'durationMs', 20000), score: 0 } })
      return true
    default:
      return false
  }
}

async function executeFeature(featureId: FeatureId, config: FeatureConfig, event: LiveEvent, ruleAction: string, ruleValues?: Record<string, FeatureValue>): Promise<void> {
  requireEntitlement(featureEntitlement(featureId))
  const activeConfig: FeatureConfig = ruleValues ? { ...config, values: { ...config.values, ...ruleValues } } : config
  const persistValue = (key: string, value: FeatureValue): void => {
    config.values[key] = value
    activeConfig.values[key] = value
  }
  const giftCount = Math.min(100, Math.max(1, event.gift?.count ?? 1))
  switch (featureId) {
    case 'green-window': {
      await windows.updateSettings('green', { background: featureValue(activeConfig, 'backgroundColor', '#00ff00'), laneCount: featureValue(activeConfig, 'laneCount', 3), alwaysOnTop: featureValue(activeConfig, 'alwaysOnTop', true) })
      await windows.open('green')
      const videoPath = featureValue(activeConfig, 'videoPath', '').trim()
      if (videoPath) await windows.playVideo({ path: resolveAssetPath(videoPath), durationMs: featureValue(activeConfig, 'videoDurationMs', 8000), loop: featureValue(activeConfig, 'videoLoop', false) })
      break
    }
    case 'component-window':
      await windows.updateSettings('slot', { width: featureValue(activeConfig, 'width', 960), height: featureValue(activeConfig, 'height', 540), alwaysOnTop: featureValue(activeConfig, 'alwaysOnTop', true) })
      await windows.open('slot')
      break
    case 'virtual-camera':
      if (!obsClient.status().connected) await obsClient.connect(settings.obsUrl, settings.obsPassword)
      await obsClient.startVirtualCamera()
      break
    case 'speed-curve':
    case 'speed-iba':
      await windows.widget({
        featureId, kind: 'acceleration', title: featureId === 'speed-curve' ? '加速度 · 曲线队列' : '加速度 · IB 批处理',
        values: activeConfig.values,
        data: { targetCount: Math.min(9999, featureValue(activeConfig, 'targetCount', 10) * giftCount), giftCount },
      })
      break
    case 'impact-gift': {
      const count = Math.min(100, featureValue(activeConfig, 'count', 8) * giftCount)
      await windows.drop({ image: resolveAssetPath(featureValue(activeConfig, 'imagePath', 'images/平底锅.png')), count, gravity: featureValue(activeConfig, 'gravity', 1800), bounce: featureValue(activeConfig, 'bounce', 0.45), durationMs: 6000 })
      await windows.widget({ featureId, kind: 'notice', title: '砸礼物', values: activeConfig.values, data: { text: `${event.gift?.name ?? '礼物'} × ${count} · 已发送到绿幕窗口` } })
      break
    }
    case 'frying-pan': {
      const maxValue = featureValue(activeConfig, 'maxValue', 100)
      const current = typeof config.values.currentValue === 'number' ? config.values.currentValue : maxValue
      const change = featureValue(activeConfig, 'damagePerGift', 10) * giftCount * (ruleAction === '减少' ? -1 : 1)
      const value = Math.min(maxValue, Math.max(0, current + change))
      persistValue('currentValue', value)
      saveSettings()
      await windows.widget({ featureId, kind: 'health', title: featureValue(activeConfig, 'displayContent', '煮播血条'), values: activeConfig.values, data: { value, maxValue } })
      break
    }
    case 'voice-broadcast': {
      const isChat = event.kind === 'chat'
      const template = featureValue(activeConfig, isChat ? 'chatTemplate' : 'template', isChat ? '{user} 说 {text}' : '{user} 送出 {gift}，数量 {count}')
      const text = formatFeatureTemplate(template, event)
      await windows.widget({ featureId, kind: 'speech', title: '语音播报', values: activeConfig.values, data: { text, volume: featureValue(activeConfig, 'volume', 0.8), interrupt: featureValue(activeConfig, 'interrupt', false) } })
      break
    }
    case 'danmaku-assistant': {
      const reply = formatFeatureTemplate(featureValue(activeConfig, 'replyTemplate', '收到 {user}'), event)
      await windows.widget({ featureId, kind: 'reply', title: '弹幕助手 · 本地回复预览', values: activeConfig.values, data: { text: reply, user: event.user?.name ?? '观众' } })
      logger.info(`[feature:danmaku-assistant] local reply preview: ${reply}`)
      break
    }
    case 'live-clock': {
      const action = event.kind === 'gift' && ruleAction === '增加' ? 'add' : event.kind === 'gift' && ruleAction === '减少' ? 'subtract' : 'start'
      await windows.widget({
        featureId, kind: 'timer', title: featureValue(activeConfig, 'displayContent', '直播倒计时'), values: activeConfig.values,
        data: { action, seconds: featureValue(activeConfig, 'durationSeconds', 60), deltaSeconds: featureValue(activeConfig, 'secondsPerGift', 10) * giftCount },
      })
      break
    }
    case 'electronic-woodfish': {
      const previous = typeof config.values.currentMerit === 'number' ? config.values.currentMerit : 0
      const perGift = featureValue(activeConfig, 'meritPerGift', featureValue(activeConfig, 'meritPerClick', 1))
      const count = event.kind === 'gift' ? previous + perGift * giftCount : previous
      if (event.kind === 'gift') { persistValue('currentMerit', count); saveSettings() }
      await windows.widget({ featureId, kind: 'woodfish', title: '电子木鱼', values: activeConfig.values, data: { count } })
      break
    }
    case 'slot-machine':
      await runFeatureSlot(activeConfig)
      break
    case 'gift-screen':
      await windows.drop({ image: resolveAssetPath(featureValue(activeConfig, 'imagePath', 'images/平底锅.png')), count: giftCount, durationMs: featureValue(activeConfig, 'durationMs', 3500), gravity: 500, bounce: 0.2, maxVisible: featureValue(activeConfig, 'maxVisible', 20) })
      await windows.widget({ featureId, kind: 'notice', title: '礼物飘屏', values: activeConfig.values, data: { text: `${event.gift?.name ?? '礼物'} × ${giftCount} · 已发送到绿幕窗口` } })
      break
    case 'lottery':
      await windows.widget({ featureId, kind: 'wheel', title: '大转盘', values: activeConfig.values })
      break
    case 'counter': {
      const current = typeof config.values.currentValue === 'number' ? config.values.currentValue : featureValue(activeConfig, 'initialValue', 0)
      const direction = ruleAction === '减少' ? -1 : 1
      const value = Math.max(0, current + featureValue(activeConfig, 'step', 1) * giftCount * direction)
      persistValue('currentValue', value)
      saveSettings()
      await windows.widget({ featureId, kind: 'counter', title: featureValue(activeConfig, 'displayContent', '礼物计数'), values: activeConfig.values, data: { value } })
      break
    }
    case 'gift-pool': {
      const pool = splitFeatureList(featureValue(activeConfig, 'pool', '啤酒, 小心心, 平底锅'))
      if (!pool.length) break
      let index = Math.floor(Math.random() * pool.length)
      if (!featureValue(activeConfig, 'randomize', true)) {
        index = Number(config.values.nextStickerIndex ?? 0) % pool.length
        persistValue('nextStickerIndex', (index + 1) % pool.length)
        saveSettings()
      }
      const selected = pool[index]
      const isImage = /\.(png|jpe?g|gif|webp|svg)$/i.test(selected)
      await windows.widget({
        featureId, kind: 'sticker', title: '礼物咖', values: activeConfig.values,
        data: { sticker: isImage ? basename(selected) : selected, imagePath: isImage ? pathToFileURL(resolveAssetPath(selected)).href : '', durationMs: featureValue(activeConfig, 'durationMs', 3500) },
      })
      break
    }
    case 'screen-lock':
      await windows.widget({ featureId, kind: 'lock', title: '屏幕锁键', values: activeConfig.values, data: { locked: true } })
      break
    case 'mosquito-slap':
      await windows.widget({
        featureId, kind: 'mosquito', title: '拍蚊子',
        values: { ...activeConfig.values, imagePath: pathToFileURL(resolveAssetPath(featureValue(activeConfig, 'imagePath', 'idle.png'))).href },
        data: { durationMs: featureValue(activeConfig, 'durationMs', 20000), score: 0 },
      })
      break
  }
}

async function runFeatureSlot(config: FeatureConfig): Promise<void> {
  const pool = splitFeatureList(featureValue(config, 'pool', '一等奖, 二等奖, 谢谢参与'))
  const weights = parseFeatureWeights(featureValue(config, 'weights', '1, 10, 89'), pool.length)
  const audioPath = featureValue(config, 'playMusic', true) ? resolveAssetPath('fruit.mp3') : undefined
  await windows.slot({
    theme: featureValue(config, 'theme', 'default'),
    pool: pool.length ? pool : ['谢谢参与'],
    weights,
    images: Array.from({ length: 14 }, (_, index) => resolveAssetPath(`水果机/${index + 1}.png`)),
    durationMs: featureValue(config, 'durationMs', 2200),
    audioPath,
  })
}

function splitFeatureList(value: string): string[] { return value.split(/[,，]/).map((item) => item.trim()).filter(Boolean) }

function parseFeatureWeights(value: string, length: number): number[] {
  const parsed = splitFeatureList(value).map((item) => Number(item)).map((item) => Number.isFinite(item) && item >= 0 ? item : 0)
  const values = Array.from({ length }, (_, index) => parsed[index] ?? 1)
  return values.some((item) => item > 0) ? values : values.map(() => 1)
}

function formatFeatureTemplate(template: string, event: LiveEvent): string {
  return template
    .replaceAll('{user}', event.user?.name ?? '观众')
    .replaceAll('{text}', event.text ?? '')
    .replaceAll('{gift}', event.gift?.name ?? '弹幕')
    .replaceAll('{count}', String(event.gift?.count ?? event.count ?? 1))
}

function featureValue<T extends string | number | boolean>(config: FeatureConfig, key: string, fallback: T): T {
  const value = config.values[key]
  return typeof value === typeof fallback ? value as T : fallback
}

function toRecord(event: LiveEvent): Omit<DanmakuRecord, 'id'> {
  return {
    source: event.source,
    roomId: event.roomId,
    kind: event.kind,
    userId: event.user?.id,
    userName: event.user?.name,
    text: event.text,
    giftName: event.gift?.name,
    giftCount: event.gift?.count,
    giftValue: event.gift?.value,
    repeatCount: event.kind === 'like' ? event.count : undefined,
    ts: event.timestamp,
    createdAt: Date.now(),
    actionResult: 'none',
    rawJson: settings.storeRaw ? JSON.stringify(event.raw ?? {}) : undefined,
  }
}

function normalizeRule(rule: Rule): Rule {
  return {
    ...rule,
    id: rule.id || randomUUID(),
    name: rule.name.trim() || '未命名玩法',
    priority: Number.isFinite(rule.priority) ? rule.priority : 0,
    trigger: { ...rule.trigger, kinds: rule.trigger.kinds?.length ? rule.trigger.kinds : ['gift'] },
    concurrency: rule.concurrency ?? 'queue',
    actions: rule.actions ?? [],
  }
}

function resolveActionPaths(action: Action): Action {
  if (action.kind === 'video' || action.kind === 'audio') return { ...action, path: resolveAssetPath(action.path) }
  if (action.kind === 'drop') return { ...action, image: resolveAssetPath(action.image) }
  return action
}

function resolveAssetPath(input: string): string {
  const root = resolve(settings.assetsRoot)
  const candidate = resolve(isAbsolute(input) ? input : join(root, input))
  if (candidate !== root && !candidate.startsWith(`${root}${sep}`)) throw new Error('素材路径必须位于素材目录内')
  return candidate
}

function defaultAssetsRoot(): string {
  const candidates = app.isPackaged
    ? [join(process.resourcesPath, 'resources')]
    : [join(app.getAppPath(), 'resources'), join(app.getAppPath(), '..', 'resources'), join(__dirname, '..', '..', 'resources')]
  return candidates.find((candidate) => existsSync(candidate)) ?? candidates[0]
}

function normalizeAssetsRoot(input: string): string {
  const candidate = input ? (isAbsolute(input) ? input : resolve(app.getAppPath(), input)) : defaultAssetsRoot()
  return existsSync(candidate) ? candidate : defaultAssetsRoot()
}

async function exportRecords(filter: DanmakuFilter, format: 'json' | 'csv' | 'txt'): Promise<{ path?: string; content?: string }> {
  const records = store.queryRecords({ ...filter, limit: 500 })
  const content = format === 'json' ? JSON.stringify(records, null, 2) : format === 'csv' ? toCsv(records) : records.map(formatRecordText).join('\n')
  const result = await dialog.showSaveDialog(mainWindow!, { defaultPath: join(app.getPath('documents'), `danmaku-${Date.now()}.${format}`), filters: [{ name: format.toUpperCase(), extensions: [format] }] })
  if (result.canceled || !result.filePath) return { content }
  await writeFile(result.filePath, content, 'utf8')
  logger.info(`[danmaku:export] ${format} rows=${records.length}`)
  return { path: result.filePath }
}

async function exportConfig(): Promise<{ ok: boolean; path?: string; message: string }> {
  const config = await buildExportConfig()
  const result = await dialog.showSaveDialog(mainWindow!, { defaultPath: join(app.getPath('documents'), 'abi-replica-config.json'), filters: [{ name: 'JSON', extensions: ['json'] }] })
  if (result.canceled || !result.filePath) return { ok: false, message: '已取消导出' }
  if (existsSync(result.filePath)) await writeFile(`${result.filePath}.bak`, await readFile(result.filePath))
  await writeFile(result.filePath, JSON.stringify(config, null, 2), 'utf8')
  logger.info(`[config] exported ${basename(result.filePath)}`)
  return { ok: true, path: result.filePath, message: '配置导出成功' }
}

async function importConfig(): Promise<{ ok: boolean; message: string; rules?: Rule[] }> {
  const result = await dialog.showOpenDialog(mainWindow!, { properties: ['openFile'], filters: [{ name: 'JSON', extensions: ['json'] }] })
  if (result.canceled || !result.filePaths[0]) return { ok: false, message: '已取消导入' }
  let parsed: Partial<ExportConfig>
  try { parsed = JSON.parse(await readFile(result.filePaths[0], 'utf8')) as Partial<ExportConfig> } catch { return { ok: false, message: '配置不是有效 JSON' } }
  if (parsed.schema_version !== 1 || !Array.isArray(parsed.rules)) return { ok: false, message: '不支持的配置版本或缺少 rules' }
  for (const rule of parsed.rules) await store.saveRule(normalizeRule(rule))
  if (parsed.overlay?.green) {
    settings.overlayGreen = mergeSettings({ overlayGreen: parsed.overlay.green }).overlayGreen
    windows.updateSettings('green', settings.overlayGreen)
  }
  if (parsed.overlay?.slot) {
    settings.overlaySlot = mergeSettings({ overlaySlot: parsed.overlay.slot }).overlaySlot
    windows.updateSettings('slot', settings.overlaySlot)
  }
  if (parsed.connectors?.platform) {
    settings.platform = parsed.connectors.platform
    settings.roomId = parsed.connectors.roomId ?? settings.roomId
  }
  if (parsed.danmaku) settings = mergeSettings({ ...settings, retentionDays: parsed.danmaku.retention_days ?? settings.retentionDays, retentionMaxRows: parsed.danmaku.retention_max_rows ?? settings.retentionMaxRows, storeRaw: parsed.danmaku.store_raw ?? settings.storeRaw })
  if (parsed.features) settings.features = createDefaultFeatureSettings(parsed.features)
  saveSettings()
  requestPersist()
  logger.info(`[config] imported ${parsed.rules.length} rules`)
  return { ok: true, message: `已导入 ${parsed.rules.length} 条规则`, rules: await store.listRules() }
}

async function buildExportConfig(): Promise<ExportConfig> {
  return {
    schema_version: 1,
    app_version: APP_VERSION,
    rules: await store.listRules(),
    assets: await listAssets(settings.assetsRoot),
    overlay: { green: settings.overlayGreen, slot: settings.overlaySlot },
    slot: { theme: 'default' },
    connectors: { platform: settings.platform, roomId: settings.roomId },
    danmaku: { retention_days: settings.retentionDays, retention_max_rows: settings.retentionMaxRows, store_raw: settings.storeRaw },
    features: settings.features,
  }
}

async function listAssets(root: string): Promise<string[]> {
  root = normalizeAssetsRoot(root)
  const output: string[] = []
  async function walk(directory: string): Promise<void> {
    if (!existsSync(directory)) return
    for (const item of await readdir(directory, { withFileTypes: true })) {
      const full = join(directory, item.name)
      if (item.isDirectory()) await walk(full)
      else if (['.png', '.jpg', '.jpeg', '.bmp', '.gif', '.svg', '.mp3', '.wav', '.mp4'].includes(extname(item.name).toLowerCase())) output.push(relative(root, full))
    }
  }
  await walk(root)
  return output.sort()
}

function mergeSettings(input: Partial<AppSettings>): AppSettings {
  return {
    ...defaultSettings,
    ...input,
    overlayGreen: { ...defaultSettings.overlayGreen, ...(input.overlayGreen ?? {}) },
    overlaySlot: { ...defaultSettings.overlaySlot, ...(input.overlaySlot ?? {}) },
    features: createDefaultFeatureSettings(input.features),
  }
}

function isAuthorized(entitlement?: LicenseEntitlement): boolean {
  if (!auth.loggedIn) return false
  if (auth.mode === 'local') return !app.isPackaged
  if (Date.now() >= authSessionExpiresAt) return false
  return entitlement === undefined || auth.features.includes(entitlement)
}

function requireEntitlement(entitlement: LicenseEntitlement): void {
  if (!isAuthorized(entitlement)) throw new Error(`当前卡密未包含 ${entitlement} 权限，或授权会话已过期`)
}

function actionEntitlement(action: Action): LicenseEntitlement | undefined {
  switch (action.kind) {
    case 'video':
    case 'drop':
    case 'obs': return 'overlay'
    case 'slot':
    case 'audio': return 'slot'
    case 'serial': return 'serial'
    default: return undefined
  }
}

function saveSettings(): void { store.setSetting('settings', settings); requestPersist() }
function redactSettings(): AppSettings { return { ...settings, obsPassword: settings.obsPassword ? '***' : '' } }

function requestPersist(): void {
  if (persistTimer) clearTimeout(persistTimer)
  persistTimer = setTimeout(() => { persistTimer = undefined; void store.persist(dbPath) }, 250)
}

function push(channel: string, payload: unknown): void { if (mainWindow && !mainWindow.isDestroyed()) mainWindow.webContents.send(channel, payload) }

function handle(channel: string, listener: (event: Electron.IpcMainInvokeEvent, ...args: any[]) => any): void {
  ipcMain.handle(channel, async (event, ...args) => {
    const needsAuthorization = [
      'conn:connect', 'conn:simulate', 'features:', 'overlay:open', 'overlay:setMode', 'overlay:toggleOpacity',
      'overlay:updateSettings', 'overlay:playVideo', 'overlay:drop', 'overlay:slot', 'overlay:widget', 'audio:play',
      'input:', 'serial:pulse', 'obs:connect', 'obs:command', 'obs:startVirtualCamera', 'obs:stopVirtualCamera',
    ].some((prefix) => prefix.endsWith(':') ? channel.startsWith(prefix) : channel === prefix)
    if (needsAuthorization && !isAuthorized()) throw new Error('请使用有效卡密登录，或在开发版中进入本地模式')
    return listener(event, ...args)
  })
}

function toCsv(records: DanmakuRecord[]): string {
  const header = ['时间', '平台', '类型', '昵称', '内容', '命中规则', '结果']
  const rows = records.map((record) => [new Date(record.ts).toLocaleString('zh-CN'), record.source, record.kind, record.userName ?? '', record.text ?? record.giftName ?? '', record.matchedRuleName ?? '', record.actionResult])
  return [header, ...rows].map((row) => row.map((value) => `"${String(value).replaceAll('"', '""')}"`).join(',')).join('\n')
}

function formatRecordText(record: DanmakuRecord): string {
  return `[${new Date(record.ts).toLocaleTimeString('zh-CN')}] [${record.source}] ${record.userName ?? '匿名用户'} ${record.text ?? record.giftName ?? record.kind} ${record.actionResult}`
}

app.on('before-quit', () => { globalShortcut.unregisterAll(); obsClient.disconnect(); windows?.markQuitting(); windows?.destroyAll(); void store?.persist(dbPath) })
app.on('window-all-closed', () => { if (process.platform !== 'darwin') app.quit() })
app.on('activate', () => { if (!mainWindow && store) createMainWindow() })

// Electron's sandboxed renderer must not inherit permissive navigation or media policies.
app.whenReady().then(() => session.defaultSession.setPermissionRequestHandler((_webContents, _permission, callback) => callback(false))).catch(() => undefined)
