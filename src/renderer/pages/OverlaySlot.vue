<script setup lang="ts">
import { onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import type { FeatureValue } from '@shared/features'
import type { OverlayWidgetPayload } from '@shared/types'
import { api } from '../services/api'

interface ActiveWidget extends OverlayWidgetPayload { data: Record<string, FeatureValue> }
interface SlotRun { theme: string; pool: string[]; weights: number[]; images: string[]; spinning: boolean; result: string }
interface AccelerationJob { total: number; completed: number; startedAt: number }

const widgets = ref<ActiveWidget[]>([])
const slotRun = ref<SlotRun | null>(null)
const speechNotice = ref('')
const fallbackFruit = ['🍺', '💖', '🎁', '🍀', '⭐', '🎈', '🌈', '🍉', '🧧', '💎', '🎯', '🪙', '🎉', '🏆']
const activeTimers = new Map<string, ReturnType<typeof setInterval>>()
const hideTimers = new Map<string, ReturnType<typeof setTimeout>>()
const accelerationQueues = new Map<string, AccelerationJob[]>()
let remove: (() => void) | undefined
let speechTimer: ReturnType<typeof setTimeout> | undefined
let slotTimer: ReturnType<typeof setTimeout> | undefined

onMounted(() => {
  remove = api.overlay.onMessage((message) => {
    if (message.target !== 'slot') return
    if (message.type === 'slot-start') startSlot(message.payload as { theme?: string; pool?: string[]; weights?: number[]; images?: string[]; durationMs?: number; audioPath?: string })
    if (message.type === 'component-widget') showWidget(message.payload as OverlayWidgetPayload)
    if (message.type === 'component-remove') removeWidget((message.payload as { featureId: ActiveWidget['featureId'] }).featureId)
  })
  window.addEventListener('keydown', handleUnlockKey)
})

onBeforeUnmount(() => {
  remove?.()
  window.removeEventListener('keydown', handleUnlockKey)
  for (const timer of activeTimers.values()) clearInterval(timer)
  for (const timer of hideTimers.values()) clearTimeout(timer)
  if (speechTimer) clearTimeout(speechTimer)
  if (slotTimer) clearTimeout(slotTimer)
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
  if (payload.kind === 'wheel') {
    if (data.preview) {
      widget.data.prizes = splitList(String(widget.values.pool ?? '一等奖, 二等奖, 谢谢参与')).join('、')
      widget.data.spinning = false
      widget.data.rotation = 0
      widget.data.result = '等待礼物触发'
    } else startWheel(widget)
  }
  if (payload.kind === 'sticker' && !data.preview) hideAfter(widget, Number(data.durationMs ?? widget.values.durationMs ?? 3500))
  if (payload.kind === 'mosquito' && !data.preview) startMosquito(widget)
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

function startSlot(payload: { theme?: string; pool?: string[]; weights?: number[]; images?: string[]; durationMs?: number; audioPath?: string }): void {
  const pool = payload.pool?.length ? payload.pool : ['一等奖', '二等奖', '谢谢参与']
  const weights = normalizeWeights(payload.weights ?? [], pool.length)
  const winningIndex = weightedIndex(weights)
  const run = reactive<SlotRun>({ theme: payload.theme ?? 'default', pool, weights, images: payload.images ?? [], spinning: true, result: '抽取中…' })
  slotRun.value = run
  const duration = Math.max(300, payload.durationMs ?? 2200)
  if (payload.audioPath) void api.audio.play({ path: payload.audioPath, volume: 0.8, interrupt: false })
  if (slotTimer) clearTimeout(slotTimer)
  slotTimer = setTimeout(() => {
    if (slotRun.value !== run) return
    run.spinning = false
    run.result = `恭喜抽取 ${pool[winningIndex]}`
    slotTimer = undefined
  }, duration)
}

function startWheel(widget: ActiveWidget): void {
  const pool = splitList(String(widget.values.pool ?? '一等奖, 二等奖, 谢谢参与'))
  const prizes = pool.length ? pool : ['谢谢参与']
  const weights = normalizeWeights(parseWeights(String(widget.values.weights ?? '1, 10, 89')), prizes.length)
  widget.data.prizes = prizes.join('、')
  widget.data.spinning = true
  widget.data.rotation = 0
  widget.data.result = '抽奖中…'
  const index = weightedIndex(weights)
  const total = weights.reduce((sum, value) => sum + value, 0)
  const before = weights.slice(0, index).reduce((sum, value) => sum + value, 0)
  const degree = ((before + Math.random() * weights[index]) / total) * 360
  const rotation = 360 * 6 + (360 - degree)
  requestAnimationFrame(() => { widget.data.rotation = rotation })
  const key = String(widget.featureId)
  const previous = hideTimers.get(key)
  if (previous) clearTimeout(previous)
  hideTimers.set(key, setTimeout(() => {
    widget.data.spinning = false
    widget.data.result = `抽中：${prizes[index]}`
    hideTimers.delete(key)
  }, Math.max(300, Number(widget.values.durationMs ?? 1700))))
}

function startMosquito(widget: ActiveWidget): void {
  const key = String(widget.featureId)
  const old = activeTimers.get(key)
  if (old) clearInterval(old)
  widget.data.score = 0
  widget.data.timeLeft = Math.max(1, Math.ceil(Number(widget.data.durationMs ?? widget.values.durationMs ?? 20000) / 1000))
  widget.data.active = true
  moveMosquito(widget)
  const timer = setInterval(() => {
    const left = Number(widget.data.timeLeft ?? 0) - 1
    widget.data.timeLeft = left
    if (left <= 0) {
      widget.data.active = false
      clearInterval(timer)
      activeTimers.delete(key)
    } else if (left % 2 === 0) moveMosquito(widget)
  }, 1000)
  activeTimers.set(key, timer)
}

function moveMosquito(widget: ActiveWidget): void {
  widget.data.x = 8 + Math.random() * 80
  widget.data.y = 8 + Math.random() * 70
}

function hitMosquito(widget: ActiveWidget): void {
  if (!widget.data.active) return
  widget.data.score = Number(widget.data.score ?? 0) + Math.max(1, Number(widget.values.score ?? 1))
  moveMosquito(widget)
}

function hideAfter(widget: ActiveWidget, duration: number): void {
  const key = String(widget.featureId)
  const old = hideTimers.get(key)
  if (old) clearTimeout(old)
  hideTimers.set(key, setTimeout(() => { removeWidget(widget.featureId) }, Math.max(100, duration)))
}

function handleUnlockKey(event: KeyboardEvent): void {
  const locked = widgets.value.find((widget) => widget.kind === 'lock' && widget.data.locked)
  if (!locked) return
  const configured = String(locked.values.unlockKey ?? 'SPACE').trim().toUpperCase()
  const actual = event.key === ' ' ? 'SPACE' : event.key.toUpperCase()
  if (event.key === 'Escape' || configured === actual) {
    event.preventDefault()
    removeWidget(locked.featureId)
  }
}

async function clickWoodfish(widget: ActiveWidget): Promise<void> {
  const merit = Math.max(1, Number(widget.values.meritPerClick ?? 1))
  const count = Number(widget.data.count ?? 0) + merit
  widget.data.count = count
  await api.features.increment('electronic-woodfish', 'currentMerit', merit)
  const path = String(widget.values.audioPath ?? '')
  if (path) void api.audio.play({ path, volume: 0.8, interrupt: false })
}

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

function slotImage(run: SlotRun, index: number): string | undefined { return run.images[index % run.images.length] }
function splitList(value: string): string[] { return value.split(/[,，]/).map((item) => item.trim()).filter(Boolean) }
function parseWeights(value: string): number[] { return splitList(value).map((item) => { const parsed = Number(item); return Number.isFinite(parsed) && parsed > 0 ? parsed : 0 }) }
function normalizeWeights(weights: number[], length: number): number[] {
  const values = Array.from({ length }, (_, index) => Math.max(0, Number(weights[index] ?? 1)))
  return values.some((value) => value > 0) ? values : values.map(() => 1)
}
function weightedIndex(weights: number[]): number {
  const total = weights.reduce((sum, item) => sum + item, 0)
  let cursor = Math.random() * total
  for (let index = 0; index < weights.length; index += 1) { cursor -= weights[index]; if (cursor < 0) return index }
  return Math.max(0, weights.length - 1)
}
function timerText(seconds: number): string {
  const safe = Math.max(0, Math.floor(seconds))
  const hours = Math.floor(safe / 3600)
  const minutes = Math.floor((safe % 3600) / 60)
  const secs = safe % 60
  return hours ? `${String(hours).padStart(2, '0')}:${String(minutes).padStart(2, '0')}:${String(secs).padStart(2, '0')}` : `${String(minutes).padStart(2, '0')}:${String(secs).padStart(2, '0')}`
}

const wheelColors = ['#f06f77', '#58a6ed', '#5dc995', '#f2bb59', '#ac83e8', '#44c4c8', '#f18dbd', '#96bd58']
function wheelGradient(poolText: string, weightText: string): string {
  const pool = poolText.split(/[,，]/).map((item) => item.trim()).filter(Boolean)
  const weights = weightText.split(/[,，]/).map((item) => Math.max(0, Number(item) || 0))
  const actual = pool.length ? pool : ['谢谢参与']
  const values = actual.map((_, index) => weights[index] ?? 1)
  const normalized = values.some((value) => value > 0) ? values : values.map(() => 1)
  const total = normalized.reduce((sum, value) => sum + value, 0)
  let position = 0
  const stops = normalized.map((weight, index) => {
    const start = position
    position += weight / total * 360
    return `${wheelColors[index % wheelColors.length]} ${start}deg ${position}deg`
  })
  return `conic-gradient(${stops.join(', ')})`
}
</script>

<template>
  <div class="slot-overlay" :class="slotRun?.theme ?? 'default'">
    <header class="component-status-bar"><b>yapp · 组件窗口</b><span>{{ widgets.length }} 个活动组件</span><span>ESC 可紧急解除窗口锁定</span></header>
    <div v-if="speechNotice" class="speech-notice" aria-live="polite">{{ speechNotice }}</div>
    <section v-if="slotRun" class="slot-game">
      <div class="slot-header"><span>LUCKY</span><b>水果机</b><span>DROP</span></div>
      <div class="slot-grid" :class="{ spinning: slotRun.spinning }">
        <div v-for="(_, index) in fallbackFruit" :key="index" class="slot-cell" :style="{ animationDelay: `${index * 35}ms` }">
          <img v-if="slotImage(slotRun, index)" :src="slotImage(slotRun, index)" :alt="fallbackFruit[index]" @error="($event.target as HTMLImageElement).style.display = 'none'">
          <span>{{ fallbackFruit[index] }}</span>
        </div>
      </div>
      <div class="slot-result" :class="{ active: !slotRun.spinning }">{{ slotRun.result }}</div>
    </section>

    <div v-if="!widgets.length && !slotRun" class="component-placeholder"><b>yapp</b><span>组件窗口已打开，等待玩法输出</span></div>
    <section v-if="widgets.length" class="widget-stage">
      <article v-for="widget in widgets" :key="widget.featureId" class="widget-card" :class="[`widget-${widget.kind}`, { 'widget-lock-active': widget.kind === 'lock' && widget.data.locked }]" :style="{ '--widget-accent': String(widget.values.barColor ?? widget.values.lockColor ?? '#51c9dc') }">
        <h2>{{ widget.title }}</h2>
        <template v-if="widget.kind === 'acceleration'">
          <strong class="widget-large-number">{{ widget.data.currentCount ?? 0 }}<small> / {{ widget.data.targetCount }}</small></strong>
          <div class="widget-progress"><i :style="{ width: `${widget.data.progress ?? 0}%` }" /></div>
          <p v-if="widget.data.preview">组件已启用 · 收到绑定礼物后开始处理</p>
          <p v-else>排队 {{ widget.data.pendingCount ?? 0 }} · {{ widget.featureId === 'speed-iba' ? `已处理批次 ${widget.data.batchCount ?? 0} · 每批 ${widget.values.batchSize}` : `曲线：${widget.values.curve}` }}</p>
        </template>
        <template v-else-if="widget.kind === 'health'">
          <strong class="widget-large-number">{{ widget.data.value }}<small> / {{ widget.data.maxValue }}</small></strong>
          <div class="health-track"><i :style="{ width: `${Math.max(0, Math.min(100, Number(widget.data.value) / Math.max(1, Number(widget.data.maxValue)) * 100))}%` }" /></div>
        </template>
        <template v-else-if="widget.kind === 'timer'">
          <div v-if="widget.values.style === 'ring'" class="timer-ring" :style="{ '--timer-progress': `${Math.max(0, Number(widget.data.remainingSeconds) / Math.max(1, Number(widget.data.totalSeconds)) * 100)}%` }"><span>{{ timerText(Number(widget.data.remainingSeconds ?? 0)) }}</span></div>
          <strong v-else class="timer-digital">{{ timerText(Number(widget.data.remainingSeconds ?? 0)) }}</strong>
          <p>{{ widget.data.preview ? '组件已启用 · 等待礼物调整倒计时' : widget.data.completed ? '倒计时结束' : '倒计时进行中' }}</p>
        </template>
        <template v-else-if="widget.kind === 'woodfish'">
          <button class="woodfish-button" type="button" aria-label="敲电子木鱼" @click="clickWoodfish(widget)"><span>木鱼</span><small>功德 +{{ widget.values.meritPerClick }}</small></button>
          <strong v-if="widget.values.showCounter" class="woodfish-count">功德 {{ widget.data.count ?? 0 }}</strong>
        </template>
        <template v-else-if="widget.kind === 'counter'">
          <strong class="widget-large-number counter-value">{{ widget.data.value ?? widget.values.initialValue ?? 0 }}</strong>
          <small>每次变化 {{ widget.values.step }}</small>
        </template>
        <template v-else-if="widget.kind === 'wheel'">
          <div class="wheel-wrap">
            <div class="wheel-pointer" />
            <div class="wheel-disc" :style="{ background: wheelGradient(String(widget.values.pool ?? ''), String(widget.values.weights ?? '')), transform: `rotate(${widget.data.rotation ?? 0}deg)`, transitionDuration: `${widget.values.durationMs ?? 1700}ms` }"><span>抽奖</span></div>
          </div>
          <strong class="wheel-result">{{ widget.data.result ?? '等待抽取' }}</strong>
          <small>{{ widget.data.prizes }}</small>
        </template>
        <template v-else-if="widget.kind === 'sticker'">
          <div class="sticker-value"><img v-if="widget.data.imagePath" :src="String(widget.data.imagePath)" :alt="String(widget.data.sticker)" @error="($event.target as HTMLImageElement).style.display = 'none'"><span>{{ widget.data.sticker }}</span></div>
          <small>{{ widget.data.preview ? '组件已启用 · 收到礼物后展示贴纸' : '礼物咖 · 贴纸展示' }}</small>
        </template>
        <template v-else-if="widget.kind === 'reply'">
          <div class="reply-bubble">{{ widget.data.text }}</div>
          <small>{{ widget.data.preview ? '组件已启用 · 匹配弹幕后显示回复' : '本地回复预览；未向直播平台发送' }}</small>
        </template>
        <template v-else-if="widget.kind === 'notice'">
          <div class="reply-bubble">{{ widget.data.text }}</div>
          <small>{{ widget.data.preview ? '等待礼物触发' : '玩法效果状态' }}</small>
        </template>
        <template v-else-if="widget.kind === 'lock'">
          <div class="lock-message">{{ widget.values.displayContent }}</div>
          <p v-if="widget.data.preview">组件已启用 · 收到绑定礼物后锁定</p>
          <p v-else>按 {{ widget.values.unlockKey }} 解锁 · ESC 可紧急解除</p>
        </template>
        <template v-else-if="widget.kind === 'mosquito'">
          <div class="mosquito-game">
            <strong>得分 {{ widget.data.score ?? 0 }} · {{ widget.data.timeLeft ?? 0 }} 秒</strong>
            <p v-if="widget.data.preview">组件已启用 · 收到绑定礼物后开始游戏</p>
            <template v-else>
              <button v-if="widget.data.active" class="mosquito-target" type="button" :style="{ left: `${widget.data.x}%`, top: `${widget.data.y}%` }" aria-label="拍中蚊子" @click="hitMosquito(widget)">
                <img :src="String(widget.values.imagePath ?? '')" alt="蚊子" @error="($event.target as HTMLImageElement).style.display = 'none'"><span>🦟</span>
              </button>
              <p v-else>游戏结束 · 命中 {{ widget.data.score ?? 0 }} 分</p>
            </template>
          </div>
        </template>
      </article>
    </section>
    <div v-if="slotRun" class="slot-tip">本地加权随机 · 组件输出</div>
  </div>
</template>
