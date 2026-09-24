<script setup lang="ts">
import { computed, onMounted, reactive, ref, toRaw } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { api } from '../services/api'
import { useAppStore } from '../stores/app'
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
const overlayStatus = computed(() => ({ green: form.overlayGreen?.visible ? '已打开' : '未打开', slot: form.overlaySlot?.visible ? '已打开' : '未打开' }))

onMounted(() => { void load() })
async function load(): Promise<void> {
  loadError.value = ''
  ready.value = false
  try {
    await store.init()
    // store.settings 是 Vue 响应式代理，structuredClone 无法克隆 Proxy，需要先取原始对象。
    Object.assign(form, structuredClone(toRaw(store.settings ?? {})))
    logs.value = await api.diagnostics.logs(40)
    const status = await api.obs.status()
    obsState.value = status.message
    virtualCameraActive.value = Boolean(status.virtualCameraActive)
    ready.value = true
  } catch (error) {
    loadError.value = error instanceof Error ? error.message : '设置读取失败'
  }
}
function awaitLogs() { return [] as Awaited<ReturnType<typeof api.diagnostics.logs>> }
async function save(): Promise<void> {
  const patch = { ...form }
  delete patch.obsPassword
  if (obsPasswordInput.value) patch.obsPassword = obsPasswordInput.value
  await store.saveSettings(patch)
  obsPasswordInput.value = ''
  saved.value = true
  ElMessage.success('设置已保存')
  setTimeout(() => { saved.value = false }, 1600)
}
async function updateOverlay(type: 'green' | 'slot', patch: Partial<OverlaySettings>): Promise<void> { const current = form[type === 'green' ? 'overlayGreen' : 'overlaySlot'] ?? {}; const next = await api.overlay.updateSettings(type, patch); Object.assign(current, next) }
async function toggleOverlay(type: 'green' | 'slot'): Promise<void> { const current = form[type === 'green' ? 'overlayGreen' : 'overlaySlot']; if (!current) return; current.visible = !current.visible; current.visible ? await api.overlay.open(type) : await api.overlay.close(type); await updateOverlay(type, { visible: current.visible }) }
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
async function exportConfig(): Promise<void> { const result = await api.config.export(); result.ok ? ElMessage.success(result.message) : ElMessage.info(result.message) }
async function importConfig(): Promise<void> { const result = await api.config.import(); result.ok ? (await store.refreshRules(), ElMessage.success(result.message)) : ElMessage.warning(result.message) }
</script>

<template>
  <div class="page-stack settings-page">
    <div class="page-heading"><div><div class="section-kicker">SYSTEM / 04</div><h1>通用设置</h1><p>连接、窗口、数据保留和诊断信息都在这里。</p></div><el-button type="primary" size="large" :class="{ saved }" :disabled="!ready" @click="save">{{ saved ? '已保存 ✓' : '保存设置' }}</el-button></div>
    <el-alert v-if="loadError" class="settings-load-error" type="error" :closable="false" show-icon title="设置读取失败">
      <template #default><span class="settings-load-error-body"><span>{{ loadError }}</span><el-button link type="primary" @click="load">重新加载</el-button></span></template>
    </el-alert>
    <div v-if="!ready && !loadError" class="settings-loading"><el-skeleton :rows="6" animated /></div>
    <div v-if="ready" class="settings-grid">
      <section class="settings-card data-card">
        <div class="settings-title"><span class="settings-index">A</span><div><h2>平台连接</h2><p>当前版本默认使用本地模拟器；平台适配器保持独立。</p></div></div>
        <el-form label-position="top"><div class="form-grid"><el-form-item label="直播平台"><el-select v-model="form.platform"><el-option label="本地模拟器" value="simulator" /><el-option label="抖音（连接器占位）" value="douyin" /><el-option label="快手（连接器占位）" value="kuaishou" /><el-option label="视频号（连接器占位）" value="shipinhao" /><el-option label="B站（连接器占位）" value="bilibili" /><el-option label="TIKTOK（连接器占位）" value="tiktok" /><el-option label="小红书（连接器占位）" value="xiaohongshu" /></el-select></el-form-item><el-form-item label="直播间 ID"><el-input v-model="form.roomId" placeholder="demo-room" /></el-form-item></div>
          <el-form-item label="连接说明"><div class="setting-callout"><span class="status-dot connected" /><span>模拟器连接支持弹幕、礼物、点赞、关注四类事件；真实平台读写仍需相应平台的合法连接器与账号授权。</span></div></el-form-item>
        </el-form>
      </section>
      <section class="settings-card data-card">
        <div class="settings-title"><span class="settings-index">B</span><div><h2>Overlay 窗口</h2><p>绿幕和组件窗口是独立窗口，可供 OBS 采集。</p></div></div>
        <div class="overlay-setting-row"><div><b>绿幕窗口</b><small>{{ overlayStatus.green }} · {{ form.overlayGreen?.width }} × {{ form.overlayGreen?.height }}</small></div><div class="row-controls"><el-switch :model-value="Boolean(form.overlayGreen?.visible)" aria-label="启用绿幕窗口" @update:model-value="toggleOverlay('green')" /><el-button size="small" @click="updateOverlay('green', { alwaysOnTop: !form.overlayGreen?.alwaysOnTop })">{{ form.overlayGreen?.alwaysOnTop ? '已置顶' : '不置顶' }}</el-button></div></div>
        <div class="overlay-setting-row"><div><b>组件窗口</b><small>{{ overlayStatus.slot }} · {{ form.overlaySlot?.width }} × {{ form.overlaySlot?.height }}</small></div><div class="row-controls"><el-switch :model-value="Boolean(form.overlaySlot?.visible)" aria-label="启用组件窗口" @update:model-value="toggleOverlay('slot')" /><el-button size="small" @click="updateOverlay('slot', { alwaysOnTop: !form.overlaySlot?.alwaysOnTop })">{{ form.overlaySlot?.alwaysOnTop ? '已置顶' : '不置顶' }}</el-button></div></div>
        <div class="form-grid"><el-form-item v-if="form.overlayGreen" label="绿幕透明度"><el-slider v-model="form.overlayGreen.opacity" :min="0.1" :max="1" :step="0.05" /></el-form-item><el-form-item label="声音音量"><el-slider v-model="form.audioVolume" :min="0" :max="1" :step="0.05" show-input /></el-form-item></div>
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
    </div>
    <div class="about-line"><span>阿比直播工具 · {{ form.version || '0.2.0' }}</span><span>Wails + Vue 3 · 本地模拟事件源</span></div>
  </div>
</template>
