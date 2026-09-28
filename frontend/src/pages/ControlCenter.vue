<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { CopyDocument, Delete, Download, Edit, MagicStick, MoreFilled, Plus, Search, Top, Upload, VideoPlay } from '@element-plus/icons-vue'
import type { EventKind, LiveEvent, Rule } from '@shared/types'
import { api } from '@/services/api'
import { useAppStore } from '../stores/app'
import RuleEditorDialog from '../components/RuleEditorDialog.vue'

const store = useAppStore()
const search = ref('')
const editorVisible = ref(false)
const editingRule = ref<Rule | undefined>()
const simulatorVisible = ref(false)
const simulatorKind = ref<EventKind>('gift')
const simulatorText = ref('啤酒')
const simulatorCount = ref(1)
const simulatorUser = ref('模拟观众')
const expiresDate = ref('')
const runtimeBusy = ref(false)
const shortcutModifier = /Mac|iPhone|iPad/.test(navigator.platform) ? '⌘' : 'Ctrl'

// 置顶的玩法固定在最前，其余按优先级从大到小排列，与后台匹配顺序一致。
const filteredRules = computed(() => store.rules
  .filter((rule) => !search.value || rule.name.toLocaleLowerCase().includes(search.value.toLocaleLowerCase()))
  .slice()
  .sort((left, right) => {
    if (Boolean(left.pinned) !== Boolean(right.pinned)) return left.pinned ? -1 : 1
    if (left.priority !== right.priority) return right.priority - left.priority
    return (left.updatedAt ?? 0) - (right.updatedAt ?? 0)
  }))

function describeRuleTrigger(rule: Rule): string {
  const labels: Record<string, string> = { gift: '礼物', chat: '弹幕', like: '点赞', follow: '关注', enter: '进场', system: '系统' }
  const kinds = rule.trigger.kinds.map((kind) => labels[kind]).join(' / ')
  const details = [...(rule.trigger.giftNames ?? []), ...(rule.trigger.keywords ?? [])]
  return details.length ? `${kinds} · ${details.join('、')}` : kinds || '未设置触发'
}
function triggerHeadline(rule: Rule): string {
  const details = [...(rule.trigger.giftNames ?? []), ...(rule.trigger.keywords ?? [])].filter(Boolean)
  if (!details.length) return '全部'
  return details.length > 1 ? `${details[0]} …` : details[0]
}
function hasSound(rule: Rule): boolean {
  return rule.actions.some((action) => action.kind === 'audio')
}
function limitSummary(rule: Rule): string {
  const limits = rule.triggerLimits
  if (!limits) return ''
  const seconds = Math.max(limits.giftSecond ?? 0, limits.textSecond ?? 0, limits.likeSecond ?? 0, limits.enterSecond ?? 0)
  return seconds > 0 ? `${seconds}s` : ''
}

onMounted(async () => {
  await store.init()
  try {
    const status = await api.auth.status()
    expiresDate.value = formatExpiry(status.expiresAt)
  } catch { expiresDate.value = '' }
})

function formatExpiry(value?: string): string {
  if (!value) return ''
  const parsed = new Date(value)
  if (Number.isNaN(parsed.getTime())) return value
  return `${parsed.getFullYear()}-${String(parsed.getMonth() + 1).padStart(2, '0')}-${String(parsed.getDate()).padStart(2, '0')}`
}

function addRule(): void { editingRule.value = undefined; editorVisible.value = true }
function editRule(rule: Rule): void { editingRule.value = rule; editorVisible.value = true }
async function removeRule(rule: Rule): Promise<void> {
  await ElMessageBox.confirm(`删除后，不可恢复！！真的要删除“${rule.name}”吗?`, '删除玩法', { type: 'warning', confirmButtonText: '删除', cancelButtonText: '取消' })
  await store.removeRule(rule.id); ElMessage.success('已删除')
}
async function clearRules(): Promise<void> {
  if (!store.rules.length) { ElMessage.info('操作列表已经为空'); return }
  await ElMessageBox.confirm('清空后，不可恢复！只删除玩法规则，不会清理弹幕记录。', '清空操作列表', { type: 'warning' })
  await store.clearRules(); ElMessage.success('操作列表已清空')
}
async function cloneRule(rule: Rule): Promise<void> {
  const result = await ElMessageBox.prompt('输入克隆数量', '克隆玩法', {
    inputValue: '1',
    inputPattern: /^[1-9]\d?$/,
    inputErrorMessage: '请输入 1 到 99 之间的数量',
    confirmButtonText: '克隆',
    cancelButtonText: '取消',
  })
  const count = Number(result.value)
  await store.cloneRule(rule.id, count)
  ElMessage.success(count > 1 ? `已克隆 ${count} 条玩法` : '已克隆玩法')
}
async function togglePinned(rule: Rule): Promise<void> {
  await store.setRulePinned(rule.id, !rule.pinned)
  ElMessage.success(rule.pinned ? '已取消置顶' : '已置顶')
}
async function debugRule(rule: Rule): Promise<void> {
  const kind = rule.trigger.kinds[0] ?? 'gift'
  const input: Partial<LiveEvent> & Pick<LiveEvent, 'kind'> = kind === 'gift' ? { kind, gift: { name: rule.trigger.giftNames?.[0] ?? rule.trigger.keywords?.[0] ?? '啤酒', count: 1 }, user: { name: '调试观众' } } : { kind, text: rule.trigger.keywords?.[0] ?? '调试弹幕', user: { name: '调试观众' } }
  await store.simulate(input); ElMessage.success(`已发送 ${rule.name} 调试事件`)
}
async function toggleRulesEnabled(enabled: boolean): Promise<void> {
  runtimeBusy.value = true
  try {
    await store.setRulesEnabled(enabled)
    ElMessage.success(enabled ? '玩法已开启' : '玩法已停止：事件仍会记录，但不会执行动作')
  } finally { runtimeBusy.value = false }
}
async function toggleRulesPaused(): Promise<void> {
  runtimeBusy.value = true
  try {
    await store.setRulesPaused(!store.runtime.paused)
    ElMessage.success(store.runtime.paused ? '已暂停事件处理' : '已继续事件处理')
  } finally { runtimeBusy.value = false }
}
async function simulate(): Promise<void> {
  await store.simulate(simulatorKind.value === 'gift' ? { kind: 'gift', gift: { name: simulatorText.value || '啤酒', count: simulatorCount.value }, user: { name: simulatorUser.value || '模拟观众' } } : simulatorKind.value === 'chat' ? { kind: 'chat', text: simulatorText.value, user: { name: simulatorUser.value || '模拟观众' } } : { kind: simulatorKind.value, count: simulatorCount.value, user: { name: simulatorUser.value || '模拟观众' } })
  ElMessage.success('模拟事件已发送')
  simulatorVisible.value = false
}
async function importConfig(): Promise<void> { const result = await api.config.import(); result.ok ? (await store.refreshRules(), ElMessage.success(result.message)) : ElMessage.warning(result.message) }
async function exportConfig(): Promise<void> { const result = await api.config.export(); result.ok ? ElMessage.success(result.message) : ElMessage.info(result.message) }
</script>

<template>
  <div class="page-stack control-page">
    <div class="page-heading"><div><div class="section-kicker">工作区 / 01</div><h1>控制中心</h1><p>管理直播触发规则与对应动作。</p></div><div class="heading-metric"><span>{{ store.rules.length }}</span><small>条玩法</small></div></div>
    <div class="toolbar-card">
      <div class="toolbar-primary"><el-button type="primary" @click="addRule"><el-icon><Plus /></el-icon>新建玩法</el-button><el-button @click="simulatorVisible = true"><el-icon><MagicStick /></el-icon>模拟事件</el-button><el-button class="clear-rules-action" type="danger" plain @click="clearRules"><el-icon><Delete /></el-icon>清空</el-button></div>
      <div class="toolbar-secondary"><el-button class="config-action" text @click="importConfig"><el-icon><Upload /></el-icon>导入</el-button><el-button class="config-action" text @click="exportConfig"><el-icon><Download /></el-icon>导出</el-button><el-input v-model="search" class="rule-search" clearable type="search" name="rule-search" autocomplete="off" placeholder="按名称搜索…"><template #prefix><el-icon><Search /></el-icon></template></el-input></div>
    </div>

    <div class="data-card rule-card">
      <div class="card-heading"><div><h2>玩法规则</h2><span class="card-subtitle">优先级大的优先执行 · 置顶玩法固定在最前</span></div><el-tag effect="plain" round>{{ filteredRules.length }} 条</el-tag></div>
      <el-table v-if="filteredRules.length" :data="filteredRules" row-key="id" class="rules-table" max-height="300">
        <el-table-column label="序号" width="62" align="center"><template #default="scope"><span class="rule-index">{{ Number(scope.$index) + 1 }}</span></template></el-table-column>
        <el-table-column prop="name" label="名称" min-width="126"><template #default="scope"><div class="rule-name-cell"><span class="rule-state" :class="{ enabled: scope.row.enabled }" /><b>{{ scope.row.name }}</b><el-tag v-if="scope.row.pinned" class="pin-tag" size="small" type="warning" effect="plain">置顶</el-tag></div></template></el-table-column>
        <el-table-column prop="priority" label="优先级" width="72"><template #default="scope"><span class="priority-pill">P{{ scope.row.priority }}</span></template></el-table-column>
        <el-table-column label="触发内容" min-width="120"><template #default="scope"><span class="trigger-copy">{{ triggerHeadline(scope.row as Rule) }}</span></template></el-table-column>
        <el-table-column label="触发方式" min-width="155"><template #default="scope"><span class="trigger-copy">{{ describeRuleTrigger(scope.row as Rule) }}</span></template></el-table-column>
        <el-table-column label="礼物触发" width="88" align="center"><template #default="scope"><el-tag size="small" :type="scope.row.trigger.kinds.includes('gift') ? 'success' : 'info'">{{ scope.row.trigger.kinds.includes('gift') ? '是' : '否' }}</el-tag></template></el-table-column>
        <el-table-column label="播放声音" width="88" align="center"><template #default="scope"><span class="boolean-text">{{ hasSound(scope.row as Rule) ? '开启' : '关闭' }}</span></template></el-table-column>
        <el-table-column label="触发限制" width="82" align="center"><template #default="scope"><span class="boolean-text">{{ limitSummary(scope.row as Rule) || '不限' }}</span></template></el-table-column>
        <el-table-column label="操作" fixed="right" width="150"><template #default="scope"><div class="row-actions"><el-button link type="primary" @click="editRule(scope.row as Rule)"><el-icon><Edit /></el-icon>修改</el-button><el-dropdown trigger="click"><el-button link aria-label="更多玩法操作"><el-icon><MoreFilled /></el-icon></el-button><template #dropdown><el-dropdown-menu><el-dropdown-item @click="togglePinned(scope.row as Rule)"><el-icon><Top /></el-icon>{{ scope.row.pinned ? '取消置顶' : '置顶' }}</el-dropdown-item><el-dropdown-item @click="cloneRule(scope.row as Rule)"><el-icon><CopyDocument /></el-icon>克隆</el-dropdown-item><el-dropdown-item @click="debugRule(scope.row as Rule)"><el-icon><VideoPlay /></el-icon>调试</el-dropdown-item><el-dropdown-item divided @click="removeRule(scope.row as Rule)"><el-icon><Delete /></el-icon>删除</el-dropdown-item></el-dropdown-menu></template></el-dropdown></div></template></el-table-column>
      </el-table>
      <div v-else class="empty-state"><div class="empty-orbit" aria-hidden="true"><span /><i /><b>+</b></div><h3>还没有玩法</h3><p>新建一条规则，直播事件就能触发对应动作。</p><el-button type="primary" plain @click="addRule">创建第一条玩法</el-button></div>
    </div>

    <div class="control-statusbar">
      <div class="status-cluster">
        <template v-if="store.runtime.debugKeys"><span class="hotkey-chip">{{ shortcutModifier }} + Shift + 1</span><span class="status-copy">按下组合键 {{ shortcutModifier }} + Shift + 1-9 调试相应功能</span></template>
        <span v-else class="status-copy">调试快捷键仅在开发模式（devMode）下注册</span>
        <template v-if="expiresDate"><i class="status-divider" /><span class="status-copy">有效期至: {{ expiresDate }}</span></template>
      </div>
      <div class="status-cluster">
        <el-button link :disabled="runtimeBusy" @click="toggleRulesPaused">{{ store.runtime.closeKey }} {{ store.runtime.paused ? '继续' : '暂停' }}</el-button>
        <el-tag size="small" effect="plain" :type="store.runtime.paused ? 'warning' : 'info'">{{ store.runtime.paused ? '已暂停' : '接收中' }}</el-tag>
        <el-tag size="small" effect="plain" :type="store.runtime.enabled ? 'success' : 'info'">{{ store.runtime.enabled ? '开启的' : '已关闭' }}</el-tag>
        <el-switch :model-value="store.runtime.enabled" :loading="runtimeBusy" :disabled="runtimeBusy" aria-label="全局开启或停止玩法" @change="toggleRulesEnabled(Boolean($event))" />
        <span class="status-copy">{{ store.runtime.openKey }} 开启/停止</span>
      </div>
    </div>
  </div>

  <RuleEditorDialog v-model="editorVisible" :rule="editingRule" @saved="store.refreshRules" />
  <el-dialog v-model="simulatorVisible" title="本地事件模拟器" width="min(460px, calc(100vw - 40px))"><div class="simulator-hint">事件会进入规则队列并展示到 Overlay。命中规则时，已配置的键鼠和串口动作也会真实执行。</div><el-form label-position="top"><el-form-item label="事件类型"><el-radio-group v-model="simulatorKind"><el-radio-button value="gift">礼物</el-radio-button><el-radio-button value="chat">弹幕</el-radio-button><el-radio-button value="like">点赞</el-radio-button><el-radio-button value="follow">关注</el-radio-button></el-radio-group></el-form-item><el-form-item label="用户"><el-input v-model="simulatorUser" /></el-form-item><el-form-item :label="simulatorKind === 'gift' ? '礼物名称 / 弹幕文本' : '弹幕文本'"><el-input v-model="simulatorText" placeholder="例如：啤酒 / 666" /></el-form-item><el-form-item v-if="simulatorKind === 'gift' || simulatorKind === 'like'" label="数量 / 连击数"><el-input-number v-model="simulatorCount" :min="1" :max="9999" /></el-form-item></el-form><template #footer><el-button @click="simulatorVisible = false">取消</el-button><el-button type="primary" @click="simulate">发送事件</el-button></template></el-dialog>
</template>
