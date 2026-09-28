<script setup lang="ts">
import { computed, nextTick, ref, toRaw, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { ArrowDown, Delete, Plus, Sort } from '@element-plus/icons-vue'
import type { Action, EventKind, KeyStep, MouseStep, Rule, RuleTriggerLimits } from '@shared/types'
import { api } from '../services/api'
import { useAppStore } from '../stores/app'
import AssetPathSelect from './AssetPathSelect.vue'
import TagListInput from './TagListInput.vue'
import HotkeyInput from './HotkeyInput.vue'
import { BUILTIN_EVENTS, BUILTIN_EVENT_DEFAULT_TIMEOUT_MS, BUILTIN_EVENT_MAX_TIMEOUT_MS, builtinEventById, builtinEventLabel } from '@shared/builtinEvents'

const props = defineProps<{ modelValue: boolean; rule?: Rule }>()
const emit = defineEmits<{ 'update:modelValue': [boolean]; saved: [] }>()
const store = useAppStore()
const draft = ref<Rule & { actions: any[] }>(blankRule() as Rule & { actions: any[] })
const keywordText = ref('')
const giftText = ref('')
const userText = ref('')
const limitsVisible = ref(false)
const serialPorts = ref<Array<{ path: string; manufacturer?: string; virtual: boolean }>>([])
const serialPortsLoading = ref(false)
const serialStopLoading = ref(false)
const serialBytesText = ref<Record<string, { onBytes: string; offBytes: string }>>({})
const dragIndex = ref<number | null>(null)
const keyOptions = [...'ABCDEFGHIJKLMNOPQRSTUVWXYZ', ...'0123456789', ...Array.from({ length: 24 }, (_, index) => `F${index + 1}`), 'CTRL', 'CONTROL', 'SHIFT', 'ALT', 'WIN', 'META', 'ENTER', 'RETURN', 'SPACE', 'SPACEBAR', 'TAB', 'ESC', 'ESCAPE', 'BACKSPACE', 'DELETE', 'INSERT', 'HOME', 'END', 'PAGEUP', 'PAGEDOWN', 'UP', 'DOWN', 'LEFT', 'RIGHT', 'CAPSLOCK', 'NUMLOCK', 'SCROLLLOCK', 'PAUSE', 'PRINTSCREEN', ...Array.from({ length: 10 }, (_, index) => `NUMPAD${index}`)]
const punctuationKeys = new Set(['-', '=', '[', ']', '\\', ';', "'", '`', ',', '.', '/'])
const title = computed(() => props.rule ? '修改玩法' : '增加新的玩法')
const countLabel = computed(() => {
  const kinds = draft.value.trigger.kinds
  if (kinds.includes('like') && !kinds.includes('gift')) return '指定点赞数量'
  if (kinds.includes('gift') && !kinds.includes('like')) return '指定礼物数量'
  return '最小数量'
})
const limitText = computed(() => {
  const limits = draft.value.triggerLimits
  if (!limits) return ''
  const parts: string[] = []
  if (limits.giftSecond) parts.push(`礼物 ${limits.giftSecond}s`)
  if (limits.textSecond) parts.push(`弹幕 ${limits.textSecond}s`)
  if (limits.likeSecond) parts.push(`点赞 ${limits.likeSecond}s`)
  if (limits.enterSecond) parts.push(`进场 ${limits.enterSecond}s`)
  return parts.join(' · ')
})
const eventKinds: Array<{ label: string; value: EventKind }> = [{ label: '礼物', value: 'gift' }, { label: '弹幕', value: 'chat' }, { label: '点赞', value: 'like' }, { label: '关注', value: 'follow' }, { label: '进场', value: 'enter' }, { label: '系统', value: 'system' }]

watch(() => props.modelValue, (visible) => { if (visible) { loadDraft(); void refreshSerialPorts() } })

function loadDraft(): void {
  // props.rule / draft 都是 Vue 响应式代理，structuredClone 无法克隆 Proxy，先取原始对象。
  draft.value = props.rule ? structuredClone(toRaw(props.rule)) : blankRule()
  draft.value.repeatCount = draft.value.repeatCount ?? 1
  draft.value.nowait = Boolean(draft.value.nowait)
  draft.value.hotkey = draft.value.hotkey ?? ''
  draft.value.triggerLimits = normalizeLimits(draft.value.triggerLimits)
  keywordText.value = draft.value.trigger.keywords?.join(', ') ?? ''
  giftText.value = draft.value.trigger.giftNames?.join(', ') ?? ''
  userText.value = draft.value.trigger.users?.join(', ') ?? ''
  serialBytesText.value = {}
  for (const action of draft.value.actions) {
    if (action.kind === 'serial') serialBytesText.value[action.id] = { onBytes: action.onBytes.join(', '), offBytes: action.offBytes.join(', ') }
  }
}
function normalizeLimits(limits?: RuleTriggerLimits): RuleTriggerLimits {
  return { giftSecond: limits?.giftSecond ?? 0, textSecond: limits?.textSecond ?? 0, likeSecond: limits?.likeSecond ?? 0, enterSecond: limits?.enterSecond ?? 0 }
}
// 触发限制的四项都为 0 时不写回配置，保持规则 JSON 干净。
function cleanedLimits(limits?: RuleTriggerLimits): RuleTriggerLimits | undefined {
  const values = normalizeLimits(limits)
  return values.giftSecond || values.textSecond || values.likeSecond || values.enterSecond ? values : undefined
}
function blankRule(): Rule {
  return { id: crypto.randomUUID(), name: '', enabled: true, priority: 1, trigger: { kinds: ['gift'], keywordMode: 'contains' }, cooldownMs: 0, probability: 1, concurrency: 'queue', actions: [], hotkey: '', repeatCount: 1, nowait: false, triggerLimits: normalizeLimits(), playSound: false, soundSimultaneous: false, soundIntervalMs: 800, countActionTime: false, noAcceleration: false, boxWeight: 1 }
}
function close(): void { emit('update:modelValue', false) }
// 事件类型改成图二那样的一排开关：点一下就增删对应的触发类型（至少保留一个）。
function toggleKind(kind: EventKind, enabled: boolean): void {
  const kinds = new Set(draft.value.trigger.kinds)
  if (enabled) kinds.add(kind)
  else kinds.delete(kind)
  if (!kinds.size) kinds.add(kind)
  draft.value.trigger.kinds = eventKinds.map((item) => item.value).filter((value) => kinds.has(value))
}
// 触发内容 / 关键词 / 用户：标签控件与逗号分隔文本互相转换。
// 模板里的 ref 会被自动解包，所以这里用 computed + setter 函数成对提供。
function splitTags(value: string): string[] {
  return value.split(/[,，]/).map((item) => item.trim()).filter(Boolean)
}
const giftTags = computed(() => splitTags(giftText.value))
const keywordTags = computed(() => splitTags(keywordText.value))
const userTags = computed(() => splitTags(userText.value))
function setGiftTags(tags: string[]): void { giftText.value = tags.join(', ') }
function setKeywordTags(tags: string[]): void { keywordText.value = tags.join(', ') }
function setUserTags(tags: string[]): void { userText.value = tags.join(', ') }
function resetDraft(): void {
  ElMessageBox.confirm('重置会清空当前编辑内容，确定继续？', '重置玩法', { type: 'warning', confirmButtonText: '重置', cancelButtonText: '取消' })
    .then(() => {
      draft.value = blankRule() as Rule & { actions: any[] }
      keywordText.value = ''
      giftText.value = ''
      userText.value = ''
      serialBytesText.value = {}
    })
    .catch(() => undefined)
}
function addAction(kind: Action['kind'] = 'drop'): void {
  const action = createAction(kind)
  draft.value.actions.push(action)
  if (action.kind === 'serial') serialBytesText.value[action.id] = { onBytes: '', offBytes: '' }
  scrollToAction(action.id)
}
// cloneAction 复制一份动作插在原动作后面（图三里的「克隆」），串口字节文本一起带过去。
function cloneAction(action: Action): void {
  const index = draft.value.actions.findIndex((item: Action) => item.id === action.id)
  if (index < 0) return
  const copy = JSON.parse(JSON.stringify(toRaw(action))) as Action
  copy.id = crypto.randomUUID()
  draft.value.actions.splice(index + 1, 0, copy)
  const serialText = serialBytesText.value[action.id]
  if (serialText) serialBytesText.value[copy.id] = { ...serialText }
  scrollToAction(copy.id)
}
// 新增 / 克隆后把新动作滚进可见区域，省得用户自己找。
function scrollToAction(id: string): void {
  void nextTick(() => document.getElementById(`action-block-${id}`)?.scrollIntoView({ block: 'nearest', behavior: 'smooth' }))
}
function createAction(kind: Action['kind']): Action {
  const base = { id: crypto.randomUUID(), delayMs: 0, repeat: 1 }
  if (kind === 'video') return { ...base, kind, path: 'videos/功德狗.mp4', lane: 1, durationMs: 8000, loop: false, chroma: { enabled: true, color: '#00ff00', similarity: 0.35, smoothness: 0.12 } }
  if (kind === 'audio') return { ...base, kind, path: 'voices/测试音效.mp3', volume: 0.8, interrupt: false, loop: false }
  if (kind === 'drop') return { ...base, kind, image: 'images/平底锅.png', count: 5, gravity: 1800, bounce: 0.45, durationMs: 3500 }
  if (kind === 'serial') return { ...base, kind, port: '', baud: 9600, onBytes: [], offBytes: [], pulseMs: 800 }
  if (kind === 'obs') return { ...base, kind, command: 'SetInputMute', args: { inputName: '直播音效', inputMuted: 'false' } }
  if (kind === 'sleep') return { ...base, kind, durationMs: 1000 }
  if (kind === 'showimage') return { ...base, kind, image: 'images/平底锅.png', count: 1, size: 200, x: 0, y: 0, durationMs: 3000 }
  if (kind === 'randombox') return { ...base, kind, pool: ['一等奖', '二等奖', '谢谢参与'], weights: [1, 10, 89], durationMs: 3000 }
  if (kind === 'app') return { ...base, kind, appAction: 'video', path: '', durationMs: 3000, loop: false }
  if (kind === 'gospeed') return { ...base, kind, targetCount: 10, speed: 1, durationMs: 3000 }
  if (kind === 'obs_filter') return { ...base, kind, filterName: '', visible: true, autoHide: false, durationMs: 3000 }
  if (kind === 'rule') return { ...base, kind, ruleEvent: '', ruleEventTimeoutMs: BUILTIN_EVENT_DEFAULT_TIMEOUT_MS, killProcessName: '', runExePath: '' }
  if (kind === 'tielian') return { ...base, kind, delta: 1, effect: '增加', countMin: 1, countMax: 1 }
  if (kind === 'trash') return { ...base, kind, count: 1, image: 'images/垃圾.png', bin: '', durationMs: 4000 }
  if (kind === 'key') return { ...base, kind, steps: [{ op: 'down', key: 'CTRL' }, { op: 'tap', key: '1' }, { op: 'up', key: 'CTRL' }] as KeyStep[] }
  return { ...base, kind: 'mouse', steps: [{ op: 'move', x: 100, y: 100 }, { op: 'click', button: 'left' }] as MouseStep[] }
}
function removeAction(id: string): void { draft.value.actions = draft.value.actions.filter((action) => action.id !== id) }
function moveAction(index: number, direction: -1 | 1): void { const target = index + direction; if (target < 0 || target >= draft.value.actions.length) return; const [item] = draft.value.actions.splice(index, 1); draft.value.actions.splice(target, 0, item) }
function startDrag(index: number): void { dragIndex.value = index }
function dropAction(index: number): void { const source = dragIndex.value; dragIndex.value = null; if (source === null || source === index) return; const [item] = draft.value.actions.splice(source, 1); draft.value.actions.splice(index, 0, item) }
function setActionKind(action: Action, kind: Action['kind']): void { const next = createAction(kind); next.id = action.id; const index = draft.value.actions.findIndex((item: Action) => item.id === action.id); if (index >= 0) draft.value.actions.splice(index, 1, next); if (next.kind === 'serial') serialBytesText.value[action.id] = { onBytes: '', offBytes: '' } }
function actionText(action: Action): string { switch (action.kind) { case 'key': return '键盘步骤'; case 'mouse': return '鼠标步骤'; case 'video': case 'audio': return action.path; case 'drop': return action.image; case 'serial': return action.port || '未选串口'; case 'obs': return action.command; case 'sleep': return `等待 ${action.durationMs ?? 1000} ms`; case 'showimage': return action.image || '未选图片'; case 'randombox': return `盲盒 ${(action.pool ?? []).length} 项`; case 'app': return action.path || '未选视频'; case 'gospeed': return `到达 ${action.targetCount ?? 10} · ×${action.speed ?? 1}`; case 'obs_filter': return action.filterName || '未选滤镜'; case 'rule': return builtinEventLabel(action.ruleEvent) || '未选事件'; case 'tielian': return `${action.effect ?? '增加'} ${action.delta ?? 1} 层`; case 'trash': return `掉落 ${action.count ?? 1} 个` } }
function changeKeyStep(action: Action, index: number, value: string | number): void {
  if (action.kind !== 'key') return
  const op = String(value) as KeyStep['op']
  if (!['down', 'up', 'tap', 'wait'].includes(op)) return
  const previous = action.steps[index]
  action.steps[index] = op === 'wait'
    ? { op, ms: 'ms' in previous ? previous.ms : 300 }
    : { op, key: 'key' in previous ? previous.key : 'F8' }
}
function changeMouseStep(action: Action, index: number, value: string | number): void {
  if (action.kind !== 'mouse') return
  const op = String(value) as MouseStep['op']
  if (!['move', 'click', 'down', 'up', 'scroll', 'wait'].includes(op)) return
  const previous = action.steps[index]
  if (op === 'wait') action.steps[index] = { op, ms: 'ms' in previous ? previous.ms : 300 }
  else if (op === 'scroll') action.steps[index] = { op, delta: 'delta' in previous ? previous.delta : 120 }
  else if (op === 'move') action.steps[index] = { op, x: 'x' in previous ? previous.x ?? 100 : 100, y: 'y' in previous ? previous.y ?? 100 : 100 }
  else action.steps[index] = { op, button: 'button' in previous ? previous.button ?? 'left' : 'left', ...('x' in previous && previous.x !== undefined ? { x: previous.x } : {}), ...('y' in previous && previous.y !== undefined ? { y: previous.y } : {}) }
}
function addInputStep(action: Action): void {
  if (action.kind === 'key') action.steps.push({ op: 'tap', key: 'F8' })
  else if (action.kind === 'mouse') action.steps.push({ op: 'move', x: 100, y: 100 })
}
function removeInputStep(action: Action, index: number): void {
  if (action.kind === 'key') action.steps.splice(index, 1)
  else if (action.kind === 'mouse') action.steps.splice(index, 1)
}
function moveInputStep(action: Action, index: number, direction: -1 | 1): void {
  if (action.kind === 'key') {
    const target = index + direction
    if (target < 0 || target >= action.steps.length) return
    const [step] = action.steps.splice(index, 1)
    action.steps.splice(target, 0, step)
  } else if (action.kind === 'mouse') {
    const target = index + direction
    if (target < 0 || target >= action.steps.length) return
    const [step] = action.steps.splice(index, 1)
    action.steps.splice(target, 0, step)
  }
}
function isSupportedKey(value: string): boolean {
  const key = value.trim().toUpperCase()
  return keyOptions.includes(key) || punctuationKeys.has(key)
}
function validateInputActions(): boolean {
  for (const action of draft.value.actions as Action[]) {
    if (action.kind === 'key') {
      if (!action.steps.length || action.steps.length > 256) { ElMessage.warning('键盘步骤需要在 1 到 256 步之间'); return false }
      for (const [index, step] of action.steps.entries()) {
        if (step.op === 'wait' && (!Number.isInteger(step.ms) || step.ms < 0 || step.ms > 60000)) { ElMessage.warning(`第 ${index + 1} 步等待时间需要在 0 到 60000 毫秒之间`); return false }
        if (step.op !== 'wait' && !isSupportedKey(step.key)) { ElMessage.warning(`第 ${index + 1} 步的按键无效，请从列表选择或输入支持的按键`); return false }
      }
    } else if (action.kind === 'mouse') {
      if (!action.steps.length || action.steps.length > 256) { ElMessage.warning('鼠标步骤需要在 1 到 256 步之间'); return false }
      for (const [index, step] of action.steps.entries()) {
        if (step.op === 'wait' && (!Number.isInteger(step.ms) || step.ms < 0 || step.ms > 60000)) { ElMessage.warning(`第 ${index + 1} 步等待时间需要在 0 到 60000 毫秒之间`); return false }
        if (step.op === 'scroll' && (!Number.isInteger(step.delta) || step.delta < -2147483648 || step.delta > 2147483647)) { ElMessage.warning(`请填写第 ${index + 1} 步的滚轮距离`); return false }
        if (step.op === 'move' || step.op === 'click' || step.op === 'down' || step.op === 'up') {
          const hasX = step.x !== undefined
          const hasY = step.y !== undefined
          if (hasX !== hasY || (step.op === 'move' && !hasX) || (hasX && (!Number.isInteger(step.x) || !Number.isInteger(step.y) || step.x! < -32768 || step.x! > 32767 || step.y! < -32768 || step.y! > 32767))) { ElMessage.warning(`请检查第 ${index + 1} 步的鼠标坐标`); return false }
        }
      }
    }
  }
  return true
}
// 随机盲盒的「奖项池 + 权重」编辑逻辑。
type PooledAction = { pool?: string[]; weights?: number[] }
function updatePool(action: PooledAction, value: string): void { action.pool = value.split(/[,，]/).map((item) => item.trim()).filter(Boolean) }
function updateWeights(action: PooledAction, value: string): void { action.weights = value.split(/[,，]/).map((item) => Number(item.trim())).filter((item) => Number.isFinite(item) && item >= 0) }
function serialBytesValue(action: Extract<Action, { kind: 'serial' }>, field: 'onBytes' | 'offBytes'): string {
  return serialBytesText.value[action.id]?.[field] ?? action[field].join(', ')
}
function updateSerialBytes(action: Extract<Action, { kind: 'serial' }>, field: 'onBytes' | 'offBytes', value: string): void {
  const current = serialBytesText.value[action.id] ?? { onBytes: action.onBytes.join(', '), offBytes: action.offBytes.join(', ') }
  serialBytesText.value = { ...serialBytesText.value, [action.id]: { ...current, [field]: value } }
}
function parseSerialBytes(value: string): number[] | undefined {
  const items = value.split(/[,，]/).map((item) => item.trim())
  if (!value.trim() || items.length > 4096 || items.some((item) => !/^\d+$/.test(item))) return undefined
  const bytes = items.map(Number)
  return bytes.every((item) => Number.isInteger(item) && item >= 0 && item <= 255) ? bytes : undefined
}
function validateSerialActions(): boolean {
  for (const action of draft.value.actions) {
    if (action.kind !== 'serial') continue
    if (!action.port.trim()) { ElMessage.warning('串口动作需要先选择或输入设备端口'); return false }
    const onBytes = parseSerialBytes(serialBytesValue(action, 'onBytes'))
    const offBytes = parseSerialBytes(serialBytesValue(action, 'offBytes'))
    if (!onBytes?.length || !offBytes?.length) { ElMessage.warning('开启字节和关闭字节都需要填写 1 到 4096 个 0–255 整数'); return false }
    action.port = action.port.trim()
    action.onBytes = onBytes
    action.offBytes = offBytes
  }
  return true
}
async function refreshSerialPorts(): Promise<void> {
  serialPortsLoading.value = true
  try { serialPorts.value = await api.serial.ports() }
  catch (error) { ElMessage.error(error instanceof Error ? error.message : '读取串口列表失败') }
  finally { serialPortsLoading.value = false }
}
async function stopSerialActions(): Promise<void> {
  serialStopLoading.value = true
  try { await api.serial.stopAll(); ElMessage.success('已请求停止全部串口脉冲') }
  catch (error) { ElMessage.error(error instanceof Error ? error.message : '停止串口动作失败') }
  finally { serialStopLoading.value = false }
}
function targetValue(action: Action, field: 'title' | 'className' | 'processName'): string {
  return action.kind === 'key' || action.kind === 'mouse' ? action.target?.[field] ?? '' : ''
}
function setTargetValue(action: Action, field: 'title' | 'className' | 'processName', value: string): void {
  if (action.kind !== 'key' && action.kind !== 'mouse') return
  const target = { ...action.target }
  const cleaned = value.trim()
  if (cleaned) target[field] = cleaned
  else delete target[field]
  action.target = Object.keys(target).length ? target : undefined
}
function updateObsArgs(action: Extract<Action, { kind: 'obs' }>, value: string): void {
  try {
    const parsed = JSON.parse(value || '{}')
    if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) action.args = parsed as Record<string, string>
    else throw new Error('object required')
  } catch { ElMessage.warning('OBS 参数必须是 JSON 对象') }
}
async function save(): Promise<void> {
  if (!draft.value.name.trim()) { ElMessage.warning('名称不能为空'); return }
  if (!draft.value.trigger.kinds.length) { ElMessage.warning('至少选择一种触发类型'); return }
  const needsContent = draft.value.trigger.kinds.some((kind) => kind === 'gift' || kind === 'chat')
  if (needsContent && !giftText.value.trim() && !keywordText.value.trim()) { ElMessage.warning('触发内容不能为空'); return }
  if (draft.value.soundSimultaneous && !draft.value.playSound) { ElMessage.warning('无法保存：当前操作不允许开启「声音同时播放」，请关闭后再试'); return }
  draft.value.trigger.keywords = keywordText.value.split(/[,，]/).map((item) => item.trim()).filter(Boolean)
  draft.value.trigger.giftNames = giftText.value.split(/[,，]/).map((item) => item.trim()).filter(Boolean)
  draft.value.trigger.users = userText.value.split(/[,，]/).map((item) => item.trim()).filter(Boolean)
  const sources = draft.value.trigger.source?.filter(Boolean) ?? []
  draft.value.trigger.source = sources.length ? sources : undefined
  if (!validateInputActions()) return
  if (!validateSerialActions()) return
  const payload = structuredClone(toRaw(draft.value))
  payload.hotkey = (payload.hotkey ?? '').trim()
  payload.repeatCount = Math.min(99, Math.max(1, Number(payload.repeatCount) || 1))
  payload.triggerLimits = cleanedLimits(payload.triggerLimits)
  try {
    await store.saveRule(payload)
  } catch (error) {
    // 热键被占用等校验由主程序负责，这里把原文提示出来。
    ElMessage.error(error instanceof Error ? error.message : '保存失败，请检查热键是否被占用')
    return
  }
  ElMessage.success('玩法已保存')
  emit('saved')
  close()
}
</script>

<template>
  <el-dialog :model-value="modelValue" :title="title" width="min(920px, calc(100vw - 32px))" top="5vh" class="rule-editor-dialog" :close-on-click-modal="false" draggable destroy-on-close @update:model-value="emit('update:modelValue', $event)">
    <div class="editor-layout">
      <section class="form-card">
        <div class="form-card-title">基本信息</div>
        <div class="field-row">
          <span class="field-label">名称</span>
          <div class="field-control"><el-input v-model="draft.name" maxlength="40" show-word-limit placeholder="例如：啤酒掉落 / 弹幕抽奖" /></div>
        </div>
        <div class="field-row">
          <span class="field-label">绑定热键</span>
          <div class="field-control field-control-inline">
            <HotkeyInput v-model="draft.hotkey" class="hotkey-field" placeholder="点击录入，例如 Ctrl + Shift + F" />
            <el-button @click="draft.hotkey = ''">解除热键</el-button>
          </div>
        </div>
        <div class="field-grid three">
          <label class="field"><span class="field-label">优先级</span><el-input-number v-model="draft.priority" :min="0" :max="999" /></label>
          <label class="field"><span class="field-label">盲盒概率</span><el-input-number v-model="draft.boxWeight" :min="0" :max="999" /></label>
          <label class="field"><span class="field-label">重复次数</span><el-input-number v-model="draft.repeatCount" :min="1" :max="99" /></label>
        </div>
        <div class="field-row">
          <span class="field-label">运行方式</span>
          <div class="field-control switch-grid">
            <label class="switch-cell"><span>启用玩法</span><el-switch v-model="draft.enabled" /></label>
            <label class="switch-cell"><span>立即执行</span><el-switch v-model="draft.nowait" /><el-tooltip content="开启后不排队，匹配到就立刻执行" placement="top"><span class="icon-hint">?</span></el-tooltip></label>
            <label class="switch-cell"><span>同时播放声音</span><el-switch v-model="draft.soundSimultaneous" /><el-tooltip content="多条声音同时播放，而不是依次播放" placement="top"><span class="icon-hint">?</span></el-tooltip></label>
            <label class="switch-cell"><span>不参与加速</span><el-switch v-model="draft.noAcceleration" /><el-tooltip content="开启后，当前操作的「加速度」将变得无效" placement="top"><span class="icon-hint">?</span></el-tooltip></label>
          </div>
        </div>
        <div v-if="!draft.nowait" class="field-row">
          <span class="field-label">等待时长</span>
          <div class="field-control field-control-inline">
            <el-input-number v-model="draft.waitMs" :min="0" :max="600000" aria-label="等待时长" />
            <span class="unit">毫秒</span>
            <span class="field-note">排队执行时，动作开始前先等这么久</span>
          </div>
        </div>
      </section>

      <section class="form-card">
        <div class="form-card-title">触发条件<span class="form-card-note">命中任意一类事件即可触发</span></div>
        <div class="field-row">
          <span class="field-label">事件类型</span>
          <div class="field-control switch-grid">
            <label class="switch-cell"><span>礼物</span><el-switch :model-value="draft.trigger.kinds.includes('gift')" aria-label="礼物触发" @update:model-value="toggleKind('gift', Boolean($event))" /></label>
            <label class="switch-cell"><span>弹幕</span><el-switch :model-value="draft.trigger.kinds.includes('chat')" aria-label="弹幕触发" @update:model-value="toggleKind('chat', Boolean($event))" /></label>
            <label class="switch-cell"><span>点赞</span><el-switch :model-value="draft.trigger.kinds.includes('like')" aria-label="点赞触发" @update:model-value="toggleKind('like', Boolean($event))" /></label>
            <label class="switch-cell"><span>关注</span><el-switch :model-value="draft.trigger.kinds.includes('follow')" aria-label="关注触发" @update:model-value="toggleKind('follow', Boolean($event))" /></label>
            <label class="switch-cell"><span>进场</span><el-switch :model-value="draft.trigger.kinds.includes('enter')" aria-label="进场触发" @update:model-value="toggleKind('enter', Boolean($event))" /></label>
            <label class="switch-cell"><span>系统</span><el-switch :model-value="draft.trigger.kinds.includes('system')" aria-label="系统触发" @update:model-value="toggleKind('system', Boolean($event))" /></label>
          </div>
        </div>
        <div class="field-row">
          <span class="field-label">触发内容</span>
          <div class="field-control"><TagListInput :model-value="giftTags" placeholder="礼物名或弹幕内容，回车添加" aria-label="触发内容" @update:model-value="setGiftTags" /></div>
        </div>
        <div class="field-row">
          <span class="field-label">关键词</span>
          <div class="field-control field-control-inline">
            <TagListInput :model-value="keywordTags" placeholder="关键词，回车添加" aria-label="关键词" @update:model-value="setKeywordTags" />
            <span class="field-label field-label-inline">匹配方式</span>
            <el-select v-model="draft.trigger.keywordMode" class="select-140"><el-option label="包含" value="contains" /><el-option label="完全匹配" value="exact" /><el-option label="正则表达式" value="regex" /></el-select>
          </div>
        </div>
        <div class="field-row">
          <span class="field-label">指定用户</span>
          <div class="field-control"><TagListInput :model-value="userTags" placeholder="用户ID或昵称，回车添加；留空表示全部用户" aria-label="指定用户触发" @update:model-value="setUserTags" /></div>
        </div>
        <div class="field-row">
          <span class="field-label">最少数量</span>
          <div class="field-control field-control-inline">
            <el-input-number v-model="draft.trigger.minCount" :min="0" :max="99999" aria-label="最少数量" />
            <span class="unit">0 = 不限</span>
          </div>
        </div>
        <div class="field-row">
          <span class="field-label">触发限制</span>
          <div class="field-control field-control-inline">
            <el-button plain @click="limitsVisible = true">设置</el-button>
            <span class="field-note">{{ limitText || '未设置（同一用户在该秒数内只触发一次）' }}</span>
          </div>
        </div>
      </section>

      <section class="form-card">
        <div class="form-card-title">动作<span class="form-card-note">按顺序执行，可拖动卡片调整顺序</span><span class="form-card-count">{{ draft.actions.length }} 个</span></div>
        <div v-if="draft.actions.length" class="action-list">
          <article v-for="(action, index) in draft.actions" :id="`action-block-${action.id}`" :key="action.id" class="action-block" draggable="true" @dragstart="startDrag(index)" @dragover.prevent @drop="dropAction(index)">
            <header class="action-head">
              <span class="action-index">{{ String(index + 1).padStart(2, '0') }}</span>
              <el-select :model-value="action.kind" class="action-kind" aria-label="动作类型" @update:model-value="setActionKind(action, $event)"><el-option label="键盘" value="key" /><el-option label="鼠标" value="mouse" /><el-option label="等待" value="sleep" /><el-option label="绿幕视频" value="video" /><el-option label="声音" value="audio" /><el-option label="砸图片" value="showimage" /><el-option label="砸落物" value="drop" /><el-option label="随机盲盒" value="randombox" /><el-option label="手机玩法" value="app" /><el-option label="物理硬件" value="serial" /><el-option label="OBS 联动" value="obs" /><el-option label="OBS场景滤镜" value="obs_filter" /><el-option label="内置事件" value="rule" /><el-option label="加速度" value="gospeed" /><el-option label="屏幕锁链" value="tielian" /><el-option label="垃圾掉落" value="trash" /></el-select>
              <span class="action-summary">{{ actionText(action) }}</span>
              <div class="action-tools">
                <el-button link aria-label="上移动作" @click.stop="moveAction(index, -1)">↑</el-button>
                <el-button link aria-label="下移动作" @click.stop="moveAction(index, 1)">↓</el-button>
                <el-button link type="primary" aria-label="克隆动作" @click.stop="cloneAction(action)">克隆</el-button>
                <el-button link type="danger" aria-label="删除动作" @click.stop="removeAction(action.id)">删除</el-button>
              </div>
            </header>
            <div class="action-body">
              <el-form label-position="left" label-width="118px" class="action-config-form">
          <template v-if="action.kind === 'video'">
            <el-form-item label="视频素材路径"><AssetPathSelect v-model="action.path" kind="video" /></el-form-item>
            <div class="form-grid three">
              <el-form-item label="播放通道"><el-input-number v-model="action.lane" aria-label="播放通道" :min="1" :max="8" /></el-form-item>
              <el-form-item label="播放时长（毫秒）"><el-input-number v-model="action.durationMs" aria-label="播放时长" :min="100" :max="600000" /></el-form-item>
              <el-form-item label="循环播放"><el-switch v-model="action.loop" aria-label="循环播放" /></el-form-item>
            </div>
          </template>
          <template v-else-if="action.kind === 'audio'">
            <el-form-item label="音频素材路径"><AssetPathSelect v-model="action.path" kind="audio" /></el-form-item>
            <div class="form-grid">
              <el-form-item label="音量"><el-slider v-model="action.volume" aria-label="音量" :min="0" :max="1" :step="0.05" show-input /></el-form-item>
              <el-form-item label="打断当前声音"><el-switch v-model="action.interrupt" aria-label="打断当前声音" /></el-form-item>
            </div>
          </template>
          <template v-else-if="action.kind === 'drop'">
            <el-form-item label="图片素材路径"><AssetPathSelect v-model="action.image" kind="image" /></el-form-item>
            <div class="form-grid three">
              <el-form-item label="数量"><el-input-number v-model="action.count" aria-label="数量" :min="1" :max="100" /></el-form-item>
              <el-form-item label="重力"><el-input-number v-model="action.gravity" aria-label="重力" :min="0" :max="5000" /></el-form-item>
              <el-form-item label="弹跳系数"><el-input-number v-model="action.bounce" aria-label="弹跳系数" :min="0" :max="1" :step="0.05" /></el-form-item>
            </div>
          </template>
          <template v-else-if="action.kind === 'serial'">
            <div class="serial-port-row">
              <el-form-item label="串口">
                <el-select v-model="action.port" filterable allow-create clearable aria-label="串口" placeholder="选择或输入串口…">
                  <el-option v-for="port in serialPorts" :key="port.path" :label="port.manufacturer ? port.path + ' · ' + port.manufacturer : port.path" :value="port.path" />
                </el-select>
              </el-form-item>
              <el-button size="small" :loading="serialPortsLoading" @click="refreshSerialPorts">刷新端口</el-button>
            </div>
            <div class="form-grid three">
              <el-form-item label="波特率"><el-input-number v-model="action.baud" aria-label="波特率" :min="300" :max="4000000" /></el-form-item>
              <el-form-item label="脉冲时间（毫秒）"><el-input-number v-model="action.pulseMs" aria-label="脉冲时间" :min="1" :max="60000" /></el-form-item>
              <el-form-item label="开启字节"><el-input :model-value="serialBytesValue(action, 'onBytes')" aria-label="开启字节" @update:model-value="updateSerialBytes(action, 'onBytes', $event)" placeholder="例如 1, 2, 3…" /></el-form-item>
              <el-form-item label="关闭字节"><el-input :model-value="serialBytesValue(action, 'offBytes')" aria-label="关闭字节" @update:model-value="updateSerialBytes(action, 'offBytes', $event)" placeholder="例如 1, 2, 0…" /></el-form-item>
            </div>
            <div class="serial-footer"><p class="action-note" role="note">会向所选串口写入真实字节，并在脉冲结束后发送关闭字节。请先确认端口和设备指令。</p><el-button size="small" plain :loading="serialStopLoading" @click="stopSerialActions">停止全部串口脉冲</el-button></div>
          </template>
          <template v-else-if="action.kind === 'obs'">
            <div class="form-grid">
              <el-form-item label="OBS 请求名称"><el-input v-model="action.command" aria-label="OBS 请求名称" placeholder="例如 SetInputMute…" /></el-form-item>
              <el-form-item label="请求参数 JSON"><el-input :model-value="JSON.stringify(action.args ?? {})" aria-label="OBS 请求参数" @update:model-value="updateObsArgs(action, $event)" placeholder="JSON 对象，例如 {&quot;sceneName&quot;:&quot;直播&quot;}…" /></el-form-item>
            </div>
          </template>
          <template v-else-if="action.kind === 'sleep'">
            <div class="form-grid"><el-form-item label="等待时长（毫秒）"><el-input-number v-model="action.durationMs" aria-label="等待时长" :min="0" :max="600000" /></el-form-item></div>
          </template>
          <template v-else-if="action.kind === 'showimage'">
            <el-form-item label="图片素材路径"><AssetPathSelect v-model="action.image" kind="image" /></el-form-item>
            <div class="form-grid three">
              <el-form-item label="砸出数量"><el-input-number v-model="action.count" aria-label="砸出数量" :min="1" :max="100" /></el-form-item>
              <el-form-item label="礼物大小"><el-input-number v-model="action.size" aria-label="礼物大小" :min="20" :max="2000" /></el-form-item>
              <el-form-item label="停留（毫秒）"><el-input-number v-model="action.durationMs" aria-label="停留时长" :min="100" :max="600000" /></el-form-item>
            </div>
            <div class="form-grid">
              <el-form-item label="砸向 X"><el-input-number v-model="action.x" aria-label="砸向位置 X" :min="-32768" :max="32767" /></el-form-item>
              <el-form-item label="砸向 Y"><el-input-number v-model="action.y" aria-label="砸向位置 Y" :min="-32768" :max="32767" /></el-form-item>
            </div>
          </template>
          <template v-else-if="action.kind === 'randombox'">
            <div class="form-grid">
              <el-form-item label="盲盒奖项池"><el-input :model-value="(action.pool ?? []).join(', ')" aria-label="盲盒奖项池" @update:model-value="updatePool(action, $event)" placeholder="逗号分隔…" /></el-form-item>
              <el-form-item label="权重"><el-input :model-value="(action.weights ?? []).join(', ')" aria-label="奖项权重" @update:model-value="updateWeights(action, $event)" placeholder="与奖项顺序对应…" /></el-form-item>
            </div>
            <div class="form-grid"><el-form-item label="动画时长（毫秒）"><el-input-number v-model="action.durationMs" aria-label="动画时长" :min="100" :max="600000" /></el-form-item></div>
          </template>
          <template v-else-if="action.kind === 'app'">
            <el-form-item label="视频素材路径"><AssetPathSelect v-model="action.path" kind="video" /></el-form-item>
            <div class="form-grid three">
              <el-form-item label="玩法动作"><el-select v-model="action.appAction" aria-label="玩法动作"><el-option label="视频" value="video" /></el-select></el-form-item>
              <el-form-item label="播放时长（毫秒）"><el-input-number v-model="action.durationMs" aria-label="播放时长" :min="100" :max="600000" /></el-form-item>
              <el-form-item label="循环播放"><el-switch v-model="action.loop" aria-label="循环播放" /></el-form-item>
            </div>
          </template>
          <template v-else-if="action.kind === 'gospeed'">
            <div class="form-grid three">
              <el-form-item label="到达数量"><el-input-number v-model="action.targetCount" aria-label="到达数量" :min="1" :max="9999" /></el-form-item>
              <el-form-item label="速度"><el-input-number v-model="action.speed" aria-label="速度" :min="1" :max="2" :step="0.1" /><small class="field-help">1.0 - 2.0</small></el-form-item>
              <el-form-item label="加速时长（毫秒）"><el-input-number v-model="action.durationMs" aria-label="加速时长" :min="100" :max="600000" /></el-form-item>
            </div>
          </template>
          <template v-else-if="action.kind === 'obs_filter'">
            <div class="form-grid three">
              <el-form-item label="选择滤镜"><el-input v-model="action.filterName" aria-label="选择滤镜" placeholder="滤镜名称…" /></el-form-item>
              <el-form-item label="显示"><el-switch v-model="action.visible" aria-label="显示滤镜" /></el-form-item>
              <el-form-item label="自动隐藏"><el-switch v-model="action.autoHide" aria-label="自动隐藏" /></el-form-item>
            </div>
            <div class="form-grid"><el-form-item label="持续时间（毫秒）"><el-input-number v-model="action.durationMs" aria-label="持续时间" :min="100" :max="600000" /></el-form-item></div>
            <p class="action-note" role="note">需要在 OBS 中开启 Websocket 服务器，并关闭鉴权。</p>
          </template>
          <template v-else-if="action.kind === 'rule'">
            <div class="form-grid">
              <el-form-item label="内置事件"><el-select v-model="action.ruleEvent" aria-label="内置事件" placeholder="选择一个内置事件"><el-option v-for="event in BUILTIN_EVENTS" :key="event.id" :label="event.label" :value="event.id" /></el-select></el-form-item>
              <el-form-item v-if="builtinEventById(action.ruleEvent)?.timed" label="持续时间(ms)"><el-input-number v-model="action.ruleEventTimeoutMs" aria-label="持续时间" :min="1" :max="BUILTIN_EVENT_MAX_TIMEOUT_MS" /><small class="field-help">单位(毫秒) 1秒 = 1000毫秒</small></el-form-item>
            </div>
            <div v-if="builtinEventById(action.ruleEvent)?.needs === 'killProcessName'" class="form-grid">
              <el-form-item label="进程名"><el-input v-model="action.killProcessName" aria-label="进程名" placeholder="app.exe" /></el-form-item>
            </div>
            <div v-if="builtinEventById(action.ruleEvent)?.needs === 'runExePath'" class="form-grid">
              <el-form-item label="应用程序路径"><el-input v-model="action.runExePath" aria-label="应用程序路径" placeholder="C:\\Program Files\\App\\app.exe" /></el-form-item>
            </div>
            <p v-if="builtinEventById(action.ruleEvent)" class="action-note" role="note">
              {{ builtinEventById(action.ruleEvent)?.detail }}
              <template v-if="action.ruleEvent === 'shutdown' || action.ruleEvent === 'restart' || action.ruleEvent === 'logoff' || action.ruleEvent === 'sleep'"> · 会立即影响整台电脑，请谨慎配置触发条件。</template>
            </p>
          </template>
          <template v-else-if="action.kind === 'tielian'">
            <div class="form-grid three">
              <el-form-item label="每次层数"><el-input-number v-model="action.delta" aria-label="每次层数" :min="1" :max="9999" /></el-form-item>
              <el-form-item label="效果"><el-select v-model="action.effect" aria-label="效果"><el-option label="增加" value="增加" /><el-option label="减少" value="减少" /></el-select></el-form-item>
              <el-form-item label="随机下限"><el-input-number v-model="action.countMin" aria-label="随机下限" :min="1" :max="9999" /></el-form-item>
              <el-form-item label="随机上限"><el-input-number v-model="action.countMax" aria-label="随机上限" :min="1" :max="9999" /></el-form-item>
            </div>
          </template>
          <template v-else-if="action.kind === 'trash'">
            <el-form-item label="垃圾素材路径"><AssetPathSelect v-model="action.image" kind="image" /></el-form-item>
            <div class="form-grid three">
              <el-form-item label="掉落数量"><el-input-number v-model="action.count" aria-label="掉落数量" :min="1" :max="100" /></el-form-item>
              <el-form-item label="垃圾桶"><el-input v-model="action.bin" aria-label="垃圾桶" placeholder="留空使用内置垃圾桶…" /></el-form-item>
              <el-form-item label="停留（毫秒）"><el-input-number v-model="action.durationMs" aria-label="停留时长" :min="100" :max="600000" /></el-form-item>
            </div>
          </template>
          <template v-else-if="action.kind === 'key' || action.kind === 'mouse'">
            <div class="form-grid three">
              <el-form-item label="窗口标题（可选）"><el-input :model-value="targetValue(action, 'title')" aria-label="目标窗口标题" @update:model-value="setTargetValue(action, 'title', $event)" placeholder="标题包含文字…" /></el-form-item>
              <el-form-item label="窗口类名（可选）"><el-input :model-value="targetValue(action, 'className')" aria-label="目标窗口类名" @update:model-value="setTargetValue(action, 'className', $event)" placeholder="精确类名…" /></el-form-item>
              <el-form-item label="进程名（可选）"><el-input :model-value="targetValue(action, 'processName')" aria-label="目标进程名" @update:model-value="setTargetValue(action, 'processName', $event)" placeholder="例如 obs64.exe…" /></el-form-item>
            </div>
            <el-form-item :label="action.kind === 'key' ? '按键步骤' : '鼠标步骤'">
              <div v-if="action.kind === 'key'" class="input-step-list">
                <div v-for="(step, stepIndex) in action.steps" :key="stepIndex" class="input-step-row">
                  <span class="input-step-index">{{ Number(stepIndex) + 1 }}</span>
                  <el-select :model-value="step.op" aria-label="按键操作" @update:model-value="changeKeyStep(action, Number(stepIndex), $event)">
                    <el-option label="按下" value="down" /><el-option label="松开" value="up" /><el-option label="点按" value="tap" /><el-option label="等待" value="wait" />
                  </el-select>
                  <el-select v-if="step.op !== 'wait'" v-model="step.key" filterable allow-create aria-label="按键" placeholder="选择或输入按键…">
                    <el-option v-for="key in keyOptions" :key="key" :label="key" :value="key" />
                  </el-select>
                  <el-input-number v-else v-model="step.ms" aria-label="等待时间（毫秒）" :min="0" :max="60000" />
                  <div class="input-step-actions">
                    <el-button link aria-label="上移按键步骤" @click.stop="moveInputStep(action, Number(stepIndex), -1)">↑</el-button>
                    <el-button link aria-label="下移按键步骤" @click.stop="moveInputStep(action, Number(stepIndex), 1)">↓</el-button>
                    <el-button link type="danger" aria-label="删除按键步骤" @click.stop="removeInputStep(action, Number(stepIndex))"><el-icon><Delete /></el-icon></el-button>
                  </div>
                </div>
                <el-button class="input-step-add" size="small" plain @click="addInputStep(action)"><el-icon><Plus /></el-icon>添加按键步骤</el-button>
              </div>
              <div v-else class="input-step-list">
                <div v-for="(step, stepIndex) in action.steps" :key="stepIndex" class="input-step-row mouse-step-row">
                  <span class="input-step-index">{{ Number(stepIndex) + 1 }}</span>
                  <el-select :model-value="step.op" aria-label="鼠标操作" @update:model-value="changeMouseStep(action, Number(stepIndex), $event)">
                    <el-option label="移动" value="move" /><el-option label="点击" value="click" /><el-option label="按下" value="down" /><el-option label="松开" value="up" /><el-option label="滚轮" value="scroll" /><el-option label="等待" value="wait" />
                  </el-select>
                  <el-input-number v-if="step.op === 'wait'" v-model="step.ms" aria-label="等待时间（毫秒）" :min="0" :max="60000" />
                  <el-input-number v-else-if="step.op === 'scroll'" v-model="step.delta" aria-label="滚轮距离" :min="-12000" :max="12000" :step="120" />
                  <div v-else class="mouse-step-fields">
                    <el-select v-if="step.op !== 'move'" v-model="step.button" aria-label="鼠标按键">
                      <el-option label="左键" value="left" /><el-option label="右键" value="right" /><el-option label="中键" value="middle" />
                    </el-select>
                    <el-input-number v-model="step.x" aria-label="鼠标 X 坐标" :min="-32768" :max="32767" placeholder="X" />
                    <el-input-number v-model="step.y" aria-label="鼠标 Y 坐标" :min="-32768" :max="32767" placeholder="Y" />
                  </div>
                  <div class="input-step-actions">
                    <el-button link aria-label="上移鼠标步骤" @click.stop="moveInputStep(action, Number(stepIndex), -1)">↑</el-button>
                    <el-button link aria-label="下移鼠标步骤" @click.stop="moveInputStep(action, Number(stepIndex), 1)">↓</el-button>
                    <el-button link type="danger" aria-label="删除鼠标步骤" @click.stop="removeInputStep(action, Number(stepIndex))"><el-icon><Delete /></el-icon></el-button>
                  </div>
                </div>
                <el-button class="input-step-add" size="small" plain @click="addInputStep(action)"><el-icon><Plus /></el-icon>添加鼠标步骤</el-button>
              </div>
            </el-form-item>
            <p v-if="action.kind === 'mouse'" class="action-note" role="note">点击、按下和松开的 X/Y 可留空以使用当前指针位置；移动步骤必须填写坐标。</p>
            <p class="action-note" role="note">匹配事件时会真实发送键鼠操作。目标条件留空时发送到当前前台窗口；填写目标后，会先匹配并聚焦该窗口。鼠标坐标相对目标窗口客户区；未指定目标时相对屏幕。Windows 上执行。</p>
          </template>
          <div class="form-grid">
            <el-form-item label="延迟（毫秒）"><el-input-number v-model="action.delayMs" aria-label="动作延迟（毫秒）" :min="0" :max="60000" /></el-form-item>
            <el-form-item label="重复次数"><el-input-number v-model="action.repeat" aria-label="动作重复次数" :min="1" :max="99" /></el-form-item>
          </div>
        </el-form>
            </div>
          </article>
        </div>
        <div v-else class="action-empty"><el-icon><Plus /></el-icon><span>还没有动作，点下面的按钮添加一个</span></div>
      </section>
    </div>
    <template #footer>
      <div class="dialog-footer-row">
        <div class="action-toolbar action-toolbar-footer"><el-dropdown size="small" @command="addAction"><el-button size="small" type="success">键/鼠<el-icon class="el-icon--right"><ArrowDown /></el-icon></el-button><template #dropdown><el-dropdown-menu><el-dropdown-item command="key">键盘</el-dropdown-item><el-dropdown-item command="mouse">鼠标</el-dropdown-item></el-dropdown-menu></template></el-dropdown><el-button size="small" type="info" @click="addAction('sleep')">等待</el-button><el-dropdown size="small" @command="addAction"><el-button size="small" type="primary">音/视频<el-icon class="el-icon--right"><ArrowDown /></el-icon></el-button><template #dropdown><el-dropdown-menu><el-dropdown-item command="video">视频</el-dropdown-item><el-dropdown-item command="audio">声音</el-dropdown-item></el-dropdown-menu></template></el-dropdown><el-button size="small" type="success" @click="addAction('showimage')">砸图片</el-button><el-button size="small" type="danger" @click="addAction('serial')">物理硬件</el-button><el-button size="small" type="primary" @click="addAction('randombox')">随机盲盒</el-button><el-button size="small" type="success" @click="addAction('app')">手机玩法</el-button><el-dropdown size="small" @command="addAction"><el-button size="small" type="primary">更多<el-icon class="el-icon--right"><ArrowDown /></el-icon></el-button><template #dropdown><el-dropdown-menu><el-dropdown-item command="gospeed">加速度</el-dropdown-item><el-dropdown-item command="obs_filter">OBS场景滤镜</el-dropdown-item><el-dropdown-item command="obs">OBS源</el-dropdown-item><el-dropdown-item command="rule">内置事件</el-dropdown-item><el-dropdown-item command="tielian">屏幕锁链</el-dropdown-item></el-dropdown-menu></template></el-dropdown></div>
        <div class="dialog-footer-actions"><el-button @click="resetDraft">重置</el-button><el-button @click="close">取消</el-button><el-button type="primary" @click="save">保存玩法</el-button></div>
      </div>
    </template>
  </el-dialog>

  <el-dialog v-model="limitsVisible" title="触发限制" width="min(560px, calc(100vw - 40px))" top="8vh" append-to-body draggable class="rule-limits-dialog">
    <p class="action-note" role="note">同一用户在该秒数内只触发一次；填 0 表示不限制。触发限制按用户计算，与「冷却」和「禁用组刷」互相独立。</p>
    <el-form label-position="top" class="editor-form">
      <div class="form-grid">
        <el-form-item label="同一用户，赠送礼物"><el-input-number v-model="draft.triggerLimits!.giftSecond" :min="0" :max="100000" class="limit-number" /><small class="field-help">秒内触发一次</small></el-form-item>
        <el-form-item label="同一用户，发送弹幕"><el-input-number v-model="draft.triggerLimits!.textSecond" :min="0" :max="100000" class="limit-number" /><small class="field-help">秒内触发一次</small></el-form-item>
        <el-form-item label="同一用户，点赞"><el-input-number v-model="draft.triggerLimits!.likeSecond" :min="0" :max="100000" class="limit-number" /><small class="field-help">秒内触发一次</small></el-form-item>
        <el-form-item label="同一用户，进场"><el-input-number v-model="draft.triggerLimits!.enterSecond" :min="0" :max="100000" class="limit-number" /><small class="field-help">秒内触发一次</small></el-form-item>
      </div>
    </el-form>
    <template #footer><el-button @click="draft.triggerLimits = normalizeLimits()">全部清零</el-button><el-button type="primary" @click="limitsVisible = false">完成</el-button></template>
  </el-dialog>
</template>
