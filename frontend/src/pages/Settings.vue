<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref, toRaw, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { api } from '../services/api'
import { useAppStore } from '../stores/app'
import { connectorBusy, connectorStateText, connectorStateTone } from '@shared/connection'
import type { AppSettings, OverlaySettings } from '@shared/types'

const store = useAppStore()
const form = reactive<Partial<AppSettings>>({})
const logs = ref(awaitLogs())
const saved = ref(false)
const ready = ref(false)
const loadError = ref('')
const obsState = ref('未连接')
const obsPasswordInput = ref('')
const virtualCameraActive = ref(false)
const autoSavedAt = ref('')
const runtimeBusy = ref(false)
const overlayStatus = computed(() => ({ green: form.overlayGreen?.visible ? '已打开' : '未打开', slot: form.overlaySlot?.visible ? '已打开' : '未打开' }))
// 原版「播放声音的音量」用 0-100 显示，配置里仍保存 0-1。
const volumePercent = computed({
  get: () => Math.round((form.audioVolume ?? 0) * 100),
  set: (value: number) => { form.audioVolume = Math.min(1, Math.max(0, value / 100)) },
})
const roomLabel = computed(() => {
  switch (form.platform) {
    case 'douyin': return '抖音直播间号 / 分享链接'
    case 'kuaishou': return '快手号'
    case 'shipinhao': return '视频号'
    case 'douyu': return '斗鱼直播间ID'
    case 'bilibili': return 'B站直播间ID'
    case 'tiktok': return 'TIKTOK平台ID'
    case 'xiaohongshu': return '小红薯直播间ID'
    default: return '直播间链接 / 平台ID'
  }
})
const connectorTone = computed(() => connectorStateTone(store.status.state))
const connectorText = computed(() => connectorStateText(store.status.state))
const connectorWorking = computed(() => connectorBusy(store.status))

let autosaveTimer: number | undefined
let applyGuard = false

// 自动保存：改动后 500ms 防抖提交；首次回填期间不触发。
watch(form, () => {
  if (!ready.value || applyGuard) return
  scheduleAutosave()
}, { deep: true })

onMounted(() => { void load() })
onBeforeUnmount(() => {
  if (autosaveTimer === undefined) return
  window.clearTimeout(autosaveTimer)
  autosaveTimer = undefined
  void persistSettings().catch(() => undefined)
})

async function load(): Promise<void> {
  loadError.value = ''
  ready.value = false
  try {
    await store.init()
    // store.settings 是 Vue 响应式代理，structuredClone 无法克隆 Proxy，需要先取原始对象。
    applyGuard = true
    Object.assign(form, structuredClone(toRaw(store.settings ?? {})))
    logs.value = await api.diagnostics.logs(40)
    const status = await api.obs.status()
    obsState.value = status.message
    virtualCameraActive.value = Boolean(status.virtualCameraActive)
    ready.value = true
    applyGuard = false
  } catch (error) {
    applyGuard = false
    loadError.value = error instanceof Error ? error.message : '设置读取失败'
  }
}
function awaitLogs() { return [] as Awaited<ReturnType<typeof api.diagnostics.logs>> }

function scheduleAutosave(): void {
  window.clearTimeout(autosaveTimer)
  autosaveTimer = window.setTimeout(() => {
    autosaveTimer = undefined
    void persistSettings().then(() => { autoSavedAt.value = new Date().toLocaleTimeString('zh-CN', { hour12: false }) }).catch((error: unknown) => {
      ElMessage.error(error instanceof Error ? error.message : '自动保存失败')
    })
  }, 500)
}

async function persistSettings(): Promise<void> {
  const patch = { ...form }
  delete patch.obsPassword
  if (obsPasswordInput.value) patch.obsPassword = obsPasswordInput.value
  applyGuard = true
  try {
    await store.saveSettings(patch)
    obsPasswordInput.value = ''
  } finally { applyGuard = false }
}

async function save(): Promise<void> {
  if (autosaveTimer !== undefined) { window.clearTimeout(autosaveTimer); autosaveTimer = undefined }
  await persistSettings()
  saved.value = true
  ElMessage.success('设置已保存')
  setTimeout(() => { saved.value = false }, 1600)
}
async function updateOverlay(type: 'green' | 'slot', patch: Partial<OverlaySettings>): Promise<void> { const current = form[type === 'green' ? 'overlayGreen' : 'overlaySlot'] ?? {}; const next = await api.overlay.updateSettings(type, patch); Object.assign(current, next) }
async function toggleOverlay(type: 'green' | 'slot'): Promise<void> { const current = form[type === 'green' ? 'overlayGreen' : 'overlaySlot']; if (!current) return; current.visible = !current.visible; current.visible ? await api.overlay.open(type) : await api.overlay.close(type); await updateOverlay(type, { visible: current.visible }) }
async function toggleSlotPerf(value: string | number | boolean): Promise<void> { await updateOverlay('slot', { showPerf: Boolean(value) }) }
async function toggleRulesEnabled(enabled: boolean): Promise<void> {
  runtimeBusy.value = true
  try { await store.setRulesEnabled(enabled) } finally { runtimeBusy.value = false }
}
async function toggleRulesPaused(): Promise<void> {
  runtimeBusy.value = true
  try { await store.setRulesPaused(!store.runtime.paused) } finally { runtimeBusy.value = false }
}
async function connectObs(): Promise<boolean> {
  const url = form.obsUrl || 'ws://127.0.0.1:4455'
  await api.diagnostics.saveSettings({ obsUrl: url })
  const result = await api.obs.connect(url, obsPasswordInput.value || undefined)
  obsState.value = result.message
  if (!result.ok) ElMessage.error(result.message)
  else ElMessage.success(result.message)
  return result.ok
}
async function toggleVirtualCamera(): Promise<void> {
  if (!(await api.obs.status()).connected && !(await connectObs())) return
  const result = virtualCameraActive.value ? await api.obs.stopVirtualCamera() : await api.obs.startVirtualCamera()
  obsState.value = result.message
  if (!result.ok) ElMessage.error(result.message)
  else { virtualCameraActive.value = Boolean((await api.obs.status()).virtualCameraActive); ElMessage.success(result.message) }
}
async function clearRecords(): Promise<void> { await ElMessageBox.confirm('清空弹幕记录不会删除玩法配置。', '确认清空', { type: 'warning' }); const count = await api.danmaku.clear(); ElMessage.success(`已清理 ${count} 条记录`) }
// 连接前先落盘，避免自动保存的防抖计时器把旧设置写回去。
async function flushSettings(): Promise<void> {
  if (autosaveTimer !== undefined) { window.clearTimeout(autosaveTimer); autosaveTimer = undefined }
  await persistSettings()
}
async function toggleConnection(): Promise<void> {
  if (connectorWorking.value) return
  if (store.status.state === 'connected') {
    try { await store.disconnect(); ElMessage.info('已断开直播间连接') }
    catch (error) { ElMessage.error(error instanceof Error ? error.message : '断开失败') }
    return
  }
  const roomId = (form.roomId ?? '').trim()
  if (!roomId) { ElMessage.warning('请先填写直播间号或直播间链接'); return }
  try {
    await flushSettings()
    await store.connect(form.platform ?? 'simulator', roomId)
    if ((form.platform ?? 'simulator') === 'simulator') ElMessage.success('已连接本地模拟事件源')
  } catch (error) { ElMessage.error(error instanceof Error ? error.message : '连接失败') }
}async function exportConfig(): Promise<void> { const result = await api.config.export(); result.ok ? ElMessage.success(result.message) : ElMessage.info(result.message) }
async function importConfig(): Promise<void> { const result = await api.config.import(); result.ok ? (await store.refreshRules(), ElMessage.success(result.message)) : ElMessage.warning(result.message) }
</script>

<template>
  <div class="page-stack settings-page">
    <div class="page-heading"><div><div class="section-kicker">SYSTEM / 04</div><h1>通用设置</h1><p>连接、窗口、节流和输入节奏都在这里；改动会立即生效。</p></div><div class="heading-actions"><span v-if="autoSavedAt" class="autosave-hint">自动保存于 {{ autoSavedAt }}</span><el-button type="primary" size="large" :class="{ saved }" :disabled="!ready" @click="save">{{ saved ? '已保存 ✓' : '保存设置' }}</el-button></div></div>
    <el-alert v-if="loadError" class="settings-load-error" type="error" :closable="false" show-icon title="设置读取失败">
      <template #default><span class="settings-load-error-body"><span>{{ loadError }}</span><el-button link type="primary" @click="load">重新加载</el-button></span></template>
    </el-alert>
    <div v-if="!ready && !loadError" class="settings-loading"><el-skeleton :rows="6" animated /></div>
    <div v-if="ready" class="settings-grid">
      <section class="settings-card data-card">
        <div class="settings-title"><span class="settings-index">A</span><div><h2>平台连接</h2><p>抖音走 HTTP 拉取线路，其余平台适配器保持独立。</p></div></div>
        <el-form label-position="top">
          <div class="form-grid">
            <el-form-item label="直播平台"><el-select v-model="form.platform"><el-option label="本地模拟器" value="simulator" /><el-option label="抖音" value="douyin" /><el-option label="快手（未实现）" value="kuaishou" /><el-option label="视频号（未实现）" value="shipinhao" /><el-option label="B站（未实现）" value="bilibili" /><el-option label="TIKTOK（未实现）" value="tiktok" /><el-option label="小红书（未实现）" value="xiaohongshu" /></el-select></el-form-item>
            <el-form-item :label="roomLabel"><el-input v-model="form.roomId" placeholder="直播间号、分享短链或直播间链接" /></el-form-item>
          </div>
          <div class="settings-links">
            <el-button type="primary" :loading="connectorWorking" :disabled="connectorWorking" @click="toggleConnection">{{ store.status.state === 'connected' ? '断开连接' : '连接到直播间' }}</el-button>
            <el-tag :type="connectorTone" effect="plain">{{ connectorText }}</el-tag>
            <el-button v-if="store.status.reconnects > 0" link type="info" disabled>已重连 {{ store.status.reconnects }} 次</el-button>
          </div>
          <el-form-item v-if="store.status.lastError" label="最近错误"><div class="setting-callout"><span>{{ store.status.lastError }}</span></div></el-form-item>
          <el-form-item label="连接说明"><div class="setting-callout"><span class="status-dot" :class="store.status.state" /><span>抖音可填直播间号、直播间链接，或直接粘贴分享短链（v.douyin.com/xxxx，含「复制打开抖音」整段文字也能识别）。使用原版参数串里的拉取线路（im/fetch，a_bogus 为占位值，不需要签名），可收到弹幕、进场、点赞、关注与在线人数。匿名会话下抖音会把昵称打码、并且不下发礼物消息（与网页版一致）。</span></div></el-form-item>
        </el-form>
      </section>
      <section class="settings-card data-card">
        <div class="settings-title"><span class="settings-index">B</span><div><h2>Overlay 窗口</h2><p>绿幕和组件窗口按普通窗口显示，可被其他窗口覆盖。软件每次启动时两个窗口都回到关闭状态，需要时在这里或「扩展功能」页手动打开。</p></div></div>
        <div class="overlay-setting-row"><div><b>绿幕窗口</b><small>{{ overlayStatus.green }} · {{ form.overlayGreen?.width }} × {{ form.overlayGreen?.height }}</small></div><div class="row-controls"><el-switch :model-value="Boolean(form.overlayGreen?.visible)" aria-label="启用绿幕窗口" @update:model-value="toggleOverlay('green')" /></div></div>
        <div class="overlay-setting-row"><div><b>组件窗口</b><small>{{ overlayStatus.slot }} · {{ form.overlaySlot?.width }} × {{ form.overlaySlot?.height }}</small></div><div class="row-controls"><el-switch :model-value="Boolean(form.overlaySlot?.visible)" aria-label="启用组件窗口" @update:model-value="toggleOverlay('slot')" /></div></div>
        <el-form label-position="top"><el-form-item v-if="form.overlayGreen" label="绿幕透明度"><el-slider v-model="form.overlayGreen.opacity" :min="0.1" :max="1" :step="0.05" /></el-form-item>
          <el-form-item label="组件窗性能显示"><el-switch :model-value="Boolean(form.overlaySlot?.showPerf)" aria-label="组件窗显示帧率" @update:model-value="toggleSlotPerf" /><small class="field-hint">打开后组件窗左上角显示帧率、最长帧与长帧次数，用来定位拖动 / 拉伸卡顿；平时可关闭。</small></el-form-item>
        </el-form>
      </section>
      <section class="settings-card data-card">
        <div class="settings-title"><span class="settings-index">C</span><div><h2>弹幕记录</h2><p>默认 30 天 / 20 万条，原始数据默认关闭。</p></div></div>
        <el-form label-position="top"><div class="form-grid"><el-form-item label="保留天数"><el-input-number v-model="form.retentionDays" :min="1" :max="3650" /></el-form-item><el-form-item label="最大条数"><el-input-number v-model="form.retentionMaxRows" :min="1000" :max="10000000" /></el-form-item></div><el-switch v-model="form.storeRaw" active-text="保存脱敏原始数据" inactive-text="不保存原始数据" /><div class="danger-zone"><span><b>清空全部记录</b><small>只清库，不删除规则与素材</small></span><el-button type="danger" plain size="small" @click="clearRecords">清空记录</el-button></div></el-form>
      </section>
      <section class="settings-card data-card">
        <div class="settings-title"><span class="settings-index">D</span><div><h2>OBS / 虚拟摄像头</h2><p>{{ obsState }}</p></div></div>
        <el-form label-position="top"><el-form-item label="OBS WebSocket 地址"><el-input v-model="form.obsUrl" placeholder="ws://127.0.0.1:4455" /></el-form-item><el-form-item label="OBS WebSocket 密码"><el-input v-model="obsPasswordInput" type="password" show-password :placeholder="form.obsPassword === '***' ? '密码已保存；留空则继续使用已保存密码' : '如 OBS 已开启鉴权，请输入密码'" /></el-form-item>
          <div class="settings-links"><el-button @click="connectObs">连接 OBS</el-button><el-button type="primary" @click="toggleVirtualCamera">{{ virtualCameraActive ? '停止虚拟摄像头' : '启动虚拟摄像头' }}</el-button></div>
          <div class="setting-callout obs-callout"><span>摄像头输出 OBS 当前场景。首次使用请在 OBS 场景中添加 yapp 组件窗口采集源；驱动和设备由 OBS 安装管理。</span></div>
          <div class="diagnostic-list"><div><span>连接器状态</span><b>{{ store.status.state }}</b></div><div><span>OBS 虚拟摄像头</span><b>{{ virtualCameraActive ? '运行中' : '已停止' }}</b></div><div><span>Overlay 绿幕 / 组件</span><b>{{ overlayStatus.green }} / {{ overlayStatus.slot }}</b></div><div><span>最近日志</span><b>{{ logs.length }} 条</b></div></div>
        </el-form><div class="settings-links"><el-button link type="primary" @click="exportConfig">导出配置</el-button><el-button link type="primary" @click="importConfig">导入配置</el-button></div>
      </section>
      <section class="settings-card data-card">
        <div class="settings-title"><span class="settings-index">E</span><div><h2>运行控制</h2><p>两个固定热键不可修改，状态与控制中心底部一致。</p></div></div>
        <div class="overlay-setting-row"><div><b>开启 / 停止</b><small>{{ store.runtime.openKey }} · 停止后事件仍会记录，但不会执行任何玩法与功能动作</small></div><div class="row-controls"><el-tag size="small" effect="plain" :type="store.runtime.enabled ? 'success' : 'info'">{{ store.runtime.enabled ? '开启的' : '已关闭' }}</el-tag><el-switch :model-value="store.runtime.enabled" :loading="runtimeBusy" :disabled="runtimeBusy" aria-label="全局开启或停止玩法" @change="toggleRulesEnabled(Boolean($event))" /></div></div>
        <div class="overlay-setting-row"><div><b>暂停 / 继续</b><small>{{ store.runtime.closeKey }} · 暂停后新事件不会进入队列（计入丢弃数），恢复后继续</small></div><div class="row-controls"><el-tag size="small" effect="plain" :type="store.runtime.paused ? 'warning' : 'info'">{{ store.runtime.paused ? '已暂停' : '接收中' }}</el-tag><el-switch :model-value="store.runtime.paused" :loading="runtimeBusy" :disabled="runtimeBusy" aria-label="暂停或继续事件处理" @change="toggleRulesPaused" /></div></div>
        <el-form label-position="top"><el-form-item label="调试延时（毫秒）"><el-input-number v-model="form.debugSleepMs" :min="0" :max="60000" /><small class="field-help">调试快捷键与玩法热键触发前的等待时间，0 表示不等待</small></el-form-item></el-form>
      </section>
      <section class="settings-card data-card">
        <div class="settings-title"><span class="settings-index">F</span><div><h2>节流与输入</h2><p>连击合并、动作排队与本地输入节奏。</p></div></div>
        <div class="overlay-setting-row"><div><b>禁用组刷</b><small>当收到多个礼物时只执行 1 次操作：同一用户 1.2 秒内连送同款礼物合并为一次；关闭时每条礼物消息都会执行一次动作</small></div><div class="row-controls"><el-switch v-model="form.isDisableGiftGroup" aria-label="禁用组刷" /></div></div>
        <div class="overlay-setting-row"><div><b>操作顺序执行</b><small>同一时间只执行一条玩法的动作列表，其余玩法排队等待</small></div><div class="row-controls"><el-switch v-model="form.isOrder" aria-label="操作顺序执行" /></div></div>
        <div class="overlay-setting-row"><div><b>禁用OBS连接</b><small>开启后所有 OBS 源、滤镜与虚拟摄像头调用都会被拒绝</small></div><div class="row-controls"><el-switch v-model="form.isDisableOBS" aria-label="禁用OBS连接" /></div></div>
        <el-form label-position="top"><div class="form-grid"><el-form-item label="播放声音的音量"><el-slider v-model="volumePercent" :min="0" :max="100" show-input /></el-form-item></div><div class="form-grid"><el-form-item label="鼠标移动步长 · 上下"><el-input-number v-model="form.moveTopBottomStep" :min="0" :max="2000" /></el-form-item><el-form-item label="鼠标移动步长 · 左右"><el-input-number v-model="form.moveLeftRightStep" :min="0" :max="2000" /></el-form-item></div><div class="setting-callout"><span>步长会把鼠标移动拆成小段，避免游戏忽略瞬移；填 0 表示一次到位。仅 Windows 版本生效。</span></div></el-form>
      </section>
    </div>
    <div class="about-line"><span>AKA直播 · {{ form.version || '0.2.0' }}</span><span>改动后 500 ms 自动保存 · Wails + Vue 3 · 本地模拟事件源</span></div>
  </div>
</template>
