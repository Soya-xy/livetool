<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { CopyDocument, Delete, Download, Edit, MagicStick, MoreFilled, Plus, Search, Upload, VideoPlay } from '@element-plus/icons-vue'
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
const shortcutModifier = /Mac|iPhone|iPad/.test(navigator.platform) ? '⌘' : 'Ctrl'

const filteredRules = computed(() => store.rules.filter((rule) => !search.value || rule.name.toLocaleLowerCase().includes(search.value.toLocaleLowerCase())))

function describeRuleTrigger(rule: Rule): string {
  const labels: Record<string, string> = { gift: '礼物', chat: '弹幕', like: '点赞', follow: '关注', enter: '进场', system: '系统' }
  const kinds = rule.trigger.kinds.map((kind) => labels[kind]).join(' / ')
  const details = [...(rule.trigger.giftNames ?? []), ...(rule.trigger.keywords ?? [])]
  return details.length ? `${kinds} · ${details.join('、')}` : kinds || '未设置触发'
}

onMounted(() => { void store.init() })
function addRule(): void { editingRule.value = undefined; editorVisible.value = true }
function editRule(rule: Rule): void { editingRule.value = rule; editorVisible.value = true }
async function removeRule(rule: Rule): Promise<void> {
  await ElMessageBox.confirm(`确定删除“${rule.name}”吗？`, '删除玩法', { type: 'warning', confirmButtonText: '删除', cancelButtonText: '取消' })
  await store.removeRule(rule.id); ElMessage.success('已删除')
}
async function clearRules(): Promise<void> {
  if (!store.rules.length) { ElMessage.info('操作列表已经为空'); return }
  await ElMessageBox.confirm('清空后只删除玩法规则，不会清理弹幕记录。', '清空操作列表', { type: 'warning' })
  await store.clearRules(); ElMessage.success('操作列表已清空')
}
async function cloneRule(rule: Rule): Promise<void> { await store.cloneRule(rule.id); ElMessage.success('已克隆玩法') }
async function debugRule(rule: Rule): Promise<void> {
  const kind = rule.trigger.kinds[0] ?? 'gift'
  const input: Partial<LiveEvent> & Pick<LiveEvent, 'kind'> = kind === 'gift' ? { kind, gift: { name: rule.trigger.giftNames?.[0] ?? rule.trigger.keywords?.[0] ?? '啤酒', count: 1 }, user: { name: '调试观众' } } : { kind, text: rule.trigger.keywords?.[0] ?? '调试弹幕', user: { name: '调试观众' } }
  await store.simulate(input); ElMessage.success(`已发送 ${rule.name} 调试事件`)
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
      <div class="toolbar-secondary"><el-button class="config-action" text @click="importConfig"><el-icon><Upload /></el-icon>导入</el-button><el-button class="config-action" text @click="exportConfig"><el-icon><Download /></el-icon>导出</el-button><el-input v-model="search" class="rule-search" clearable type="search" name="rule-search" autocomplete="off" placeholder="搜索玩法名称…"><template #prefix><el-icon><Search /></el-icon></template></el-input></div>
    </div>

    <div class="data-card rule-card">
      <div class="card-heading"><div><h2>玩法规则</h2><span class="card-subtitle">按优先级匹配 · Ctrl + Shift + 1–9 可快速调试</span></div><el-tag effect="plain" round>{{ filteredRules.length }} 条</el-tag></div>
      <el-table v-if="filteredRules.length" :data="filteredRules" row-key="id" class="rules-table" max-height="300">
        <el-table-column prop="name" label="名称" min-width="126"><template #default="scope"><div class="rule-name-cell"><span class="rule-state" :class="{ enabled: scope.row.enabled }" /><b>{{ scope.row.name }}</b></div></template></el-table-column><el-table-column prop="priority" label="优先级" width="72"><template #default="scope"><span class="priority-pill">P{{ scope.row.priority }}</span></template></el-table-column><el-table-column label="触发条件" min-width="155"><template #default="scope"><span class="trigger-copy">{{ describeRuleTrigger(scope.row as Rule) }}</span></template></el-table-column><el-table-column label="礼物" width="68"><template #default="scope"><el-tag size="small" :type="scope.row.trigger.kinds.includes('gift') ? 'success' : 'info'">{{ scope.row.trigger.kinds.includes('gift') ? '是' : '否' }}</el-tag></template></el-table-column><el-table-column label="声音" width="74"><template #default="scope"><span class="boolean-text">{{ scope.row.actions.some((action: any) => action.kind === 'audio') ? '开启' : '关闭' }}</span></template></el-table-column><el-table-column label="操作" fixed="right" width="138"><template #default="scope"><div class="row-actions"><el-button link type="primary" @click="editRule(scope.row as Rule)"><el-icon><Edit /></el-icon>编辑</el-button><el-dropdown trigger="click"><el-button link aria-label="更多玩法操作"><el-icon><MoreFilled /></el-icon></el-button><template #dropdown><el-dropdown-menu><el-dropdown-item @click="cloneRule(scope.row as Rule)"><el-icon><CopyDocument /></el-icon>克隆</el-dropdown-item><el-dropdown-item @click="debugRule(scope.row as Rule)"><el-icon><VideoPlay /></el-icon>调试</el-dropdown-item><el-dropdown-item divided @click="removeRule(scope.row as Rule)"><el-icon><Delete /></el-icon>删除</el-dropdown-item></el-dropdown-menu></template></el-dropdown></div></template></el-table-column>
      </el-table>
      <div v-else class="empty-state"><div class="empty-orbit" aria-hidden="true"><span /><i /><b>+</b></div><h3>还没有玩法</h3><p>新建一条规则，直播事件就能触发对应动作。</p><el-button type="primary" plain @click="addRule">创建第一条玩法</el-button></div>
    </div>
    <div v-if="store.settings?.devMode" class="debug-note"><span class="note-icon" aria-hidden="true">⌘</span><span>快捷调试 <b>{{ shortcutModifier }} + Shift + 1–9</b></span><span class="note-state">开发模式</span></div>
  </div>

  <RuleEditorDialog v-model="editorVisible" :rule="editingRule" @saved="store.refreshRules" />
  <el-dialog v-model="simulatorVisible" title="本地事件模拟器" width="460px"><div class="simulator-hint">事件会进入规则队列并展示到 Overlay。命中规则时，已配置的键鼠和串口动作也会真实执行。</div><el-form label-position="top"><el-form-item label="事件类型"><el-radio-group v-model="simulatorKind"><el-radio-button value="gift">礼物</el-radio-button><el-radio-button value="chat">弹幕</el-radio-button><el-radio-button value="like">点赞</el-radio-button><el-radio-button value="follow">关注</el-radio-button></el-radio-group></el-form-item><el-form-item label="用户"><el-input v-model="simulatorUser" /></el-form-item><el-form-item :label="simulatorKind === 'gift' ? '礼物名称 / 弹幕文本' : '弹幕文本'"><el-input v-model="simulatorText" placeholder="例如：啤酒 / 666" /></el-form-item><el-form-item v-if="simulatorKind === 'gift' || simulatorKind === 'like'" label="数量 / 连击数"><el-input-number v-model="simulatorCount" :min="1" :max="9999" /></el-form-item></el-form><template #footer><el-button @click="simulatorVisible = false">取消</el-button><el-button type="primary" @click="simulate">发送事件</el-button></template></el-dialog>
</template>
