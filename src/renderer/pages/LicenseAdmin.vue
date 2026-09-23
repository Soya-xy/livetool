<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import type { AppSettings, LicenseAuditEntry, LicenseCardCreateInput, LicenseCardDetail, LicenseCardQuery, LicenseCardStatus, LicenseCardSummary, LicenseEntitlement, Platform } from '@shared/types'
import { api } from '../services/api'
import { useAppStore } from '../stores/app'

const store = useAppStore()
const serverUrl = ref('http://127.0.0.1:8787')
const username = ref('')
const password = ref('')
const adminReady = ref(false)
const adminExpiresAt = ref('')
const busy = ref(false)
const query = ref('')
const statusFilter = ref<LicenseCardQuery['status']>('')
const page = ref(1)
const pageSize = ref(20)
const total = ref(0)
const cards = ref<LicenseCardSummary[]>([])
const audit = ref<LicenseAuditEntry[]>([])
const createVisible = ref(false)
const issuedVisible = ref(false)
const issuedCodes = ref<string[]>([])
const detailVisible = ref(false)
const selectedCard = ref<LicenseCardDetail>()
const createForm = ref<LicenseCardCreateInput>(freshCreateForm())

const platformOptions: Array<{ value: Platform; label: string }> = [
  { value: 'simulator', label: '本地模拟器' }, { value: 'douyin', label: '抖音' }, { value: 'kuaishou', label: '快手' },
  { value: 'shipinhao', label: '视频号' }, { value: 'bilibili', label: 'B站' }, { value: 'tiktok', label: 'TikTok' },
  { value: 'douyu', label: '斗鱼' }, { value: 'xiaohongshu', label: '小红书' },
]
const entitlementOptions: Array<{ value: LicenseEntitlement; label: string }> = [
  { value: 'overlay', label: '绿幕 / OBS' }, { value: 'slot', label: '组件窗口' }, { value: 'serial', label: '串口' },
]

onMounted(async () => {
  await store.init()
  const settings = await api.diagnostics.settings() as AppSettings
  serverUrl.value = settings.authServerUrl || serverUrl.value
  try {
    const status = await api.auth.adminStatus()
    adminReady.value = status.loggedIn
    adminExpiresAt.value = status.expiresAt ?? ''
    if (adminReady.value) await refreshData()
  } catch { adminReady.value = false }
})

function freshCreateForm(): LicenseCardCreateInput {
  return { count: 10, durationDays: 30, maxDevices: 1, platforms: [], features: ['overlay', 'slot'], note: '' }
}

async function login(): Promise<void> {
  busy.value = true
  try {
    const result = await api.auth.adminLogin({ serverUrl: serverUrl.value.trim(), username: username.value.trim(), password: password.value })
    if (!result.ok) throw new Error(result.message)
    adminReady.value = true
    adminExpiresAt.value = result.expiresAt ?? ''
    password.value = ''
    await refreshData()
    ElMessage.success(result.message)
  } catch (error) { ElMessage.error(error instanceof Error ? error.message : '管理员登录失败') }
  finally { busy.value = false }
}

async function logout(): Promise<void> {
  await api.auth.adminLogout()
  adminReady.value = false
  cards.value = []
  audit.value = []
  total.value = 0
}

async function refreshData(): Promise<void> {
  busy.value = true
  try {
    const [result, auditRows] = await Promise.all([
      api.auth.listCards({ query: query.value.trim(), status: statusFilter.value, page: page.value, pageSize: pageSize.value }),
      api.auth.auditLog(40),
    ])
    cards.value = result.items
    total.value = result.total
    audit.value = auditRows
  } catch (error) {
    adminReady.value = false
    ElMessage.error(error instanceof Error ? error.message : '读取卡密数据失败')
  } finally { busy.value = false }
}

function searchCards(): void { page.value = 1; void refreshData() }

async function createCards(): Promise<void> {
  busy.value = true
  try {
    const created = await api.auth.createCards(createForm.value)
    issuedCodes.value = created.map((item) => item.code)
    createVisible.value = false
    issuedVisible.value = true
    createForm.value = freshCreateForm()
    await refreshData()
  } catch (error) { ElMessage.error(error instanceof Error ? error.message : '生成卡密失败') }
  finally { busy.value = false }
}

async function copyCodes(): Promise<void> {
  try {
    await navigator.clipboard.writeText(issuedCodes.value.join('\n'))
    ElMessage.success(`已复制 ${issuedCodes.value.length} 张卡密`)
  } catch { ElMessage.error('复制失败，请手动选中并复制') }
}

async function openDetails(card: LicenseCardDetail): Promise<void> {
  try {
    selectedCard.value = await api.auth.cardDetail(card.id)
    detailVisible.value = true
  } catch (error) { ElMessage.error(error instanceof Error ? error.message : '读取卡密详情失败') }
}

async function updateStatus(card: LicenseCardSummary, status: Extract<LicenseCardStatus, 'active' | 'disabled' | 'revoked'>): Promise<void> {
  const action = status === 'revoked' ? '永久撤销' : status === 'disabled' ? '停用' : '重新启用'
  try {
    await ElMessageBox.confirm(`确定${action}尾号 ${card.suffix} 的卡密吗？`, `${action}卡密`, { type: status === 'revoked' ? 'error' : 'warning' })
    await api.auth.setCardStatus(card.id, status)
    ElMessage.success(`卡密已${action}`)
    await refreshData()
  } catch (error) {
    if (error !== 'cancel' && error !== 'close') ElMessage.error(error instanceof Error ? error.message : '更新卡密状态失败')
  }
}

async function resetBindings(card: LicenseCardSummary): Promise<void> {
  try {
    await ElMessageBox.confirm(`将移除尾号 ${card.suffix} 卡密的全部设备绑定，设备需要重新激活。`, '重置设备绑定', { type: 'warning' })
    const removed = await api.auth.resetCardBindings(card.id)
    ElMessage.success(`已重置 ${removed} 个设备绑定`)
    await refreshData()
  } catch (error) {
    if (error !== 'cancel' && error !== 'close') ElMessage.error(error instanceof Error ? error.message : '重置绑定失败')
  }
}

async function extendCard(card: LicenseCardSummary): Promise<void> {
  try {
    const { value } = await ElMessageBox.prompt('输入要增加的天数', '卡密续期', { inputValue: '30', inputPattern: /^[1-9]\d{0,4}$/, inputErrorMessage: '请输入 1 到 99999 的正整数' })
    await api.auth.extendCard(card.id, Number(value))
    ElMessage.success('卡密有效期已延长')
    await refreshData()
  } catch (error) {
    if (error !== 'cancel' && error !== 'close') ElMessage.error(error instanceof Error ? error.message : '卡密续期失败')
  }
}

function statusLabel(status: LicenseCardStatus): string {
  return ({ pending: '未激活', active: '有效', disabled: '已停用', revoked: '已撤销', expired: '已过期' })[status]
}
function statusType(status: LicenseCardStatus): 'success' | 'warning' | 'danger' | 'info' {
  if (status === 'active') return 'success'
  if (status === 'pending') return 'info'
  if (status === 'disabled') return 'warning'
  return 'danger'
}
function dateText(value?: number): string { return value ? new Date(value).toLocaleString() : '—' }
function entitlementText(features: LicenseEntitlement[]): string { return features.map((feature) => entitlementOptions.find((option) => option.value === feature)?.label ?? feature).join('、') }
function platformText(platforms: Platform[]): string { return platforms.length ? platforms.map((platform) => platformOptions.find((option) => option.value === platform)?.label ?? platform).join('、') : '全部平台' }
</script>

<template>
  <div class="page-stack license-admin-page">
    <div class="page-heading">
      <div><div class="section-kicker">AUTHORIZATION / ADMIN</div><h1>卡密管理</h1><p>管理自建授权服务的卡密、有效期、设备绑定和功能权限。</p></div>
      <div v-if="adminReady" class="heading-metric"><span>{{ total }}</span><small>当前结果</small></div>
    </div>

    <section class="data-card license-login-card">
      <div class="license-login-head"><div><h2>授权服务</h2><p>管理员凭据只用于本次登录，服务端令牌仅保存在主进程内存中。</p></div><el-tag v-if="adminReady" type="success" effect="plain">管理员已登录 · {{ adminExpiresAt ? dateText(Date.parse(adminExpiresAt)) : '会话有效' }}</el-tag></div>
      <el-form class="license-login-form" label-position="top" @submit.prevent="login">
        <el-form-item label="服务地址"><el-input v-model="serverUrl" :disabled="adminReady" placeholder="https://license.example.com" /></el-form-item>
        <el-form-item label="管理员账号"><el-input v-model="username" :disabled="adminReady" autocomplete="username" /></el-form-item>
        <el-form-item label="管理员密码"><el-input v-model="password" type="password" show-password :disabled="adminReady" autocomplete="current-password" @keyup.enter="login" /></el-form-item>
        <div class="license-login-actions"><el-button v-if="!adminReady" type="primary" :loading="busy" @click="login">登录管理后台</el-button><template v-else><el-button :loading="busy" @click="refreshData">刷新</el-button><el-button @click="logout">退出管理员</el-button></template></div>
      </el-form>
      <div class="license-security-note">卡密仅以带服务端密钥的 HMAC 摘要保存；明文只在创建后展示一次。远程部署必须经 HTTPS 反向代理。</div>
    </section>

    <template v-if="adminReady">
      <section class="data-card license-toolbar">
        <div class="license-toolbar-search"><el-input v-model="query" clearable placeholder="搜索卡密尾号、备注或记录 ID" @keyup.enter="searchCards" /><el-select v-model="statusFilter" clearable placeholder="全部状态" @change="searchCards"><el-option label="未激活" value="pending" /><el-option label="有效" value="active" /><el-option label="已停用" value="disabled" /><el-option label="已撤销" value="revoked" /><el-option label="已过期" value="expired" /></el-select><el-button @click="searchCards">查询</el-button></div>
        <el-button type="primary" @click="createVisible = true">批量生成卡密</el-button>
      </section>

      <section class="data-card license-list-card">
        <div class="card-heading"><div><h2>卡密列表</h2><span class="card-subtitle">不会回显已生成的卡密明文；如遗失，请撤销并重新生成。</span></div><el-tag effect="plain" round>{{ total }} 张</el-tag></div>
        <el-table :data="cards" row-key="id" class="license-table" v-loading="busy" empty-text="没有匹配的卡密">
          <el-table-column label="卡密" min-width="130"><template #default="scope"><code>••••-{{ scope.row.suffix }}</code></template></el-table-column>
          <el-table-column label="状态" width="90"><template #default="scope"><el-tag :type="statusType(scope.row.status)" size="small">{{ statusLabel(scope.row.status) }}</el-tag></template></el-table-column>
          <el-table-column label="有效期" min-width="155"><template #default="scope">{{ scope.row.expiresAt ? dateText(scope.row.expiresAt) : `${scope.row.durationDays} 天 · 首次激活后计时` }}</template></el-table-column>
          <el-table-column label="设备" width="100"><template #default="scope">{{ scope.row.boundDevices }} / {{ scope.row.maxDevices }}</template></el-table-column>
          <el-table-column label="权限" min-width="130"><template #default="scope">{{ entitlementText(scope.row.features) }}</template></el-table-column>
          <el-table-column label="平台" min-width="120"><template #default="scope">{{ platformText(scope.row.platforms) }}</template></el-table-column>
          <el-table-column prop="note" label="备注" min-width="100" show-overflow-tooltip />
          <el-table-column label="操作" fixed="right" width="255"><template #default="scope"><div class="license-row-actions">
            <el-button link type="primary" @click="openDetails(scope.row)">详情</el-button>
            <el-button link :disabled="scope.row.status === 'disabled' || scope.row.status === 'revoked'" @click="extendCard(scope.row)">续期</el-button>
            <el-button link :disabled="scope.row.boundDevices === 0" @click="resetBindings(scope.row)">重置绑定</el-button>
            <el-dropdown v-if="scope.row.status !== 'revoked'" trigger="click"><el-button link>更多</el-button><template #dropdown><el-dropdown-menu><el-dropdown-item v-if="scope.row.status !== 'disabled'" @click="updateStatus(scope.row, 'disabled')">停用</el-dropdown-item><el-dropdown-item v-else @click="updateStatus(scope.row, 'active')">重新启用</el-dropdown-item><el-dropdown-item divided @click="updateStatus(scope.row, 'revoked')">永久撤销</el-dropdown-item></el-dropdown-menu></template></el-dropdown>
          </div></template></el-table-column>
        </el-table>
        <div class="pagination-line"><el-pagination v-model:current-page="page" v-model:page-size="pageSize" :total="total" :page-sizes="[10, 20, 50, 100]" layout="total, sizes, prev, pager, next" @change="refreshData" /></div>
      </section>

      <section class="data-card license-audit-card">
        <div class="card-heading"><div><h2>最近操作</h2><span class="card-subtitle">服务端记录创建、状态变更、续期和绑定重置。</span></div></div>
        <el-table :data="audit" row-key="id" size="small" max-height="260" empty-text="暂无操作记录"><el-table-column label="时间" width="180"><template #default="scope">{{ dateText(scope.row.at) }}</template></el-table-column><el-table-column prop="action" label="操作" min-width="150" /><el-table-column prop="cardId" label="卡密记录 ID" min-width="220" /><el-table-column prop="detail" label="备注" min-width="180" show-overflow-tooltip /></el-table>
      </section>
    </template>

    <el-dialog v-model="createVisible" title="批量生成卡密" width="620px">
      <el-form label-position="top"><div class="license-create-grid">
        <el-form-item label="生成数量"><el-input-number v-model="createForm.count" :min="1" :max="1000" /></el-form-item>
        <el-form-item label="有效天数（首次激活后计时）"><el-input-number v-model="createForm.durationDays" :min="1" :max="36500" /></el-form-item>
        <el-form-item label="绑定设备数"><el-input-number v-model="createForm.maxDevices" :min="1" :max="100000" /></el-form-item>
        <el-form-item label="平台范围"><el-select v-model="createForm.platforms" multiple collapse-tags placeholder="留空表示全部平台"><el-option v-for="option in platformOptions" :key="option.value" :label="option.label" :value="option.value" /></el-select></el-form-item>
      </div><el-form-item label="功能权限"><el-checkbox-group v-model="createForm.features"><el-checkbox v-for="option in entitlementOptions" :key="option.value" :value="option.value">{{ option.label }}</el-checkbox></el-checkbox-group></el-form-item><el-form-item label="备注"><el-input v-model="createForm.note" maxlength="200" show-word-limit placeholder="可选" /></el-form-item></el-form>
      <template #footer><el-button @click="createVisible = false">取消</el-button><el-button type="primary" :loading="busy" @click="createCards">生成并显示卡密</el-button></template>
    </el-dialog>

    <el-dialog v-model="issuedVisible" title="卡密已生成 · 请立即保存" width="620px" :close-on-click-modal="false" @closed="issuedCodes = []">
      <el-alert type="warning" :closable="false" title="明文不会再次提供；关闭此窗口后只能查看尾号。如遗失，请撤销后重发。" />
      <el-input class="issued-codes" type="textarea" :rows="10" :model-value="issuedCodes.join('\n')" readonly />
      <template #footer><el-button @click="issuedVisible = false">关闭</el-button><el-button type="primary" @click="copyCodes">复制全部</el-button></template>
    </el-dialog>

    <el-dialog v-model="detailVisible" title="卡密详情" width="640px">
      <template v-if="selectedCard"><el-descriptions :column="2" border><el-descriptions-item label="尾号">{{ selectedCard.suffix }}</el-descriptions-item><el-descriptions-item label="状态">{{ statusLabel(selectedCard.status) }}</el-descriptions-item><el-descriptions-item label="创建时间">{{ dateText(selectedCard.createdAt) }}</el-descriptions-item><el-descriptions-item label="首次激活">{{ dateText(selectedCard.activatedAt) }}</el-descriptions-item><el-descriptions-item label="到期时间">{{ dateText(selectedCard.expiresAt) }}</el-descriptions-item><el-descriptions-item label="设备数">{{ selectedCard.boundDevices }} / {{ selectedCard.maxDevices }}</el-descriptions-item><el-descriptions-item label="功能权限" :span="2">{{ entitlementText(selectedCard.features) }}</el-descriptions-item><el-descriptions-item label="平台范围" :span="2">{{ platformText(selectedCard.platforms) }}</el-descriptions-item><el-descriptions-item label="备注" :span="2">{{ selectedCard.note || '—' }}</el-descriptions-item></el-descriptions><h3 class="binding-heading">设备绑定记录</h3><el-table :data="selectedCard.bindings" size="small" empty-text="当前没有设备绑定"><el-table-column prop="fingerprint" label="安装标识摘要" /><el-table-column label="绑定时间"><template #default="scope">{{ dateText(scope.row.boundAt) }}</template></el-table-column></el-table></template>
    </el-dialog>
  </div>
</template>
