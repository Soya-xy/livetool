<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import Vue3DraggableResizable from 'vue3-draggable-resizable'
import 'vue3-draggable-resizable/dist/Vue3DraggableResizable.css'
import ComponentWidgetBody from '../components/ComponentWidgetBody.vue'
import ScreenLockChain from '../components/ScreenLockChain.vue'
import type { FeatureValue, GiftMenu, GiftMenuItem } from '@shared/features'
import { readGiftMenus } from '@shared/features'
import type { OverlayWidgetPayload } from '@shared/types'
import { api } from '../services/api'

interface ActiveWidget extends OverlayWidgetPayload { data: Record<string, FeatureValue> }
interface AccelerationJob { total: number; completed: number; startedAt: number }
interface WidgetLayout { x: number; y: number; w: number; h: number }

const widgets = ref<ActiveWidget[]>([])
const speechNotice = ref('')
// 底板模式：true = 客户区完全透明；false = 显示不透明深色底板。
const backgroundTransparent = ref(true)
const chain = ref<InstanceType<typeof ScreenLockChain> | null>(null)
// 组件位置与大小：拖动 / 拉伸后写回设置，重开窗口或重启仍生效。
const layouts = ref<Record<string, WidgetLayout>>({})
const activeWidgetId = ref('')
// 性能显示：设置页打开后，组件窗角落显示帧率、最长帧与长帧次数。
const showPerf = ref(false)
// 只保留四角手柄：上下左右中键的细条既不好点，也容易误触拉伸。
const resizeHandles = ['tl', 'tr', 'bl', 'br']
const activeTimers = new Map<string, ReturnType<typeof setInterval>>()
const hideTimers = new Map<string, ReturnType<typeof setTimeout>>()
const accelerationQueues = new Map<string, AccelerationJob[]>()
let remove: (() => void) | undefined
let removeStatus: (() => void) | undefined
let speechTimer: ReturnType<typeof setTimeout> | undefined

// 屏幕锁键：锁链特效只在真正锁定时渲染，玩法预览不显示任何内容。
const lockWidget = computed(() => widgets.value.find((widget) => widget.kind === 'lock' && !widget.data.preview) ?? null)
const stageWidgets = computed(() => widgets.value.filter((widget) => widget.kind !== 'lock'))
const lockActive = computed(() => Boolean(lockWidget.value?.data.locked))
const lockLeaving = computed(() => Boolean(lockWidget.value?.data.unlocking))
const lockTitle = computed(() => String(lockWidget.value?.values.displayContent ?? '').trim() || '请按空格解锁')
const lockMessage = computed(() => String(lockWidget.value?.data.message ?? '').trim())
const lockColor = computed(() => String(lockWidget.value?.values.lockColor ?? '').trim() || '#e53935')
const lockOpenSound = computed(() => String(lockWidget.value?.values.lockOpenSound ?? '').trim())
const lockHitSound = computed(() => String(lockWidget.value?.values.lockHitSound ?? '').trim())
const lockVolume = computed(() => clampNumber(lockWidget.value?.values.lockSoundVolume, 0, 1, 0.8))
const lockBlur = computed(() => clampNumber(lockWidget.value?.values.lockBlurMax, 0, 40, 0))
const lockMedia = computed(() => {
  const widget = lockWidget.value
  if (!widget || widget.data.lockMediaFailed) return ''
  return lockMediaPath(widget)
})
const lockMediaVideo = computed(() => /\.(mp4|webm|mov|m4v)$/i.test(lockMedia.value))

function clampNumber(value: FeatureValue | undefined, min: number, max: number, fallback: number): number {
  const parsed = Number(value)
  return Number.isFinite(parsed) ? Math.max(min, Math.min(max, parsed)) : fallback
}

onMounted(() => {
  remove = api.overlay.onMessage((message) => {
    if (message.target !== 'slot') return
    if (message.type === 'component-widget') showWidget(message.payload as OverlayWidgetPayload)
    if (message.type === 'component-remove') removeWidget((message.payload as { featureId: ActiveWidget['featureId'] }).featureId)
    if (message.type === 'background-mode') backgroundTransparent.value = (message.payload as { transparent?: boolean }).transparent !== false
  })
  removeStatus = api.overlay.onStatus((status) => {
    if (status.type === 'slot') showPerf.value = Boolean(status.showPerf)
  })
  void api.overlay.ready('slot')
  void loadLayouts()
  window.addEventListener('keydown', handleUnlockKey)
  // 主进程推送可能早于订阅，这里再主动拉一次当前底板状态。
  void api.overlay.status()
    .then((list) => {
      const slot = list.find((item) => item.type === 'slot')
      backgroundTransparent.value = slot?.backgroundTransparent !== false
      showPerf.value = Boolean(slot?.showPerf)
    })
    .catch(() => undefined)
})

onBeforeUnmount(() => {
  remove?.()
  removeStatus?.()
  stopPerfSampler()
  window.removeEventListener('keydown', handleUnlockKey)
  for (const timer of activeTimers.values()) clearInterval(timer)
  for (const timer of hideTimers.values()) clearTimeout(timer)
  if (speechTimer) clearTimeout(speechTimer)
  accelerationQueues.clear()
  window.speechSynthesis?.cancel()
})

function showWidget(payload: OverlayWidgetPayload): void {
  const data = { ...(payload.data ?? {}) }
  if (payload.kind === 'speech') {
    const text = String(data.text ?? '').trim()
    if (text) speak(text, Number(data.volume ?? 0.8), Boolean(data.interrupt), String(payload.values.audioPath ?? ''))
    return
  }

  let widget = widgets.value.find((item) => item.featureId === payload.featureId)
  if (payload.kind === 'lock') {
    // 预览不再下发锁链组件（后端 showFeaturePreview 直接返回「未展示」），这里只处理真实锁定。
    if (data.locked && data.remainingPresses === undefined) data.remainingPresses = 1
    clearWidgetTimers(payload.featureId)
    if (!widget) {
      widget = { ...payload, data }
      widgets.value.push(widget)
    } else {
      widget.title = payload.title
      widget.values = { ...payload.values }
      widget.data = data
    }
    if (data.unlocking) hideAfter(widget, 850)
    return
  }
  if (payload.kind === 'acceleration') {
    if (!widget) {
      widget = { ...payload, data: { ...data, currentCount: 0, targetCount: 0, pendingCount: 0, progress: 0, batchCount: 0 } }
      widgets.value.push(widget)
      widget = widgets.value[widgets.value.length - 1]!
    } else {
      widget.title = payload.title
      widget.values = { ...payload.values }
    }
    if (data.preview) {
      clearWidgetTimers(payload.featureId)
      accelerationQueues.delete(String(payload.featureId))
      widget.data = { ...widget.data, ...data, pendingCount: 0, progress: 0 }
      return
    }
    widget.data.preview = false
    enqueueAcceleration(widget, Math.max(1, Number(data.targetCount ?? widget.values.targetCount ?? 10)))
    return
  }

  if (payload.kind === 'timer' && data.action === 'add' && widget) {
    widget.data.preview = false
    const seconds = Math.max(0, Number(data.deltaSeconds ?? 0))
    widget.data.remainingSeconds = Math.max(0, Number(widget.data.remainingSeconds ?? 0) + seconds)
    widget.data.totalSeconds = Math.max(1, Number(widget.data.totalSeconds ?? 0) + seconds)
    widget.data.completed = false
    restartTimer(widget)
    return
  }
  if (payload.kind === 'timer' && data.action === 'subtract' && widget) {
    widget.data.preview = false
    const seconds = Math.max(0, Number(data.deltaSeconds ?? 0))
    widget.data.remainingSeconds = Math.max(0, Number(widget.data.remainingSeconds ?? 0) - seconds)
    widget.data.completed = Number(widget.data.remainingSeconds) <= 0
    restartTimer(widget)
    return
  }

  // 倒计时：与加班时钟共用倒计时状态机，但使用自己的配色面板。
  if (payload.kind === 'countdown' && widget && data.action === 'add') {
    widget.data.preview = false
    const seconds = Math.max(0, Number(data.deltaSeconds ?? 0))
    widget.data.remainingSeconds = Math.max(0, Number(widget.data.remainingSeconds ?? 0) + seconds)
    widget.data.totalSeconds = Math.max(1, Number(widget.data.totalSeconds ?? 0) + seconds)
    widget.data.completed = false
    restartTimer(widget)
    return
  }
  if (payload.kind === 'countdown' && widget && data.action === 'subtract') {
    widget.data.preview = false
    const seconds = Math.max(0, Number(data.deltaSeconds ?? 0))
    widget.data.remainingSeconds = Math.max(0, Number(widget.data.remainingSeconds ?? 0) - seconds)
    widget.data.completed = Number(widget.data.remainingSeconds) <= 0
    restartTimer(widget)
    return
  }
  if (payload.kind === 'countdown' && data.action === 'stop') {
    if (widget) {
      clearWidgetTimers(payload.featureId)
      widget.data.preview = false
    }
    return
  }

  if (!widget) {
    widget = { ...payload, data }
    widgets.value.push(widget)
    widget = widgets.value[widgets.value.length - 1]!
  } else {
    widget.kind = payload.kind
    widget.title = payload.title
    widget.values = { ...payload.values }
    widget.data = data
  }

  if (payload.kind === 'timer') {
    const baseSeconds = Math.max(0, Number(data.seconds ?? widget.values.durationSeconds ?? 60))
    const delta = Math.max(0, Number(data.deltaSeconds ?? 0))
    const action = String(data.action ?? 'start')
    const seconds = action === 'add' ? baseSeconds + delta : action === 'subtract' ? Math.max(0, baseSeconds - delta) : baseSeconds
    widget.data.remainingSeconds = seconds
    widget.data.totalSeconds = Math.max(1, seconds)
    widget.data.completed = seconds <= 0
    if (data.preview) {
      clearWidgetTimers(payload.featureId)
      widget.data.preview = true
    } else {
      widget.data.preview = false
      restartTimer(widget)
    }
  }
  if (payload.kind === 'sticker' && !data.preview) hideAfter(widget, Number(data.durationMs ?? widget.values.durationMs ?? 3500))
  if (payload.kind === 'menu') void loadGiftIcons(widget)
  if (payload.kind === 'countdown') {
    const baseSeconds = Math.max(0, Number(data.seconds ?? widget.values.durationSeconds ?? 60))
    widget.data.remainingSeconds = baseSeconds
    widget.data.totalSeconds = Math.max(1, baseSeconds)
    widget.data.completed = baseSeconds <= 0
    widget.data.preview = Boolean(data.preview)
    if (data.preview) clearWidgetTimers(payload.featureId)
    else restartTimer(widget)
  }
  if (payload.kind === 'trash') {
    if (data.preview) {
      widget.data.items = Math.max(1, Number(widget.values.count ?? 1))
      widget.data.preview = true
    } else {
      const amount = Math.max(1, Number(data.amount ?? 1))
      const maxVisible = Math.max(1, Number(widget.values.maxVisible ?? 60))
      widget.data.items = Math.min(maxVisible, Number(widget.data.items ?? 0) + amount)
      widget.data.preview = false
      hideAfter(widget, Number(widget.values.durationMs ?? 4000))
    }
  }
}

function removeWidget(featureId: ActiveWidget['featureId']): void {
  clearWidgetTimers(featureId)
  accelerationQueues.delete(String(featureId))
  widgets.value = widgets.value.filter((widget) => widget.featureId !== featureId)
}

function clearWidgetTimers(featureId: ActiveWidget['featureId']): void {
  const key = String(featureId)
  const timer = activeTimers.get(key)
  if (timer) clearInterval(timer)
  activeTimers.delete(key)
  const hideTimer = hideTimers.get(key)
  if (hideTimer) clearTimeout(hideTimer)
  hideTimers.delete(key)
}

function restartTimer(widget: ActiveWidget): void {
  const key = String(widget.featureId)
  const old = activeTimers.get(key)
  if (old) clearInterval(old)
  if (Number(widget.data.remainingSeconds ?? 0) <= 0) {
    widget.data.remainingSeconds = 0
    widget.data.completed = true
    activeTimers.delete(key)
    return
  }
  const timer = setInterval(() => {
    const current = Number(widget.data.remainingSeconds ?? 0)
    if (current <= 1) {
      widget.data.remainingSeconds = 0
      widget.data.completed = true
      clearInterval(timer)
      activeTimers.delete(key)
    } else widget.data.remainingSeconds = current - 1
  }, 1000)
  activeTimers.set(key, timer)
}

function enqueueAcceleration(widget: ActiveWidget, count: number): void {
  const key = String(widget.featureId)
  const queue = accelerationQueues.get(key) ?? []
  queue.push({ total: count, completed: 0, startedAt: 0 })
  accelerationQueues.set(key, queue)
  const pending = queue.reduce((sum, job) => sum + job.total - job.completed, 0)
  widget.data.targetCount = Number(widget.data.currentCount ?? 0) + pending
  widget.data.pendingCount = pending
  if (activeTimers.has(key)) return

  const duration = Math.max(100, Number(widget.values.durationMs ?? 3000))
  const batchSize = Math.max(1, Number(widget.values.batchSize ?? 1))
  const isBatch = widget.featureId === 'speed-iba'
  const timer = setInterval(() => {
    const jobs = accelerationQueues.get(key)
    const job = jobs?.[0]
    if (!jobs || !job) {
      clearInterval(timer)
      activeTimers.delete(key)
      return
    }
    const now = performance.now()
    if (!job.startedAt) job.startedAt = now
    const elapsed = Math.min(1, (now - job.startedAt) / duration)
    const batchCount = Math.ceil(job.total / batchSize)
    const curve = String(widget.values.curve ?? 'ease-out')
    const progress = curve === 'linear' ? elapsed : curve === 'ease-in-out' ? elapsed * elapsed * (3 - 2 * elapsed) : 1 - (1 - elapsed) ** 3
    const desired = isBatch ? Math.min(job.total, Math.floor(progress * batchCount) * batchSize) : Math.floor(job.total * progress)
    const completed = elapsed >= 1 ? job.total : Math.max(job.completed, desired)
    const newlyCompleted = completed - job.completed
    job.completed = completed
    widget.data.currentCount = Number(widget.data.currentCount ?? 0) + newlyCompleted
    widget.data.batchCount = isBatch ? Math.ceil(Number(widget.data.currentCount) / batchSize) : 0
    widget.data.progress = elapsed * 100
    const pending = jobs.reduce((sum, item) => sum + item.total - item.completed, 0)
    widget.data.pendingCount = pending
    widget.data.targetCount = Number(widget.data.currentCount) + pending
    if (elapsed >= 1) {
      jobs.shift()
      if (jobs.length) jobs[0].startedAt = now
      else {
        accelerationQueues.delete(key)
        clearInterval(timer)
        activeTimers.delete(key)
        widget.data.progress = 100
      }
    }
  }, 40)
  activeTimers.set(key, timer)
}

// 礼物菜单：点一下菜单里的礼物，就等于收到该礼物（走规则引擎）。
const giftIcons = ref<Record<string, string>>({})

// 组件默认尺寸：按组件类型给一个合理的初始大小，之后以用户拖出来的为准。
const widgetDefaultSize: Record<string, { w: number; h: number }> = {
  menu: { w: 240, h: 280 },
  countdown: { w: 260, h: 200 },
  trash: { w: 260, h: 240 },
  timer: { w: 260, h: 180 },
  health: { w: 260, h: 150 },
  acceleration: { w: 260, h: 170 },
  reply: { w: 320, h: 150 },
  notice: { w: 320, h: 150 },
  sticker: { w: 220, h: 200 },
}

function layoutOf(widget: ActiveWidget): WidgetLayout {
  const key = String(widget.featureId)
  const saved = layouts.value[key]
  if (saved) return saved
  const size = widgetDefaultSize[widget.kind] ?? { w: 260, h: 180 }
  const index = stageWidgets.value.findIndex((item) => item.featureId === widget.featureId)
  return { x: 24 + Math.max(0, index) * 26, y: 56 + Math.max(0, index) * 26, w: size.w, h: size.h }
}

async function loadLayouts(): Promise<void> {
  try {
    layouts.value = await api.overlay.widgetLayouts()
  } catch {
    layouts.value = {}
  }
}

// 拉伸期间把内容容器钉在拉伸前的像素尺寸：内容在拉伸过程中完全不重排 / 不回流，
// 松手后再按新尺寸回流一次。实测（CDP Performance.getMetrics）拉伸 120 帧：
// 跟着重排 LayoutDuration 13.4 ms，冻结后 5.1 ms —— 拉伸比拖动卡的主因就是这里。
const resizeFreeze = ref<Record<string, { w: number; h: number }>>({})
const widgetPadding = 24

function frozenContentStyle(widget: ActiveWidget): Record<string, string> | undefined {
  const frozen = resizeFreeze.value[String(widget.featureId)]
  if (!frozen) return undefined
  return { width: `${frozen.w}px`, height: `${frozen.h}px`, flex: 'none' }
}

function beginResize(widget: ActiveWidget, position?: Partial<WidgetLayout>): void {
  const layout = layoutOf(widget)
  resizeFreeze.value = {
    ...resizeFreeze.value,
    [String(widget.featureId)]: {
      w: Math.max(1, Math.round(position?.w ?? layout.w) - widgetPadding),
      h: Math.max(1, Math.round(position?.h ?? layout.h) - widgetPadding),
    },
  }
}

function finishResize(widget: ActiveWidget, position: Partial<WidgetLayout>): void {
  const next = { ...resizeFreeze.value }
  delete next[String(widget.featureId)]
  resizeFreeze.value = next
  void saveLayout(widget, position)
}

// saveLayout 由 drag-end / resize-end 触发：事件里带的就是拖动后的实际位置与大小。
async function saveLayout(widget: ActiveWidget, position: Partial<WidgetLayout>): Promise<void> {
  const key = String(widget.featureId)
  const current = layouts.value[key] ?? layoutOf(widget)
  const next: WidgetLayout = {
    x: Math.round(position.x ?? current.x),
    y: Math.round(position.y ?? current.y),
    w: Math.round(position.w ?? current.w),
    h: Math.round(position.h ?? current.h),
  }
  layouts.value = { ...layouts.value, [key]: next }
  try {
    await api.overlay.saveWidgetLayout(key, next)
  } catch {
    // 保存失败只影响下次打开的位置，不打断组件本身。
  }
}

// 性能显示：每秒统计一次帧率、最长帧与长帧（>33ms）次数，开关在设置页。
const perfStats = ref({ fps: 0, maxFrame: 0, longFrames: 0 })
let perfRaf = 0
let perfTimer: ReturnType<typeof setInterval> | undefined
let perfSamples: number[] = []
let perfLastFrame = 0

function samplePerfFrame(time: number): void {
  if (!showPerf.value) {
    stopPerfSampler()
    return
  }
  if (perfLastFrame) perfSamples.push(time - perfLastFrame)
  perfLastFrame = time
  perfRaf = requestAnimationFrame(samplePerfFrame)
}

function startPerfSampler(): void {
  if (perfRaf) return
  perfSamples = []
  perfLastFrame = 0
  perfRaf = requestAnimationFrame(samplePerfFrame)
  perfTimer = setInterval(() => {
    const samples = perfSamples
    perfSamples = []
    if (!samples.length) {
      perfStats.value = { fps: 0, maxFrame: 0, longFrames: 0 }
      return
    }
    const total = samples.reduce((sum, value) => sum + value, 0)
    perfStats.value = {
      fps: Math.round(1000 / Math.max(1, total / samples.length)),
      maxFrame: Math.round(Math.max(...samples) * 10) / 10,
      longFrames: samples.filter((value) => value > 33).length,
    }
  }, 1000)
}

function stopPerfSampler(): void {
  if (perfRaf) cancelAnimationFrame(perfRaf)
  perfRaf = 0
  if (perfTimer) clearInterval(perfTimer)
  perfTimer = undefined
  perfLastFrame = 0
  perfSamples = []
}

watch(showPerf, (value) => {
  if (value) startPerfSampler()
  else stopPerfSampler()
}, { immediate: true })

function onWidgetDeactivated(featureId: string): void {
  if (activeWidgetId.value === featureId) activeWidgetId.value = ''
}

// 拖动 / 拉伸时 vue3-draggable-resizable 每帧都会重跑插槽渲染函数，所以传给
// 组件内容子组件的值必须是稳定引用（组件内容本身已经拆到 ComponentWidgetBody，
// 拖动时不再参与渲染）。这里缓存布局样式，礼物菜单与转盘参数在子组件里同样按
// widget.values 的身份缓存。
const giftMenuCache = new WeakMap<Record<string, FeatureValue>, GiftMenu[]>()
const widgetStyleCache = new WeakMap<Record<string, FeatureValue>, Record<string, string>>()

function widgetStyle(widget: ActiveWidget): Record<string, string> {
  const cached = widgetStyleCache.get(widget.values)
  if (cached) return cached
  const style = { '--widget-accent': String(widget.values.barColor ?? widget.values.lockColor ?? '#51c9dc') }
  widgetStyleCache.set(widget.values, style)
  return style
}

function activeMenus(widget: ActiveWidget): GiftMenu[] {
  const cached = giftMenuCache.get(widget.values)
  if (cached) return cached
  const menus = readGiftMenus(widget.values).filter((menu) => menu.enabled)
  giftMenuCache.set(widget.values, menus)
  return menus
}

function giftIcon(name: string): string {
  return giftIcons.value[name] ?? ''
}

async function loadGiftIcons(widget: ActiveWidget): Promise<void> {
  const names = new Set<string>()
  for (const menu of activeMenus(widget)) {
    for (const gift of menu.gifts) if (gift.giftName.trim()) names.add(gift.giftName.trim())
  }
  for (const name of names) {
    if (giftIcons.value[name] !== undefined) continue
    let url = ''
    try {
      url = await api.features.giftIcon(name)
    } catch {
      url = ''
    }
    giftIcons.value = { ...giftIcons.value, [name]: url }
  }
}

async function sendMenuGift(gift: GiftMenuItem): Promise<void> {
  const name = (gift.giftName || gift.title).trim()
  if (!name) return
  try {
    await api.features.menuGift(name)
  } catch {
    // 主进程会把失败写进日志，这里只保证界面不报错。
  }
}

function hideAfter(widget: ActiveWidget, duration: number): void {
  const key = String(widget.featureId)
  const old = hideTimers.get(key)
  if (old) clearTimeout(old)
  hideTimers.set(key, setTimeout(() => { removeWidget(widget.featureId) }, Math.max(100, duration)))
}

function handleUnlockKey(event: KeyboardEvent): void {
  const locked = widgets.value.find((widget) => widget.kind === 'lock' && widget.data.locked)
  if (!locked || locked.data.preview || locked.data.unlocking || event.code !== 'Space' || event.repeat) return
  // 主进程已用全局键盘钩子统计空格（只监听、不独占）时，页面不要再扣一次。
  if (locked.data.globalKey) return
  event.preventDefault()
  void api.overlay.decrementScreenLock().catch(() => undefined)
}

// 锁链次数减少时震动一次并播放打铁音效。
// 全局空格键由主进程的低级键盘钩子统计，按键不会进入页面，所以只能靠次数变化来触发反馈。
// 最后一次按键时次数已经归零，这里强制播一次打铁音效，保持与手动按键一致的听感。
watch(() => Number(lockWidget.value?.data.remainingPresses ?? -1), (next, previous) => {
  if (previous < 0 || next < 0 || next >= previous) return
  if (lockWidget.value?.data.preview) return
  chain.value?.triggerHit(true)
})

function speak(text: string, volume: number, interrupt: boolean, fallbackPath: string): void {
  speechNotice.value = text
  if (speechTimer) clearTimeout(speechTimer)
  speechTimer = setTimeout(() => { speechNotice.value = '' }, 5000)
  if (!('speechSynthesis' in window)) {
    if (fallbackPath) void api.audio.play({ path: fallbackPath, volume, interrupt })
    return
  }
  if (interrupt) window.speechSynthesis.cancel()
  const utterance = new SpeechSynthesisUtterance(text)
  utterance.lang = 'zh-CN'
  utterance.volume = Math.max(0, Math.min(1, volume))
  window.speechSynthesis.speak(utterance)
}

function lockMediaPath(widget: ActiveWidget): string {
  const value = widget.values.lockMediaPath
  return typeof value === 'string' ? value : ''
}

function markLockMediaFailed(): void {
  const widget = lockWidget.value
  if (widget) widget.data.lockMediaFailed = true
}
</script>

<template>
  <div class="slot-overlay" :class="{ 'panel-background': !backgroundTransparent }">
    <header class="component-status-bar"><b>yapp · 组件窗口</b><span>{{ widgets.length }} 个活动组件</span><span>屏幕锁键需按对应次数空格解锁</span></header>
    <div v-if="showPerf" class="perf-hud" aria-hidden="true">
      <b>{{ perfStats.fps }} FPS</b>
      <span>最长帧 {{ perfStats.maxFrame }} ms</span>
      <span>长帧 {{ perfStats.longFrames }} 次/秒</span>
      <span>{{ stageWidgets.length }} 个组件</span>
    </div>
    <div v-if="speechNotice" class="speech-notice" aria-live="polite">{{ speechNotice }}</div>
    <div v-if="!widgets.length" class="component-placeholder"><b>yapp</b><span>组件窗口已打开，等待玩法输出</span></div>
    <ScreenLockChain
      ref="chain"
      :active="lockActive"
      :leaving="lockLeaving"
      :count="Number(lockWidget?.data.remainingPresses ?? 0)"
      :title="lockTitle"
      :message="lockMessage"
      :color="lockColor"
      :open-sound="lockOpenSound"
      :hit-sound="lockHitSound"
      :volume="lockVolume"
      :blur-max="lockBlur"
    >
      <template v-if="lockMedia">
        <video v-if="lockMediaVideo" :key="lockMedia" :src="lockMedia" autoplay muted loop playsinline aria-hidden="true" @error="markLockMediaFailed" />
        <img v-else :key="lockMedia" :src="lockMedia" alt="" @error="markLockMediaFailed">
      </template>
    </ScreenLockChain>
    <section v-if="stageWidgets.length" class="widget-stage">
      <Vue3DraggableResizable
        v-for="widget in stageWidgets"
        :key="widget.featureId"
        class="widget-box"
        :class="[`widget-${widget.kind}`, { 'widget-box-active': activeWidgetId === String(widget.featureId) }]"
        :style="widgetStyle(widget)"
        :parent="true"
        :x="layoutOf(widget).x"
        :y="layoutOf(widget).y"
        :w="layoutOf(widget).w"
        :h="layoutOf(widget).h"
        :min-w="140"
        :min-h="90"
        :handles="resizeHandles"
        :active="activeWidgetId === String(widget.featureId)"
        @activated="activeWidgetId = String(widget.featureId)"
        @deactivated="onWidgetDeactivated(String(widget.featureId))"
        @drag-end="(position: WidgetLayout) => saveLayout(widget, position)"
        @resize-start="(position: WidgetLayout) => beginResize(widget, position)"
        @resize-end="(position: WidgetLayout) => finishResize(widget, position)"
      >
        <div class="widget-content" :style="frozenContentStyle(widget)">
          <ComponentWidgetBody
            :widget="widget"
            :gift-icon="giftIcon"
            :menu-gift="sendMenuGift"
          />
        </div>
      </Vue3DraggableResizable>
    </section>
  </div>
</template>
