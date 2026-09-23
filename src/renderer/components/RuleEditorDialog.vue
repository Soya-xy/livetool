<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { Delete, Plus, Sort } from '@element-plus/icons-vue'
import type { Action, EventKind, KeyStep, MouseStep, Rule } from '@shared/types'
import { useAppStore } from '../stores/app'

const props = defineProps<{ modelValue: boolean; rule?: Rule }>()
const emit = defineEmits<{ 'update:modelValue': [boolean]; saved: [] }>()
const store = useAppStore()
const draft = ref<Rule & { actions: any[] }>(blankRule() as Rule & { actions: any[] })
const keywordText = ref('')
const giftText = ref('')
const userText = ref('')
const actionJson = ref<Record<string, string>>({})
const activeAction = ref('')
const dragIndex = ref<number | null>(null)
const title = computed(() => props.rule ? '修改玩法' : '增加新的玩法')
const eventKinds: Array<{ label: string; value: EventKind }> = [{ label: '礼物', value: 'gift' }, { label: '弹幕', value: 'chat' }, { label: '点赞', value: 'like' }, { label: '关注', value: 'follow' }, { label: '进场', value: 'enter' }, { label: '系统', value: 'system' }]

watch(() => props.modelValue, (visible) => { if (visible) loadDraft() })

function loadDraft(): void {
  draft.value = props.rule ? structuredClone(props.rule) : blankRule()
  keywordText.value = draft.value.trigger.keywords?.join(', ') ?? ''
  giftText.value = draft.value.trigger.giftNames?.join(', ') ?? ''
  userText.value = draft.value.trigger.users?.join(', ') ?? ''
  actionJson.value = {}
  for (const action of draft.value.actions) {
    if (action.kind === 'key' || action.kind === 'mouse') actionJson.value[action.id] = JSON.stringify(action.steps)
  }
}
function blankRule(): Rule {
  return { id: crypto.randomUUID(), name: '', enabled: true, priority: 1, trigger: { kinds: ['gift'], keywordMode: 'contains' }, cooldownMs: 0, probability: 1, concurrency: 'queue', actions: [] }
}
function close(): void { emit('update:modelValue', false) }
function addAction(kind: Action['kind'] = 'drop'): void { const action = createAction(kind); draft.value.actions.push(action); activeAction.value = action.id; if ((action.kind === 'key' || action.kind === 'mouse')) actionJson.value[action.id] = JSON.stringify(action.steps) }
function createAction(kind: Action['kind']): Action {
  const base = { id: crypto.randomUUID(), delayMs: 0, repeat: 1 }
  if (kind === 'video') return { ...base, kind, path: 'videos/功德狗.mp4', lane: 1, durationMs: 8000, loop: false, chroma: { enabled: true, color: '#00ff00', similarity: 0.35, smoothness: 0.12 } }
  if (kind === 'audio') return { ...base, kind, path: 'voices/测试音效.mp3', volume: 0.8, interrupt: false, loop: false }
  if (kind === 'drop') return { ...base, kind, image: 'images/平底锅.png', count: 5, gravity: 1800, bounce: 0.45, durationMs: 3500 }
  if (kind === 'slot') return { ...base, kind, theme: 'default', pool: ['一等奖', '二等奖', '谢谢参与'], weights: [1, 10, 89] }
  if (kind === 'serial') return { ...base, kind, port: 'VIRTUAL-COM1', baud: 9600, onBytes: [1, 2, 3], offBytes: [1, 2, 0], pulseMs: 800 }
  if (kind === 'obs') return { ...base, kind, command: 'SetInputMute', args: { inputName: '直播音效', inputMuted: 'false' } }
  if (kind === 'key') return { ...base, kind, steps: [{ op: 'down', key: 'CTRL' }, { op: 'tap', key: '1' }, { op: 'up', key: 'CTRL' }] as KeyStep[] }
  return { ...base, kind: 'mouse', steps: [{ op: 'move', x: 100, y: 100 }, { op: 'click', button: 'left' }] as MouseStep[] }
}
function removeAction(id: string): void { draft.value.actions = draft.value.actions.filter((action) => action.id !== id) }
function moveAction(index: number, direction: -1 | 1): void { const target = index + direction; if (target < 0 || target >= draft.value.actions.length) return; const [item] = draft.value.actions.splice(index, 1); draft.value.actions.splice(target, 0, item) }
function startDrag(index: number): void { dragIndex.value = index }
function dropAction(index: number): void { const source = dragIndex.value; dragIndex.value = null; if (source === null || source === index) return; const [item] = draft.value.actions.splice(source, 1); draft.value.actions.splice(index, 0, item) }
function setActionKind(action: Action, kind: Action['kind']): void { const next = createAction(kind); next.id = action.id; const index = draft.value.actions.findIndex((item: Action) => item.id === action.id); if (index >= 0) draft.value.actions.splice(index, 1, next); if (next.kind === 'key' || next.kind === 'mouse') actionJson.value[action.id] = JSON.stringify(next.steps) }
function actionText(action: Action): string { switch (action.kind) { case 'key': case 'mouse': return '键鼠步骤'; case 'video': case 'audio': return action.path; case 'drop': return action.image; case 'slot': return '加权抽取'; case 'serial': return action.port; case 'obs': return action.command } }
function parseJsonSteps(action: Action): void { if (action.kind !== 'key' && action.kind !== 'mouse') return; try { action.steps = JSON.parse(actionJson.value[action.id] || '[]') } catch { ElMessage.warning('键鼠步骤 JSON 格式不正确') } }
function updateSlotPool(action: Extract<Action, { kind: 'slot' }>, value: string): void { action.pool = value.split(/[,，]/).map((item) => item.trim()).filter(Boolean) }
function updateSlotWeights(action: Extract<Action, { kind: 'slot' }>, value: string): void { action.weights = value.split(/[,，]/).map((item) => Number(item.trim())).filter((item) => Number.isFinite(item) && item >= 0) }
function updateSerialBytes(action: Extract<Action, { kind: 'serial' }>, field: 'onBytes' | 'offBytes', value: string): void { action[field] = value.split(/[,，]/).map((item) => Number(item.trim())).filter((item) => Number.isInteger(item) && item >= 0 && item <= 255) }
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
  for (const action of draft.value.actions) parseJsonSteps(action)
  await store.saveRule(structuredClone(draft.value))
  ElMessage.success('玩法已保存')
  emit('saved')
  close()
}
</script>

<template>
  <el-dialog :model-value="modelValue" :title="title" width="760px" top="5vh" class="rule-editor-dialog" destroy-on-close @update:model-value="emit('update:modelValue', $event)">
    <div class="editor-layout">
      <section class="editor-section"><div class="editor-section-title"><span>01</span><div><h3>基本信息</h3><small>给这套互动动作一个容易识别的名字</small></div></div><el-form label-position="top" class="editor-form"><el-form-item label="玩法名称"><el-input v-model="draft.name" maxlength="40" show-word-limit placeholder="例如：啤酒掉落 / 弹幕抽奖" /></el-form-item><div class="form-grid"><el-form-item label="优先级"><el-input-number v-model="draft.priority" :min="0" :max="999" /></el-form-item><el-form-item label="并发策略"><el-select v-model="draft.concurrency"><el-option label="排队执行" value="queue" /><el-option label="并行执行" value="parallel" /><el-option label="替换旧动作" value="replace" /><el-option label="互斥执行" value="exclusive" /></el-select></el-form-item></div><el-switch v-model="draft.enabled" inline-prompt active-text="开启" inactive-text="关闭" /></el-form></section>
      <section class="editor-section"><div class="editor-section-title"><span>02</span><div><h3>触发条件</h3><small>礼物、弹幕、点赞和关注都可以进入同一条规则链</small></div></div><el-form label-position="top" class="editor-form"><el-form-item label="事件类型"><el-checkbox-group v-model="draft.trigger.kinds"><el-checkbox v-for="item in eventKinds" :key="item.value" :value="item.value">{{ item.label }}</el-checkbox></el-checkbox-group></el-form-item><div class="form-grid"><el-form-item label="礼物名称（逗号分隔）"><el-input v-model="giftText" placeholder="啤酒, 小心心" /></el-form-item><el-form-item label="关键词（逗号分隔）"><el-input v-model="keywordText" placeholder="666, 欧皇" /></el-form-item></div><div class="form-grid"><el-form-item label="关键词模式"><el-select v-model="draft.trigger.keywordMode"><el-option label="包含" value="contains" /><el-option label="完全匹配" value="exact" /><el-option label="正则表达式" value="regex" /></el-select></el-form-item><el-form-item label="最小数量"><el-input-number v-model="draft.trigger.minCount" :min="0" :max="99999" /></el-form-item></div></el-form></section>
      <section class="editor-section"><div class="editor-section-title"><span>03</span><div><h3>过滤与节奏</h3><small>避免重复触发，也可以让互动带一点随机性</small></div></div><el-form label-position="top" class="editor-form"><div class="form-grid three"><el-form-item label="冷却（毫秒）"><el-input-number v-model="draft.cooldownMs" :min="0" :max="86400000" /></el-form-item><el-form-item label="命中概率"><el-input-number v-model="draft.probability" :min="0" :max="1" :step="0.05" /></el-form-item><el-form-item label="来源"><el-select v-model="draft.trigger.source" multiple collapse-tags placeholder="全部平台"><el-option label="全部 / 不限" value="" /><el-option label="本地模拟器" value="simulator" /><el-option label="抖音" value="douyin" /><el-option label="快手" value="kuaishou" /><el-option label="B站" value="bilibili" /></el-select></el-form-item></div><div class="form-grid"><el-form-item label="用户白名单"><el-input v-model="userText" placeholder="昵称，逗号分隔" /></el-form-item></div></el-form></section>
      <section class="editor-section actions-editor"><div class="editor-section-title"><span>04</span><div><h3>动作编排</h3><small>按顺序执行；每个动作可设置延迟和重复次数</small></div><el-button size="small" type="primary" plain @click="addAction()"><el-icon><Plus /></el-icon>添加动作</el-button></div><div v-if="draft.actions.length" class="action-list"><div v-for="(action, index) in draft.actions" :key="action.id" class="action-row" :class="{ focused: activeAction === action.id }" draggable="true" @dragstart="startDrag(index)" @dragover.prevent @drop="dropAction(index)" @click="activeAction = action.id"><div class="action-order"><el-icon><Sort /></el-icon><span>{{ String(index + 1).padStart(2, '0') }}</span></div><el-select :model-value="action.kind" class="action-kind" @update:model-value="setActionKind(action, $event)"><el-option label="键盘" value="key" /><el-option label="鼠标" value="mouse" /><el-option label="绿幕视频" value="video" /><el-option label="声音" value="audio" /><el-option label="砸落物" value="drop" /><el-option label="水果机" value="slot" /><el-option label="串口脉冲" value="serial" /><el-option label="OBS 联动" value="obs" /></el-select><span class="action-summary">{{ actionText(action) }}</span><el-input-number v-model="action.delayMs" :min="0" :max="60000" controls-position="right" class="mini-number" /><span class="unit-label">ms</span><el-input-number v-model="action.repeat" :min="1" :max="99" controls-position="right" class="mini-number repeat-number" /><span class="unit-label">次</span><div class="row-mini-actions"><el-button link @click.stop="moveAction(index, -1)">↑</el-button><el-button link @click.stop="moveAction(index, 1)">↓</el-button><el-button link type="danger" @click.stop="removeAction(action.id)"><el-icon><Delete /></el-icon></el-button></div></div></div><div v-else class="action-empty"><el-icon><Plus /></el-icon><span>还没有动作，添加一个视频、声音或砸落物</span></div><div v-for="action in draft.actions" :key="`${action.id}-config`" class="action-config" v-show="activeAction === action.id"><template v-if="action.kind === 'video'"><el-input v-model="action.path" placeholder="相对素材路径，例如 videos/功德狗.mp4" /><div class="form-grid"><el-input-number v-model="action.lane" :min="1" :max="8" /><el-input-number v-model="action.durationMs" :min="100" :max="600000" /><el-switch v-model="action.loop" active-text="循环" /></div></template><template v-else-if="action.kind === 'audio'"><el-input v-model="action.path" placeholder="相对素材路径，例如 voices/测试音效.mp3" /><div class="form-grid"><el-slider v-model="action.volume" :min="0" :max="1" :step="0.05" show-input /><el-switch v-model="action.interrupt" active-text="打断当前声音" /></div></template><template v-else-if="action.kind === 'drop'"><el-input v-model="action.image" placeholder="相对素材路径，例如 images/平底锅.png" /><div class="form-grid three"><el-input-number v-model="action.count" :min="1" :max="100" /><el-input-number v-model="action.gravity" :min="0" :max="5000" /><el-input-number v-model="action.bounce" :min="0" :max="1" :step="0.05" /></div></template><template v-else-if="action.kind === 'slot'"><el-input v-model="action.theme" placeholder="主题名" /><el-input :model-value="action.pool.join(', ')" @update:model-value="updateSlotPool(action, $event)" placeholder="奖项池，用逗号分隔" /><el-input :model-value="action.weights.join(', ')" @update:model-value="updateSlotWeights(action, $event)" placeholder="权重，用逗号分隔" /></template><template v-else-if="action.kind === 'serial'"><div class="form-grid three"><el-input v-model="action.port" /><el-input-number v-model="action.pulseMs" :min="1" :max="60000" /><el-input :model-value="action.onBytes.join(', ')" @update:model-value="updateSerialBytes(action, 'onBytes', $event)" placeholder="开启字节，如 1,2,3" /><el-input :model-value="action.offBytes.join(', ')" @update:model-value="updateSerialBytes(action, 'offBytes', $event)" placeholder="关闭字节，如 1,2,0" /></div></template><template v-else-if="action.kind === 'obs'"><el-input v-model="action.command" placeholder="OBS 请求名称" /><el-input :model-value="JSON.stringify(action.args ?? {})" @update:model-value="updateObsArgs(action, $event)" placeholder="参数 JSON，例如 {&quot;sceneName&quot;:&quot;直播&quot;}" /></template><template v-else><el-input v-model="actionJson[action.id]" type="textarea" :rows="2" placeholder="键鼠步骤 JSON，例如 [{&quot;op&quot;:&quot;tap&quot;,&quot;key&quot;:&quot;1&quot;}]" /></template></div></section>
      <section class="editor-section preview-section"><div class="editor-section-title"><span>05</span><div><h3>测试预览</h3><small>保存后可从控制中心发送对应类型的模拟事件</small></div></div><div class="preview-lane"><div class="preview-dot" /><span>{{ draft.name || '未命名玩法' }}</span><i /> <span>{{ draft.trigger.kinds.join(' / ') || '未设置触发' }}</span><i /> <span>{{ draft.actions.length }} 个动作</span></div></section>
    </div>
    <template #footer><el-button @click="close">取消</el-button><el-button type="primary" @click="save">保存玩法</el-button></template>
  </el-dialog>
</template>
