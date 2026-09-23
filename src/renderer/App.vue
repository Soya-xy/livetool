<script setup lang="ts">
import { computed, onBeforeUnmount, onErrorCaptured, onMounted, ref, watch, watchEffect } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import {
  Bell, ChatDotRound, CircleClose, Connection, Grid, Key, MagicStick, Minus, Monitor, Setting, SwitchButton, VideoCamera,
} from '@element-plus/icons-vue'
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
const authServerUrl = ref('http://127.0.0.1:8787')
const safeCode = ref('')
const authStatus = ref<LicenseAuthStatus>({ loggedIn: false, mode: 'remote', platform: 'simulator', features: [] })
const authLoading = ref(false)
const safeCodeLoading = ref(false)
const pageError = ref('')
const fatalInfos = new Set(['render function', 'component update', 'setup function'])
let authPolling: ReturnType<typeof setInterval> | undefined

const overlayPage = computed(() => route.path.startsWith('/overlay-'))
// Overlay 窗口本体是透明窗口：html/body 都不能画底色，否则透明会被底色盖住（:root 上就有 --paper 底色）。
watchEffect(() => {
  document.documentElement.classList.toggle('overlay-body', overlayPage.value)
  document.body.classList.toggle('overlay-body', overlayPage.value)
})
const currentTitle = computed(() => ({ '/control': '控制中心', '/diary': '弹幕日记', '/extensions': '扩展功能', '/settings': '通用设置', '/license-admin': '卡密管理' }[route.path] ?? 'Overlay'))
const navItems = [
  { path: '/control', label: '控制中心', icon: Grid },
  { path: '/diary', label: '弹幕日记', icon: ChatDotRound },
  { path: '/extensions', label: '扩展功能', icon: MagicStick },
  { path: '/settings', label: '通用设置', icon: Setting },
  { path: '/license-admin', label: '卡密管理', icon: Key },
]

onMounted(() => {
  void refreshAuthStatus(true)
  authPolling = setInterval(() => { void refreshAuthStatus() }, 60_000)
})
onBeforeUnmount(() => { if (authPolling) clearInterval(authPolling) })

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
    if (loadSettings) authServerUrl.value = (await api.diagnostics.settings()).authServerUrl || authServerUrl.value
    authStatus.value = await api.auth.status()
  } catch {
    authStatus.value = { loggedIn: false, mode: 'remote', platform: authPlatform.value, features: [] }
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
async function login(local: boolean): Promise<void> {
  authLoading.value = true
  try {
    if (!local) await api.diagnostics.saveSettings({ authServerUrl: authServerUrl.value.trim() })
    const result = await api.auth.login({ platform: authPlatform.value, code: authCode.value, local })
    if (!result.loggedIn) { ElMessage.error(result.message); return }
    authVisible.value = false
    authCode.value = ''
    await refreshAuthStatus()
    ElMessage.success(result.message)
  } catch (error) { ElMessage.error(error instanceof Error ? error.message : '登录失败') }
  finally { authLoading.value = false }
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
    <aside class="sidebar">
      <div class="brand-block">
        <div class="brand-mark"><span>AB</span><i /></div>
        <div><div class="brand-name">阿比整蛊</div><div class="brand-version">ver 7.6.3</div></div>
      </div>
      <nav class="side-nav">
        <button v-for="item in navItems" :key="item.path" class="nav-item" :class="{ active: route.path === item.path }" @click="navigate(item.path)">
          <el-icon><component :is="item.icon" /></el-icon><span>{{ item.label }}</span>
          <b v-if="item.path === '/diary' && store.unread">{{ Math.min(store.unread, 99) }}</b>
        </button>
      </nav>
      <div class="sidebar-spacer" />
      <button class="connection-link" @click="connect">
        <span class="status-dot" :class="store.status.state" /><span>{{ store.status.state === 'connected' ? '已连接模拟器' : '连接到直播间' }}</span>
        <el-icon><Connection /></el-icon>
      </button>
      <button class="quit-link" @click="api.window.close"><el-icon><SwitchButton /></el-icon><span>退出程序</span></button>
      <div class="sidebar-footnote">安全本地模式 · 不包含原版授权密钥</div>
    </aside>

    <main class="workspace">
      <header class="titlebar">
        <div class="titlebar-left"><span class="eyebrow">LIVE INTERACTION CONSOLE</span><span class="title-divider">/</span><strong>{{ currentTitle }}</strong></div>
        <div class="titlebar-right">
          <button class="title-icon" title="通知"><el-icon><Bell /></el-icon></button>
          <button class="title-icon" title="最小化" @click="api.window.minimize"><el-icon><Minus /></el-icon></button>
          <button class="title-icon" title="关闭" @click="api.window.close"><el-icon><CircleClose /></el-icon></button>
        </div>
      </header>
      <div class="notice-strip"><el-icon><Monitor /></el-icon><span>7.6.3 新增手势拍蚊子、手机玩法铁链/视频特效/垃圾掉落与虚拟摄像头。当前为本地模拟闭环，可随时替换连接器。</span><button @click="openAuth">授权 / 模式</button></div>
      <section class="page-area">
        <div v-if="pageError" class="page-fallback">
          <div class="page-fallback-mark">!</div>
          <h3>页面加载失败</h3>
          <p>{{ pageError }}</p>
          <div class="page-fallback-actions"><el-button type="primary" @click="retryPage">重试</el-button><el-button @click="recoverPage()">返回控制中心</el-button></div>
        </div>
        <router-view v-else />
      </section>
      <footer class="status-footer">
        <div class="status-summary"><span class="status-dot" :class="store.status.state" /><span>{{ store.status.state === 'connected' ? `已连接 · ${store.status.platform}` : '未连接直播间' }}</span><span class="footer-separator">·</span><span>队列 {{ store.status.dropped ? `丢弃 ${store.status.dropped}` : '正常' }}</span></div>
        <div class="footer-actions"><span class="safe-label">{{ authStatus.mode === 'local' && authStatus.loggedIn ? '本地开发模式' : authStatus.loggedIn ? '卡密已授权' : '未授权' }}</span><button @click="openAuth">登录 / 切换模式</button></div>
      </footer>
    </main>

    <el-dialog v-model="authVisible" title="登录 / 运行模式" width="500px" class="auth-dialog">
      <div class="auth-intro"><div class="auth-icon">◎</div><div><b>自建授权服务</b><p>卡密由自建服务验证；本地开发模式仅在未打包的开发版开放。</p></div></div>
      <el-alert v-if="authStatus.loggedIn" :closable="false" type="success" show-icon :title="authStatus.mode === 'local' ? '当前为本地开发模式' : `已授权至 ${authStatus.expiresAt ? new Date(authStatus.expiresAt).toLocaleString() : '有效期未知'}`" class="auth-state-alert">
        <template v-if="authStatus.mode === 'remote'" #default><span>功能权限：{{ authStatus.features.join('、') || '无' }}</span></template>
      </el-alert>
      <el-form label-position="top">
        <el-form-item label="直播平台"><el-select v-model="authPlatform" style="width: 100%"><el-option label="本地模拟器" value="simulator" /><el-option label="抖音" value="douyin" /><el-option label="快手" value="kuaishou" /><el-option label="视频号" value="shipinhao" /><el-option label="B站" value="bilibili" /><el-option label="TikTok" value="tiktok" /><el-option label="斗鱼" value="douyu" /><el-option label="小红书" value="xiaohongshu" /></el-select></el-form-item>
        <el-form-item label="授权服务器地址"><el-input v-model="authServerUrl" placeholder="http://127.0.0.1:8787（远程服务需 HTTPS）" /></el-form-item>
        <el-form-item label="卡密"><el-input v-model="authCode" placeholder="输入自建授权服务生成的卡密" clearable show-password /></el-form-item>
        <template v-if="authStatus.mode === 'remote' && authStatus.loggedIn">
          <el-form-item :label="authStatus.safeCodeSet ? '更新安全码' : '设置安全码'"><el-input v-model="safeCode" type="password" show-password placeholder="至少 6 位，用于解绑本机" /></el-form-item>
          <div class="auth-safe-actions"><el-button :loading="safeCodeLoading" @click="saveSafeCode">保存安全码</el-button><el-button type="danger" plain :loading="safeCodeLoading" @click="unbindDevice">解除本机绑定</el-button></div>
        </template>
      </el-form>
      <template #footer><el-button v-if="authStatus.loggedIn" @click="api.auth.logout().then(() => refreshAuthStatus())">退出授权</el-button><el-button @click="authVisible = false">关闭</el-button><el-button type="primary" :loading="authLoading" @click="login(true)">进入本地模式</el-button><el-button type="info" plain :loading="authLoading" @click="login(false)">验证卡密</el-button></template>
    </el-dialog>
  </div>
</template>
