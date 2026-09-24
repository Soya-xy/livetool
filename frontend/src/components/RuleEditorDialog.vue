<script setup lang="ts">
import { computed, ref, toRaw, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { Delete, Plus, Sort } from '@element-plus/icons-vue'
import type { Action, EventKind, KeyStep, MouseStep, Rule } from '@shared/types'
import { api } from '../services/api'
import { useAppStore } from '../stores/app'

const props = defineProps<{ modelValue: boolean; rule?: Rule }>()
const emit = defineEmits<{ 'update:modelValue': [boolean]; saved: [] }>()
const store = useAppStore()
const draft = ref<Rule & { actions: any[] }>(blankRule() as Rule & { actions: any[] })
const keywordText = ref('')
const giftText = ref('')
const userText = ref('')
const serialPorts = ref<Array<{ path: string; manufacturer?: string; virtual: boolean }>>([])
const serialPortsLoading = ref(false)
const serialStopLoading = ref(false)
const serialBytesText = ref<Record<string, { onBytes: string; offBytes: string }>>({})
const activeAction = ref('')
const dragIndex = ref<number | null>(null)
const keyOptions = [...'ABCDEFGHIJKLMNOPQRSTUVWXYZ', ...'0123456789', ...Array.from({ length: 24 }, (_, index) => `F${index + 1}`), 'CTRL', 'CONTROL', 'SHIFT', 'ALT', 'WIN', 'META', 'ENTER', 'RETURN', 'SPACE', 'SPACEBAR', 'TAB', 'ESC', 'ESCAPE', 'BACKSPACE', 'DELETE', 'INSERT', 'HOME', 'END', 'PAGEUP', 'PAGEDOWN', 'UP', 'DOWN', 'LEFT', 'RIGHT', 'CAPSLOCK', 'NUMLOCK', 'SCROLLLOCK', 'PAUSE', 'PRINTSCREEN', ...Array.from({ length: 10 }, (_, index) => `NUMPAD${index}`)]
const punctuationKeys = new Set(['-', '=', '[', ']', '\\', ';', "'", '`', ',', '.', '/'])
const title = computed(() => props.rule ? '修改玩法' : '增加新的玩法')
const eventKinds: Array<{ label: string; value: EventKind }> = [{ label: '礼物', value: 'gift' }, { label: '弹幕', value: 'chat' }, { label: '点赞', value: 'like' }, { label: '关注', value: 'follow' }, { label: '进场', value: 'enter' }, { label: '系统', value: 'system' }]

watch(() => props.modelValue, (visible) => { if (visible) { loadDraft(); void refreshSerialPorts() } })

function loadDraft(): void {
  // props.rule / draft 都是 Vue 响应式代理，structuredClone 无法克隆 Proxy，先取原始对象。
  draft.value = props.rule ? structuredClone(toRaw(props.rule)) : blankRule()
  keywordText.value = draft.value.trigger.keywords?.join(', ') ?? ''
  giftText.value = draft.value.trigger.giftNames?.join(', ') ?? ''
  userText.value = draft.value.trigger.users?.join(', ') ?? ''
  serialBytesText.value = {}
  for (const action of draft.value.actions) {
    if (action.kind === 'serial') serialBytesText.value[action.id] = { onBytes: action.onBytes.join(', '), offBytes: action.offBytes.join(', ') }
  }
}
function blankRule(): Rule {
  return { id: crypto.randomUUID(), name: '', enabled: true, priority: 1, trigger: { kinds: ['gift'], keywordMode: 'contains' }, cooldownMs: 0, probability: 1, concurrency: 'queue', actions: [] }
}
function close(): void { emit('update:modelValue', false) }
function addAction(kind: Action['kind'] = 'drop'): void { const action = createAction(kind); draft.value.actions.push(action); activeAction.value = action.id; if (action.kind === 'serial') serialBytesText.value[action.id] = { onBytes: '', offBytes: '' } }
function createAction(kind: Action['kind']): Action {
  const base = { id: crypto.randomUUID(), delayMs: 0, repeat: 1 }
  if (kind === 'video') return { ...base, kind, path: 'videos/功德狗.mp4', lane: 1, durationMs: 8000, loop: false, chroma: { enabled: true, color: '#00ff00', similarity: 0.35, smoothness: 0.12 } }
  if (kind === 'audio') return { ...base, kind, path: 'voices/测试音效.mp3', volume: 0.8, interrupt: false, loop: false }
  if (kind === 'drop') return { ...base, kind, image: 'images/平底锅.png', count: 5, gravity: 1800, bounce: 0.45, durationMs: 3500 }
  if (kind === 'slot') return { ...base, kind, theme: 'default', pool: ['一等奖', '二等奖', '谢谢参与'], weights: [1, 10, 89] }
  if (kind === 'serial') return { ...base, kind, port: '', baud: 9600, onBytes: [], offBytes: [], pulseMs: 800 }
  if (kind === 'obs') return { ...base, kind, command: 'SetInputMute', args: { inputName: '直播音效', inputMuted: 'false' } }
  if (kind === 'key') return { ...base, kind, steps: [{ op: 'down', key: 'CTRL' }, { op: 'tap', key: '1' }, { op: 'up', key: 'CTRL' }] as KeyStep[] }
  return { ...base, kind: 'mouse', steps: [{ op: 'move', x: 100, y: 100 }, { op: 'click', button: 'left' }] as MouseStep[] }
}
function removeAction(id: string): void { draft.value.actions = draft.value.actions.filter((action) => action.id !== id) }
function moveAction(index: number, direction: -1 | 1): void { const target = index + direction; if (target < 0 || target >= draft.value.actions.length) return; const [item] = draft.value.actions.splice(index, 1); draft.value.actions.splice(target, 0, item) }
function startDrag(index: number): void { dragIndex.value = index }
function dropAction(index: number): void { const source = dragIndex.value; dragIndex.value = null; if (source === null || source === index) return; const [item] = draft.value.actions.splice(source, 1); draft.value.actions.splice(index, 0, item) }
function setActionKind(action: Action, kind: Action['kind']): void { const next = createAction(kind); next.id = action.id; const index = draft.value.actions.findIndex((item: Action) => item.id === action.id); if (index >= 0) draft.value.actions.splice(index, 1, next); if (next.kind === 'serial') serialBytesText.value[action.id] = { onBytes: '', offBytes: '' } }
function actionText(action: Action): string { switch (action.kind) { case 'key': return '键盘步骤'; case 'mouse': return '鼠标步骤'; case 'video': case 'audio': return action.path; case 'drop': return action.image; case 'slot': return '加权抽取'; case 'serial': return action.port || '未选串口'; case 'obs': return action.command } }
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
function updateSlotPool(action: Extract<Action, { kind: 'slot' }>, value: string): void { action.pool = value.split(/[,，]/).map((item) => item.trim()).filter(Boolean) }
function updateSlotWeights(action: Extract<Action, { kind: 'slot' }>, value: string): void { action.weights = value.split(/[,，]/).map((item) => Number(item.trim())).filter((item) => Number.isFinite(item) && item >= 0) }
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
  if (!draft.value.name.trim()) { ElMessage.warning('请填写玩法名称'); return }
  if (!draft.value.trigger.kinds.length) { ElMessage.warning('至少选择一种触发类型'); return }
  draft.value.trigger.keywords = keywordText.value.split(/[,，]/).map((item) => item.trim()).filter(Boolean)
  draft.value.trigger.giftNames = giftText.value.split(/[,，]/).map((item) => item.trim()).filter(Boolean)
  draft.value.trigger.users = userText.value.split(/[,，]/).map((item) => item.trim()).filter(Boolean)
  const sources = draft.value.trigger.source?.filter(Boolean) ?? []
  draft.value.trigger.source = sources.length ? sources : undefined
  if (!validateInputActions()) return
  if (!validateSerialActions()) return
  await store.saveRule(structuredClone(toRaw(draft.value)))
  ElMessage.success('玩法已保存')
  emit('saved')
  close()
}
</script>

<template>
  <el-dialog :model-value="modelValue" :title="title" width="min(700px, calc(100vw - 40px))" top="4vh" class="rule-editor-dialog" destroy-on-close @update:model-value="emit('update:modelValue', $event)">
    <div class="editor-layout">
      <section class="editor-section"><div class="editor-section-title"><span>01</span><div><h3>基本信息</h3><small>给这套互动动作一个容易识别的名字</small></div></div><el-form label-position="top" class="editor-form"><el-form-item label="玩法名称"><el-input v-model="draft.name" maxlength="40" show-word-limit placeholder="例如：啤酒掉落 / 弹幕抽奖" /></el-form-item><div class="form-grid"><el-form-item label="优先级"><el-input-number v-model="draft.priority" :min="0" :max="999" /></el-form-item><el-form-item label="并发策略"><el-select v-model="draft.concurrency"><el-option label="排队执行" value="queue" /><el-option label="并行执行" value="parallel" /><el-option label="替换旧动作" value="replace" /><el-option label="互斥执行" value="exclusive" /></el-select></el-form-item></div><el-switch v-model="draft.enabled" inline-prompt active-text="开启" inactive-text="关闭" /></el-form></section>
      <section class="editor-section"><div class="editor-section-title"><span>02</span><div><h3>触发条件</h3><small>礼物、弹幕、点赞和关注都可以进入同一条规则链</small></div></div><el-form label-position="top" class="editor-form"><el-form-item label="事件类型"><el-checkbox-group v-model="draft.trigger.kinds"><el-checkbox v-for="item in eventKinds" :key="item.value" :value="item.value">{{ item.label }}</el-checkbox></el-checkbox-group></el-form-item><div class="form-grid"><el-form-item label="礼物名称（逗号分隔）"><el-input v-model="giftText" placeholder="啤酒, 小心心" /></el-form-item><el-form-item label="关键词（逗号分隔）"><el-input v-model="keywordText" placeholder="666, 欧皇" /></el-form-item></div><div class="form-grid"><el-form-item label="关键词模式"><el-select v-model="draft.trigger.keywordMode"><el-option label="包含" value="contains" /><el-option label="完全匹配" value="exact" /><el-option label="正则表达式" value="regex" /></el-select></el-form-item><el-form-item label="最小数量"><el-input-number v-model="draft.trigger.minCount" :min="0" :max="99999" /></el-form-item></div></el-form></section>
      <section class="editor-section"><div class="editor-section-title"><span>03</span><div><h3>过滤与节奏</h3><small>避免重复触发，也可以让互动带一点随机性</small></div></div><el-form label-position="top" class="editor-form"><div class="form-grid three"><el-form-item label="冷却（毫秒）"><el-input-number v-model="draft.cooldownMs" :min="0" :max="86400000" /></el-form-item><el-form-item label="命中概率"><el-input-number v-model="draft.probability" :min="0" :max="1" :step="0.05" /></el-form-item><el-form-item label="来源"><el-select v-model="draft.trigger.source" multiple collapse-tags placeholder="全部平台"><el-option label="全部 / 不限" value="" /><el-option label="本地模拟器" value="simulator" /><el-option label="抖音" value="douyin" /><el-option label="快手" value="kuaishou" /><el-option label="B站" value="bilibili" /></el-select></el-form-item></div><div class="form-grid"><el-form-item label="用户白名单"><el-input v-model="userText" placeholder="昵称，逗号分隔" /></el-form-item></div></el-form></section>
      <section class="editor-section actions-editor"><div class="editor-section-title"><span>04</span><div><h3>动作编排</h3><small>按顺序执行；每个动作可设置延迟和重复次数</small></div><el-button size="small" type="primary" plain @click="addAction()"><el-icon><Plus /></el-icon>添加动作</el-button></div><div v-if="draft.actions.length" class="action-list"><div v-for="(action, index) in draft.actions" :key="action.id" class="action-row" :class="{ focused: activeAction === action.id }" draggable="true" @dragstart="startDrag(index)" @dragover.prevent @drop="dropAction(index)" @focusin="activeAction = action.id"><div class="action-order"><el-icon><Sort /></el-icon><span>{{ String(index + 1).padStart(2, '0') }}</span></div><el-select :model-value="action.kind" class="action-kind" aria-label="动作类型" @update:model-value="setActionKind(action, $event)"><el-option label="键盘" value="key" /><el-option label="鼠标" value="mouse" /><el-option label="绿幕视频" value="video" /><el-option label="声音" value="audio" /><el-option label="砸落物" value="drop" /><el-option label="水果机" value="slot" /><el-option label="串口脉冲" value="serial" /><el-option label="OBS 联动" value="obs" /></el-select><button class="action-summary" type="button" :aria-label="`选择动作配置：${actionText(action)}`" :aria-controls="`action-config-${action.id}`" :aria-expanded="activeAction === action.id" @click="activeAction = action.id">{{ actionText(action) }}</button><el-input-number v-model="action.delayMs" aria-label="动作延迟（毫秒）" :min="0" :max="60000" controls-position="right" class="mini-number" /><span class="unit-label">ms</span><el-input-number v-model="action.repeat" aria-label="动作重复次数" :min="1" :max="99" controls-position="right" class="mini-number repeat-number" /><span class="unit-label">次</span><div class="row-mini-actions"><el-button link aria-label="上移动作" @click.stop="moveAction(index, -1)">↑</el-button><el-button link aria-label="下移动作" @click.stop="moveAction(index, 1)">↓</el-button><el-button link type="danger" aria-label="删除动作" @click.stop="removeAction(action.id)"><el-icon><Delete /></el-icon></el-button></div></div></div><div v-else class="action-empty"><el-icon><Plus /></el-icon><span>还没有动作，添加一个视频、声音或砸落物</span></div><div v-for="action in draft.actions" :key="action.id + '-config'" class="action-config" :id="`action-config-${action.id}`" v-show="activeAction === action.id">
        <el-form label-position="top" class="action-config-form">
          <template v-if="action.kind === 'video'">
            <el-form-item label="视频素材路径"><el-input v-model="action.path" aria-label="视频素材路径" placeholder="相对路径，例如 videos/效果.mp4…" /></el-form-item>
            <div class="form-grid three">
              <el-form-item label="播放通道"><el-input-number v-model="action.lane" aria-label="播放通道" :min="1" :max="8" /></el-form-item>
              <el-form-item label="播放时长（毫秒）"><el-input-number v-model="action.durationMs" aria-label="播放时长" :min="100" :max="600000" /></el-form-item>
              <el-form-item label="循环播放"><el-switch v-model="action.loop" aria-label="循环播放" /></el-form-item>
            </div>
          </template>
          <template v-else-if="action.kind === 'audio'">
            <el-form-item label="音频素材路径"><el-input v-model="action.path" aria-label="音频素材路径" placeholder="相对路径，例如 voices/提示音.mp3…" /></el-form-item>
            <div class="form-grid">
              <el-form-item label="音量"><el-slider v-model="action.volume" aria-label="音量" :min="0" :max="1" :step="0.05" show-input /></el-form-item>
              <el-form-item label="打断当前声音"><el-switch v-model="action.interrupt" aria-label="打断当前声音" /></el-form-item>
            </div>
          </template>
          <template v-else-if="action.kind === 'drop'">
            <el-form-item label="图片素材路径"><el-input v-model="action.image" aria-label="图片素材路径" placeholder="相对路径，例如 images/平底锅.png…" /></el-form-item>
            <div class="form-grid three">
              <el-form-item label="数量"><el-input-number v-model="action.count" aria-label="数量" :min="1" :max="100" /></el-form-item>
              <el-form-item label="重力"><el-input-number v-model="action.gravity" aria-label="重力" :min="0" :max="5000" /></el-form-item>
              <el-form-item label="弹跳系数"><el-input-number v-model="action.bounce" aria-label="弹跳系数" :min="0" :max="1" :step="0.05" /></el-form-item>
            </div>
          </template>
          <template v-else-if="action.kind === 'slot'">
            <div class="form-grid three">
              <el-form-item label="主题"><el-input v-model="action.theme" aria-label="主题" placeholder="例如 default…" /></el-form-item>
              <el-form-item label="奖项池"><el-input :model-value="action.pool.join(', ')" aria-label="奖项池" @update:model-value="updateSlotPool(action, $event)" placeholder="逗号分隔…" /></el-form-item>
              <el-form-item label="权重"><el-input :model-value="action.weights.join(', ')" aria-label="奖项权重" @update:model-value="updateSlotWeights(action, $event)" placeholder="与奖项顺序对应…" /></el-form-item>
            </div>
          </template>
          <template v-else-if="action.kind === 'serial'">
            <div class="serial-port-row">
              <el-form-item label="实际设备端口">
                <el-select v-model="action.port" filterable allow-create clearable aria-label="实际设备端口" placeholder="选择或输入串口…">
                  <el-option v-for="port in serialPorts" :key="port.path" :label="port.manufacturer ? port.path + ' · ' + port.manufacturer : port.path" :value="port.path" />
                </el-select>
              </el-form-item>
              <el-button size="small" :loading="serialPortsLoading" @click="refreshSerialPorts">刷新端口</el-button>
            </div>
            <div class="form-grid three">
              <el-form-item label="波特率"><el-input-number v-model="action.baud" aria-label="波特率" :min="300" :max="4000000" controls-position="right" /></el-form-item>
              <el-form-item label="脉冲时间（毫秒）"><el-input-number v-model="action.pulseMs" aria-label="脉冲时间" :min="1" :max="60000" controls-position="right" /></el-form-item>
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
                  <el-input-number v-else v-model="step.ms" aria-label="等待时间（毫秒）" :min="0" :max="60000" controls-position="right" />
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
                  <el-input-number v-if="step.op === 'wait'" v-model="step.ms" aria-label="等待时间（毫秒）" :min="0" :max="60000" controls-position="right" />
                  <el-input-number v-else-if="step.op === 'scroll'" v-model="step.delta" aria-label="滚轮距离" :min="-12000" :max="12000" :step="120" controls-position="right" />
                  <div v-else class="mouse-step-fields">
                    <el-select v-if="step.op !== 'move'" v-model="step.button" aria-label="鼠标按键">
                      <el-option label="左键" value="left" /><el-option label="右键" value="right" /><el-option label="中键" value="middle" />
                    </el-select>
                    <el-input-number v-model="step.x" aria-label="鼠标 X 坐标" :min="-32768" :max="32767" controls-position="right" placeholder="X" />
                    <el-input-number v-model="step.y" aria-label="鼠标 Y 坐标" :min="-32768" :max="32767" controls-position="right" placeholder="Y" />
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
        </el-form>
      </div></section>
      <section class="editor-section preview-section"><div class="editor-section-title"><span>05</span><div><h3>测试预览</h3><small>模拟事件会执行已配置的动作；键鼠与串口动作会作用于真实窗口或设备</small></div></div><div class="preview-lane"><div class="preview-dot" /><span>{{ draft.name || '未命名玩法' }}</span><i /> <span>{{ draft.trigger.kinds.join(' / ') || '未设置触发' }}</span><i /> <span>{{ draft.actions.length }} 个动作</span></div></section>
    </div>
    <template #footer><el-button @click="close">取消</el-button><el-button type="primary" @click="save">保存玩法</el-button></template>
  </el-dialog>
</template>
