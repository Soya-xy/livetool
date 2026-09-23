import { BrowserWindow, screen } from 'electron'
import { join } from 'node:path'
import { pathToFileURL } from 'node:url'
import type { ChromaConfig, OverlaySettings, OverlayType, OverlayWidgetPayload, OverlayWindowMode, OverlayWindowStatus } from '@shared/types'
import type { FeatureId } from '@shared/features'

export type OverlayMessage = { target: OverlayType; type: string; payload: unknown }
type MessageListener = (message: OverlayMessage) => void
type StatusListener = (status: OverlayWindowStatus) => void

export class WindowManager {
  mainWindow?: BrowserWindow
  private greenWindow?: BrowserWindow
  private slotWindow?: BrowserWindow
  private audioWindow?: BrowserWindow
  private quitting = false
  private readonly loadPromises = new WeakMap<BrowserWindow, Promise<void>>()
  private readonly listeners = new Set<MessageListener>()
  private readonly statusListeners = new Set<StatusListener>()
  private readonly slotWidgets = new Map<FeatureId, OverlayWidgetPayload>()
  private readonly modes: Record<OverlayType, OverlayWindowMode> = { green: 'green', slot: 'landscape-16-9' }
  private readonly settings: Record<OverlayType, OverlaySettings> = {
    green: { visible: false, alwaysOnTop: true, opacity: 1, width: 960, height: 540, laneCount: 3, background: '#00ff00' },
    slot: { visible: false, alwaysOnTop: true, opacity: 1, width: 960, height: 540, laneCount: 1, background: '#000000' },
  }

  constructor(private readonly preloadPath: string) {}

  onMessage(listener: MessageListener): () => void { this.listeners.add(listener); return () => this.listeners.delete(listener) }

  onStatus(listener: StatusListener): () => void { this.statusListeners.add(listener); return () => this.statusListeners.delete(listener) }

  attachMainWindow(window: BrowserWindow): void { this.mainWindow = window }

  markQuitting(): void { this.quitting = true }

  getSettings(type: 'green' | 'slot'): OverlaySettings { return { ...this.settings[type] } }

  hasEventWidget(featureId: FeatureId): boolean {
    const payload = this.slotWidgets.get(featureId)
    return Boolean(payload && payload.data?.preview !== true)
  }

  getStatus(): OverlayWindowStatus[] { return ['green', 'slot'].map((type) => this.statusFor(type as OverlayType)) }

  updateSettings(type: 'green' | 'slot', patch: Partial<OverlaySettings>): OverlaySettings {
    this.settings[type] = { ...this.settings[type], ...patch }
    const target = type === 'green' ? this.greenWindow : this.slotWindow
    if (target && !target.isDestroyed()) {
      this.applySettings(type, target)
    }
    this.emitStatus(type)
    return this.getSettings(type)
  }

  async open(type: 'green' | 'slot'): Promise<void> {
    const existing = type === 'green' ? this.greenWindow : this.slotWindow
    const created = !existing || existing.isDestroyed()
    const window = this.ensureOverlay(type)
    await this.waitReady(window)
    this.settings[type].visible = true
    window.showInactive()
    this.applySettings(type, window)
    await window.webContents.executeJavaScript('window.dispatchEvent(new Event("overlay-ready"))').catch(() => undefined)
    if (type === 'slot' && created) {
      for (const payload of this.slotWidgets.values()) this.send('slot', 'component-widget', payload)
    }
    this.emitStatus(type)
  }

  async close(type: 'green' | 'slot'): Promise<void> {
    this.settings[type].visible = false
    const target = type === 'green' ? this.greenWindow : this.slotWindow
    if (target && !target.isDestroyed()) target.hide()
    this.emitStatus(type)
  }

  async setMode(type: OverlayType, mode: OverlayWindowMode): Promise<OverlayWindowStatus> {
    const window = this.ensureOverlay(type)
    await this.waitReady(window)
    this.modes[type] = mode
    if (mode === 'fullscreen') {
      window.setFullScreen(true)
    } else {
      if (window.isFullScreen()) window.setFullScreen(false)
      const size = this.sizeFor(type, mode)
      this.settings[type] = { ...this.settings[type], visible: true, width: size.width, height: size.height }
      window.setSize(size.width, size.height)
      window.center()
      this.applySettings(type, window)
    }
    this.settings[type].visible = true
    window.showInactive()
    this.emitStatus(type)
    return this.statusFor(type)
  }

  async toggleOpacity(type: OverlayType): Promise<OverlayWindowStatus> {
    const window = this.ensureOverlay(type)
    await this.waitReady(window)
    const opacity = this.settings[type].opacity <= 0.3 ? 1 : 0.25
    this.settings[type].opacity = opacity
    if (!window.isDestroyed()) window.setOpacity(opacity)
    this.emitStatus(type)
    return this.statusFor(type)
  }

  async playVideo(payload: { path: string; lane?: number; durationMs?: number; loop?: boolean; chroma?: ChromaConfig }): Promise<void> {
    await this.open('green')
    this.send('green', 'play-video', { ...payload, path: toMediaUrl(payload.path) })
  }

  async drop(payload: { image: string; count?: number; gravity?: number; bounce?: number; durationMs?: number; maxVisible?: number }): Promise<void> {
    await this.open('green')
    this.send('green', 'drop', { ...payload, image: toMediaUrl(payload.image) })
  }

  async slot(payload: { theme?: string; pool?: string[]; weights?: number[]; images?: string[]; durationMs?: number; audioPath?: string }): Promise<void> {
    await this.open('slot')
    this.send('slot', 'slot-start', { ...payload, images: payload.images?.map(toMediaUrl), audioPath: payload.audioPath ? toMediaUrl(payload.audioPath) : undefined })
  }

  async widget(payload: OverlayWidgetPayload): Promise<void> {
    const slotWindowWasAbsent = !this.slotWindow || this.slotWindow.isDestroyed()
    if (payload.kind !== 'speech') {
      this.slotWidgets.set(payload.featureId, {
        ...payload,
        values: { ...payload.values },
        data: payload.data ? { ...payload.data } : undefined,
      })
    }
    await this.open('slot')
    if (slotWindowWasAbsent && payload.kind !== 'speech') return
    this.send('slot', 'component-widget', payload)
  }

  removeWidget(featureId: FeatureId): void {
    this.slotWidgets.delete(featureId)
    const window = this.slotWindow
    if (window && !window.isDestroyed()) this.send('slot', 'component-remove', { featureId })
  }

  async playAudio(payload: { path: string; volume?: number; interrupt?: boolean; loop?: boolean }): Promise<void> {
    const window = this.ensureAudio()
    await this.waitReady(window)
    this.sendAudio('play-audio', { ...payload, path: toMediaUrl(payload.path) })
    if (window.isDestroyed()) this.audioWindow = undefined
  }

  async stopAudio(): Promise<void> {
    if (this.audioWindow && !this.audioWindow.isDestroyed()) await this.waitReady(this.audioWindow)
    this.sendAudio('stop-audio', {})
  }

  destroyAll(): void {
    for (const window of [this.greenWindow, this.slotWindow, this.audioWindow]) {
      if (window && !window.isDestroyed()) window.destroy()
    }
    this.greenWindow = undefined
    this.slotWindow = undefined
    this.audioWindow = undefined
  }

  private ensureOverlay(type: 'green' | 'slot'): BrowserWindow {
    const existing = type === 'green' ? this.greenWindow : this.slotWindow
    if (existing && !existing.isDestroyed()) return existing
    const value = this.settings[type]
    const display = screen.getPrimaryDisplay().workAreaSize
    const window = new BrowserWindow({
      width: value.width,
      height: value.height,
      x: Math.max(0, Math.round((display.width - value.width) / 2)),
      y: Math.max(0, Math.round((display.height - value.height) / 2)),
      title: this.titleFor(type),
      frame: true,
      transparent: false,
      backgroundColor: this.nativeBackground(type, value.background),
      resizable: true,
      minimizable: false,
      maximizable: false,
      alwaysOnTop: value.alwaysOnTop,
      skipTaskbar: false,
      show: false,
      opacity: value.opacity,
      autoHideMenuBar: true,
      webPreferences: this.webPreferences(),
    })
    window.setTitle(this.titleFor(type))
    window.setMenuBarVisibility(false)
    window.setSkipTaskbar(false)
    window.webContents.on('page-title-updated', (event) => {
      event.preventDefault()
      window.setTitle(this.titleFor(type))
    })
    window.on('closed', () => {
      this.settings[type].visible = false
      if (type === 'green') this.greenWindow = undefined
      else this.slotWindow = undefined
      this.emit({ target: type, type: 'window-closed', payload: {} })
      this.emitStatus(type)
    })
    window.on('resize', () => this.emitStatus(type))
    window.on('move', () => this.emitStatus(type))
    window.on('enter-full-screen', () => this.emitStatus(type))
    window.on('leave-full-screen', () => this.emitStatus(type))
    this.trackLoad(window, type === 'green' ? 'overlay-green' : 'overlay-slot')
    if (type === 'green') this.greenWindow = window
    else this.slotWindow = window
    return window
  }

  private ensureAudio(): BrowserWindow {
    if (this.audioWindow && !this.audioWindow.isDestroyed()) return this.audioWindow
    const window = new BrowserWindow({
      width: 2,
      height: 2,
      show: false,
      webPreferences: this.webPreferences(),
    })
    this.trackLoad(window, 'overlay-audio')
    this.audioWindow = window
    return window
  }

  private async load(window: BrowserWindow, hash: string): Promise<void> {
    const rendererUrl = process.env.ELECTRON_RENDERER_URL
    if (rendererUrl) await window.loadURL(`${rendererUrl}#/${hash}`)
    else await window.loadFile(join(__dirname, '../renderer/index.html'), { hash: `/${hash}` })
  }

  private trackLoad(window: BrowserWindow, hash: string): void {
    const loadPromise = this.load(window, hash).catch(() => undefined)
    this.loadPromises.set(window, loadPromise)
  }

  private async waitReady(window: BrowserWindow): Promise<void> {
    await this.loadPromises.get(window)
  }

  private webPreferences(): Electron.WebPreferences {
    return { preload: this.preloadPath, sandbox: true, contextIsolation: true, nodeIntegration: false }
  }

  private applySettings(type: OverlayType, window: BrowserWindow): void {
    const value = this.settings[type]
    window.setAlwaysOnTop(value.alwaysOnTop)
    window.setOpacity(Math.min(1, Math.max(0.1, value.opacity)))
    if (!window.isFullScreen()) window.setSize(Math.max(320, value.width), Math.max(240, value.height))
    window.setBackgroundColor(this.nativeBackground(type, value.background))
    if (value.visible) window.showInactive()
    else window.hide()
  }

  private sizeFor(type: OverlayType, mode: OverlayWindowMode): { width: number; height: number } {
    if (mode === 'green') return { width: this.settings[type].width || 960, height: this.settings[type].height || 540 }
    if (mode === 'landscape-4-3') return { width: 800, height: 600 }
    if (mode === 'portrait-9-16') return { width: 540, height: 960 }
    return { width: 960, height: 540 }
  }

  private statusFor(type: OverlayType): OverlayWindowStatus {
    const window = type === 'green' ? this.greenWindow : this.slotWindow
    const bounds = window && !window.isDestroyed() ? window.getBounds() : { width: this.settings[type].width, height: this.settings[type].height }
    return {
      type,
      role: type === 'green' ? 'ylm' : 'yapp',
      title: this.titleFor(type),
      visible: Boolean(window && !window.isDestroyed() && window.isVisible()),
      width: bounds.width,
      height: bounds.height,
      mode: this.modes[type],
      fullscreen: Boolean(window && !window.isDestroyed() && window.isFullScreen()),
      opacity: this.settings[type].opacity,
    }
  }

  private emitStatus(type: OverlayType): void {
    const status = this.statusFor(type)
    for (const listener of this.statusListeners) listener(status)
  }

  private titleFor(type: OverlayType): string {
    return type === 'green'
      ? '阿比整蛊 - 绿幕窗口 【禁止最小化】（按Tab键可以管理视频列表）'
      : '阿比整蛊 - 组件窗口 【禁止最小化】 快捷键切换透明度 Ctrl + F1'
  }

  private nativeBackground(type: OverlayType, color: string): string {
    if (type === 'green') return color === 'transparent' ? '#00ff00' : color
    return color === 'transparent' ? '#000000' : color
  }

  private send(target: OverlayType, type: string, payload: unknown): void {
    const window = target === 'green' ? this.greenWindow : this.slotWindow
    if (window && !window.isDestroyed()) window.webContents.send('overlay:message', { target, type, payload })
    this.emit({ target, type, payload })
  }

  private sendAudio(type: string, payload: unknown): void {
    if (this.audioWindow && !this.audioWindow.isDestroyed()) this.audioWindow.webContents.send('audio:message', { type, payload })
  }

  private emit(message: OverlayMessage): void { for (const listener of this.listeners) listener(message) }
}

function toMediaUrl(filePath: string): string {
  if (/^(file|https?):\/\//i.test(filePath)) return filePath
  return pathToFileURL(filePath).toString()
}
