<script setup lang="ts">
import { computed, onBeforeUnmount, onErrorCaptured, onMounted, ref, watch, watchEffect } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ChatDotRound, CircleCheck, Connection, Grid, MagicStick, Setting, SwitchButton } from '@element-plus/icons-vue'
import { Events, Updater } from '@wailsio/runtime'
import { ElMessage, ElMessageBox } from 'element-plus'
import type { LicenseAuthStatus, Platform } from '@shared/types'
import { useAppStore } from './stores/app'
import { api } from './services/api'

const route = useRoute()
const router = useRouter()
const store = useAppStore()
const authVisible = ref(false)
const authCode = ref('')
const authPlatform = ref<Platform>('simulator')
const safeCode = ref('')
const authStatus = ref<LicenseAuthStatus>({ loggedIn: false, mode: 'remote', platform: 'simulator', features: [] })
const authLoading = ref(false)
const safeCodeLoading = ref(false)
const localDevelopmentAvailable = ref(false)
const pageError = ref('')
const updateProgress = ref<number | null>(null)
const fatalInfos = new Set(['render function', 'component update', 'setup function'])
let authPolling: ReturnType<typeof setInterval> | undefined
let updatePolling: ReturnType<typeof setInterval> | undefined
const removeUpdateListeners: Array<() => void> = []

const overlayPage = computed(() => route.path.startsWith('/overlay-'))
// Overlay 窗口本体是透明窗口：html/body 都不能画底色，否则透明会被底色盖住（:root 上就有 --paper 底色）。
watchEffect(() => {
  document.documentElement.classList.toggle('overlay-body', overlayPage.value)
  document.body.classList.toggle('overlay-body', overlayPage.value)
})
const navItems = [
  { path: '/control', label: '控制中心', icon: Grid },
  { path: '/diary', label: '弹幕日记', icon: ChatDotRound },
  { path: '/extensions', label: '扩展功能', icon: MagicStick },
  { path: '/settings', label: '通用设置', icon: Setting },
]

onMounted(() => {
  void api.auth.developmentModeAvailable().then((allowed) => { localDevelopmentAvailable.value = allowed }).catch(() => undefined)
  void refreshAuthStatus(true)
  authPolling = setInterval(() => { void refreshAuthStatus() }, 60_000)
  removeUpdateListeners.push(Events.On(Updater.Events.UpdateAvailable, (event) => {
    const release = event.data
    void ElMessageBox.confirm(`发现新版本 ${release.version}，现在下载并安装吗？`, '发现更新', { confirmButtonText: '下载更新', cancelButtonText: '稍后', type: 'info' })
      .then(() => installUpdate())
      .catch(() => undefined)
  }))
  removeUpdateListeners.push(Events.On(Updater.Events.DownloadStarted, () => { updateProgress.value = 0 }))
  removeUpdateListeners.push(Events.On(Updater.Events.DownloadProgress, (event) => {
    if (event.data.total > 0) updateProgress.value = Math.min(100, Math.round(event.data.written * 100 / event.data.total))
  }))
  removeUpdateListeners.push(Events.On(Updater.Events.UpdateReady, (event) => {
    updateProgress.value = null
    void ElMessageBox.confirm(`版本 ${event.data.version} 已准备好，重启应用后生效。`, '更新完成', { confirmButtonText: '重启应用', cancelButtonText: '稍后', type: 'success' })
      .then(() => api.updater.restart())
      .catch(() => undefined)
  }))
  removeUpdateListeners.push(Events.On(Updater.Events.Error, (event) => {
    updateProgress.value = null
    ElMessage.error(`更新失败：${event.data.message}`)
  }))
})
onBeforeUnmount(() => {
  if (authPolling) clearInterval(authPolling)
  if (updatePolling) clearInterval(updatePolling)
  removeUpdateListeners.forEach((remove) => remove())
})

// 页面渲染崩溃时兜底：只拦渲染/挂载阶段的错误，动作类报错继续交给全局 errorHandler。
onErrorCaptured((error, _instance, info) => {
  if (!fatalInfos.has(info)) return
  pageError.value = error instanceof Error ? error.message : String(error)
  return false
})
watch(() => route.fullPath, () => { pageError.value = '' })

function retryPage(): void { pageError.value = '' }
function recoverPage(path = '/control'): void { pageError.value = ''; void router.push(path) }
function navigate(path: string): void { void router.push(path) }
async function refreshAuthStatus(loadSettings = false): Promise<void> {
  try {
    if (loadSettings) {
      const settings = await api.diagnostics.settings()
      authPlatform.value = settings.platform || authPlatform.value
    }
    authStatus.value = await api.auth.status()
  } catch {
    authStatus.value = { loggedIn: false, mode: 'remote', platform: authPlatform.value, features: [] }
  }
  if (authStatus.value.loggedIn && !updatePolling) {
    void checkForUpdate(true)
    updatePolling = setInterval(() => { if (authStatus.value.loggedIn) void checkForUpdate(true) }, 30 * 60_000)
  } else if (!authStatus.value.loggedIn && updatePolling) {
    clearInterval(updatePolling)
    updatePolling = undefined
  }
}
function openAuth(): void { authVisible.value = true; void refreshAuthStatus(true) }
async function connect(): Promise<void> {
  try {
    const roomId = store.settings?.roomId || 'demo-room'
    await store.connect(store.settings?.platform || 'simulator', roomId)
    ElMessage.success('已连接本地模拟事件源')
  } catch (error) { ElMessage.error(error instanceof Error ? error.message : '连接失败') }
}
async function login(local = false): Promise<void> {
  authLoading.value = true
  try {
    const result = await api.auth.login({ platform: authPlatform.value, code: authCode.value, local })
    if (!result.loggedIn) { ElMessage.error(result.message); return }
    authVisible.value = false
    authCode.value = ''
    await refreshAuthStatus()
    ElMessage.success(result.message)
  } catch (error) { ElMessage.error(error instanceof Error ? error.message : '登录失败') }
  finally { authLoading.value = false }
}
async function logout(): Promise<void> {
  try {
    await api.auth.logout()
    await refreshAuthStatus()
    ElMessage.success('已退出授权')
  } catch (error) { ElMessage.error(error instanceof Error ? error.message : '退出授权失败') }
}
async function checkForUpdate(quiet = false): Promise<void> {
  try {
    const result = await api.updater.check()
    if (!result.ok) ElMessage.warning(result.message)
    else if (!quiet) ElMessage.success(result.message)
  } catch (error) { ElMessage.error(error instanceof Error ? error.message : '检查更新失败') }
}
async function installUpdate(): Promise<void> {
  try {
    const result = await api.updater.install()
    if (!result.ok) ElMessage.error(result.message)
    else ElMessage.info('正在下载安装更新…')
  } catch (error) { ElMessage.error(error instanceof Error ? error.message : '安装更新失败') }
}
async function saveSafeCode(): Promise<void> {
  safeCodeLoading.value = true
  try {
    const result = await api.auth.setSafeCode(safeCode.value)
    if (!result.ok) { ElMessage.error(result.message); return }
    safeCode.value = ''
    await refreshAuthStatus()
    ElMessage.success(result.message)
  } catch (error) { ElMessage.error(error instanceof Error ? error.message : '安全码设置失败') }
  finally { safeCodeLoading.value = false }
}
async function unbindDevice(): Promise<void> {
  try {
    await ElMessageBox.confirm('解除后此安装将退出授权；如设备数已满，需要管理员重置后才能重新绑定。', '解除本机绑定', { type: 'warning' })
    const result = await api.auth.unbind(safeCode.value)
    if (!result.ok) { ElMessage.error(result.message); return }
    safeCode.value = ''
    await refreshAuthStatus()
    ElMessage.success(result.message)
  } catch (error) {
    if (error !== 'cancel' && error !== 'close') ElMessage.error(error instanceof Error ? error.message : '设备解绑失败')
  }
}
</script>

<template>
  <div v-if="overlayPage" class="overlay-root"><router-view /></div>
  <div v-else class="app-shell">
    <a class="skip-link" href="#main-content">跳到主内容</a>
    <aside class="sidebar">
      <div class="brand-block">
        <div class="brand-mark" aria-hidden="true"><span>AB</span><i /></div>
        <div><div class="brand-name">阿比整蛊</div><div class="brand-version">ver {{ store.settings?.version ?? '0.2.0' }}</div></div>
      </div>
      <nav class="side-nav" aria-label="主导航">
        <RouterLink v-for="item in navItems" :key="item.path" :to="item.path" class="nav-item" :class="{ active: route.path === item.path }" :aria-current="route.path === item.path ? 'page' : undefined">
          <el-icon><component :is="item.icon" /></el-icon><span>{{ item.label }}</span>
          <b v-if="item.path === '/diary' && store.unread">{{ Math.min(store.unread, 99) }}</b>
        </RouterLink>
      </nav>
      <div class="sidebar-spacer" />
      <button class="connection-link" @click="connect">
        <span class="status-dot" :class="store.status.state" /><span>{{ store.status.state === 'connected' ? '已连接模拟器' : '连接到直播间' }}</span>
        <el-icon><Connection /></el-icon>
      </button>
      <button class="quit-link" @click="api.window.close"><el-icon><SwitchButton /></el-icon><span>退出程序</span></button>
      <div class="sidebar-footnote">本地模拟事件源 · 授权后启用互动功能</div>
    </aside>

    <main class="workspace">
      <header class="titlebar">
        <div class="titlebar-left"><span class="workspace-indicator" aria-hidden="true" /><strong>直播工作区</strong><span class="titlebar-context">LOCAL</span></div>
        <div class="titlebar-right">
          <span v-if="updateProgress !== null" class="update-progress" role="status" aria-live="polite">正在更新 {{ updateProgress }}%</span>
          <button v-else-if="authStatus.loggedIn" class="subtle-action" @click="() => checkForUpdate(false)">检查更新</button>
          <button class="auth-status-action" :class="{ active: authStatus.loggedIn }" @click="openAuth">{{ authStatus.mode === 'local' && authStatus.loggedIn ? '本地开发模式' : authStatus.loggedIn ? '卡密已验证' : '验证卡密' }}</button>
        </div>
      </header>
      <section id="main-content" class="page-area" tabindex="-1">
        <div v-if="pageError" class="page-fallback">
          <div class="page-fallback-mark">!</div>
          <h3>页面加载失败</h3>
          <p>{{ pageError }}</p>
          <div class="page-fallback-actions"><el-button type="primary" @click="retryPage">重试</el-button><el-button @click="recoverPage()">返回控制中心</el-button></div>
        </div>
        <router-view v-else />
      </section>
      <footer class="status-footer">
        <div class="status-summary" role="status" aria-live="polite"><span class="status-dot" :class="store.status.state" aria-hidden="true" /><span>{{ store.status.state === 'connected' ? `已连接 · ${store.status.platform}` : '未连接直播间' }}</span><span class="footer-separator" aria-hidden="true">·</span><span>队列 {{ store.status.dropped ? `丢弃 ${store.status.dropped}` : '正常' }}</span></div>
        <div class="footer-actions"><span class="safe-label">{{ authStatus.mode === 'local' && authStatus.loggedIn ? '本地开发模式' : authStatus.loggedIn ? '卡密有效' : '未验证' }}</span><button @click="openAuth">{{ authStatus.loggedIn ? '授权设置' : '验证卡密' }}</button></div>
      </footer>
    </main>

    <el-dialog v-model="authVisible" title="卡密验证" width="380px" class="auth-dialog">
      <div class="auth-intro"><div class="auth-icon" aria-hidden="true"><el-icon><CircleCheck /></el-icon></div><div><b>验证设备授权</b><p>输入卡密后，本机将绑定到授权服务。</p></div></div>
      <el-alert v-if="authStatus.loggedIn" :closable="false" type="success" show-icon :title="authStatus.mode === 'local' ? '当前为本地开发模式' : `已授权至 ${authStatus.expiresAt ? new Date(authStatus.expiresAt).toLocaleString() : '有效期未知'}`" class="auth-state-alert">
        <template #default><span>功能权限：{{ authStatus.features.join('、') || '无' }}</span></template>
      </el-alert>
      <el-form label-position="top">
        <el-form-item label="卡密"><el-input v-model="authCode" name="license-code" autocomplete="off" spellcheck="false" placeholder="输入你的卡密" clearable show-password @keyup.enter="login" /></el-form-item>
        <template v-if="authStatus.loggedIn && authStatus.mode === 'remote'">
          <el-form-item :label="authStatus.safeCodeSet ? '更新安全码' : '设置安全码'"><el-input v-model="safeCode" type="password" show-password placeholder="至少 6 位，用于解绑本机" /></el-form-item>
          <div class="auth-safe-actions"><el-button :loading="safeCodeLoading" @click="saveSafeCode">保存安全码</el-button><el-button type="danger" plain :loading="safeCodeLoading" @click="unbindDevice">解除本机绑定</el-button></div>
        </template>
      </el-form>
      <div v-if="localDevelopmentAvailable" class="dev-auth-entry"><span>开发调试</span><el-button link type="info" :loading="authLoading" @click="login(true)">进入本地开发模式</el-button></div>
      <template #footer><el-button v-if="authStatus.loggedIn" @click="logout">退出授权</el-button><el-button @click="authVisible = false">取消</el-button><el-button type="primary" :loading="authLoading" @click="login()">验证卡密</el-button></template>
    </el-dialog>
  </div>
</template>
