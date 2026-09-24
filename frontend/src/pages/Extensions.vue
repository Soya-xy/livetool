<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, type Component } from 'vue'
import { ElMessage } from 'element-plus'
import {
  AlarmClock,
  Camera,
  ChatDotRound,
  DataAnalysis,
  Dish,
  Grid,
  Lock,
  Microphone,
  Odometer,
  PictureRounded,
  Pointer,
  Present,
  Setting,
  Tickets,
  Trophy,
  VideoCamera,
} from '@element-plus/icons-vue'
import type { FeatureConfig, FeatureDefinition, FeatureId, FeatureSettings, FeatureValue } from '@shared/features'
import { createDefaultFeatureSettings, FEATURE_DEFINITIONS } from '@shared/features'
import type { OverlayType, OverlayWindowMode, OverlayWindowStatus } from '@shared/types'
import { api } from '../services/api'
import { useAppStore } from '../stores/app'
import FeatureSettingsDialog from '../components/FeatureSettingsDialog.vue'

const store = useAppStore()
const assets = ref<string[]>([])
const featureSettings = ref<FeatureSettings>(createDefaultFeatureSettings())
const selectedFeature = ref<FeatureDefinition | null>(null)
const settingsVisible = ref(false)
const busyId = ref<FeatureId | null>(null)
const overlayStatuses = ref<OverlayWindowStatus[]>([])
let removeOverlayStatus: (() => void) | undefined
const enabledCount = computed(() => FEATURE_DEFINITIONS.filter((feature) => configOf(feature.id).enabled).length)

const iconMap: Record<string, Component> = {
  AlarmClock,
  Camera,
  ChatDotRound,
  DataAnalysis,
  Dish,
  Grid,
  Lock,
  Microphone,
  Odometer,
  PictureRounded,
  Pointer,
  Present,
  Tickets,
  Trophy,
  VideoCamera,
}

onMounted(async () => {
  await store.init()
  featureSettings.value = createDefaultFeatureSettings(store.settings?.features)
  assets.value = await api.diagnostics.assets()
  overlayStatuses.value = await api.overlay.status()
  removeOverlayStatus = api.overlay.onStatus((status) => {
    const index = overlayStatuses.value.findIndex((item) => item.type === status.type)
    if (index >= 0) overlayStatuses.value.splice(index, 1, status)
    else overlayStatuses.value.push(status)
  })
})

onBeforeUnmount(() => removeOverlayStatus?.())

function configOf(id: FeatureId): FeatureConfig {
  return featureSettings.value[id]
}

function cloneSettings(value: FeatureSettings): FeatureSettings {
  return JSON.parse(JSON.stringify(value)) as FeatureSettings
}

async function persistFeatures(): Promise<void> {
  await store.saveSettings({ features: cloneSettings(featureSettings.value) })
}

async function toggleFeature(feature: FeatureDefinition, enabled: boolean): Promise<void> {
  configOf(feature.id).enabled = enabled
  await persistFeatures()
  try {
    if (feature.id === 'green-window' || feature.id === 'component-window') {
      const type = feature.id === 'green-window' ? 'green' : 'slot'
      if (enabled) {
        await openFeatureWindow(type, configOf(feature.id))
        if (type === 'slot') await syncEnabledComponents()
      } else await api.overlay.close(type)
    } else if (feature.id === 'virtual-camera') {
      if (enabled) {
        const connected = await api.obs.connect(store.settings?.obsUrl ?? 'ws://127.0.0.1:4455', store.settings?.obsPassword)
        if (!connected.ok) throw new Error(connected.message)
        const started = await api.obs.startVirtualCamera()
        if (!started.ok) throw new Error(started.message)
      } else if ((await api.obs.status()).virtualCameraActive) {
        const stopped = await api.obs.stopVirtualCamera()
        if (!stopped.ok) throw new Error(stopped.message)
      }
    } else if (enabled) {
      const result = await api.features.show(feature.id)
      if (!result.ok) throw new Error(result.message)
    } else {
      await api.overlay.removeWidget(feature.id)
    }
  } catch (error) {
    configOf(feature.id).enabled = false
    await persistFeatures()
    if (feature.id !== 'green-window' && feature.id !== 'component-window' && feature.id !== 'virtual-camera') await api.overlay.removeWidget(feature.id)
    ElMessage.error(error instanceof Error ? error.message : `${feature.name}启用失败`)
    return
  }
  ElMessage.success(`${feature.name}${enabled ? '已启用' : '已停用'}`)
}

function openSettings(feature: FeatureDefinition): void {
  selectedFeature.value = feature
  settingsVisible.value = true
}

async function saveFeature(config: FeatureConfig): Promise<void> {
  if (!selectedFeature.value) return
  featureSettings.value[selectedFeature.value.id] = config
  await persistFeatures()
  if (config.enabled && selectedFeature.value.id === 'green-window') await openFeatureWindow('green', config)
  if (config.enabled && selectedFeature.value.id === 'component-window') {
    await openFeatureWindow('slot', config)
    await syncEnabledComponents()
  } else if (config.enabled) {
    const result = await api.features.show(selectedFeature.value.id)
    if (!result.ok) {
      ElMessage.error(result.message)
      return
    }
  } else if (selectedFeature.value.id === 'green-window' || selectedFeature.value.id === 'component-window') {
    await api.overlay.close(selectedFeature.value.id === 'green-window' ? 'green' : 'slot')
  } else if (selectedFeature.value.id === 'virtual-camera') {
    if ((await api.obs.status()).virtualCameraActive) await api.obs.stopVirtualCamera()
  } else {
    await api.overlay.removeWidget(selectedFeature.value.id)
  }
  ElMessage.success(`${selectedFeature.value.name}设置已保存`)
}

async function syncEnabledComponents(): Promise<void> {
  for (const feature of FEATURE_DEFINITIONS) {
    if (feature.id === 'green-window' || feature.id === 'component-window' || feature.id === 'virtual-camera') continue
    if (configOf(feature.id).enabled) await api.features.show(feature.id)
  }
}

function valueOf<T extends FeatureValue>(config: FeatureConfig, key: string, fallback: T): T {
  const value = config.values[key]
  return typeof value === typeof fallback ? value as T : fallback
}

function parseList(value: string): string[] {
  return value.split(/[,，]/).map((item) => item.trim()).filter(Boolean)
}

function parseWeights(value: string, length: number): number[] {
  const values = value.split(/[,，]/).map((item) => Number(item.trim())).map((item) => Number.isFinite(item) && item >= 0 ? item : 0)
  return Array.from({ length }, (_, index) => values[index] ?? 1)
}

function statusOf(type: OverlayType): OverlayWindowStatus | undefined {
  return overlayStatuses.value.find((item) => item.type === type)
}

async function openFeatureWindow(type: OverlayType, config: FeatureConfig): Promise<void> {
  if (type === 'green') {
    await api.overlay.updateSettings('green', {
      background: valueOf(config, 'backgroundColor', '#00ff00'),
      laneCount: valueOf(config, 'laneCount', 3),
      alwaysOnTop: valueOf(config, 'alwaysOnTop', true),
    })
  } else {
    await api.overlay.updateSettings('slot', {
      width: valueOf(config, 'width', 960),
      height: valueOf(config, 'height', 540),
      alwaysOnTop: valueOf(config, 'alwaysOnTop', true),
      background: '#00ff00',
    })
  }
  await api.overlay.open(type)
}

async function setOverlayMode(type: OverlayType, mode: OverlayWindowMode): Promise<void> {
  try {
    await api.overlay.setMode(type, mode)
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : '窗口模式切换失败')
  }
}

async function toggleSlotBackground(): Promise<void> {
  const status = await api.overlay.toggleOpacity('slot')
  ElMessage.info(status.backgroundTransparent
    ? '组件窗背景已透明，OBS 窗口采集请开启「允许透明度」'
    : '组件窗已显示深色底板')
}

async function closeOverlay(type: OverlayType): Promise<void> {
  await api.overlay.close(type)
  ElMessage.info(`${type === 'green' ? '绿幕' : '组件'}窗口已关闭`)
}

async function runFeature(feature: FeatureDefinition): Promise<void> {
  if (!configOf(feature.id).enabled) {
    ElMessage.warning(`请先启用${feature.name}`)
    return
  }
  busyId.value = feature.id
  try {
    const result = await api.features.test(feature.id)
    if (!result.ok) throw new Error(result.message)
    ElMessage.success(result.message)
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : '功能演示失败')
  } finally {
    busyId.value = null
  }
}
</script>

<template>
  <div class="page-stack extensions-page feature-page">
    <div class="feature-page-head">
      <div>
        <div class="section-kicker">TOOLBOX / 03</div>
        <h1>扩展功能</h1>
        <p>独立启用和设置，修改后自动保存。</p>
      </div>
      <div class="feature-page-summary">
        <strong>{{ enabledCount }}<small>/{{ FEATURE_DEFINITIONS.length }}</small></strong>
        <span>已启用功能</span>
      </div>
    </div>

    <div v-if="statusOf('green')?.visible || statusOf('slot')?.visible" class="overlay-window-tools">
      <div v-if="statusOf('green')?.visible" class="overlay-window-toolbar">
        <div class="overlay-window-label"><b>ylm</b><span>绿幕窗口</span><small>{{ statusOf('green')?.width }} × {{ statusOf('green')?.height }}</small></div>
        <el-button type="success" size="small" @click="setOverlayMode('green', 'green')">绿屏</el-button>
        <el-button type="primary" size="small" @click="setOverlayMode('green', 'landscape-16-9')">横屏（16:9）</el-button>
        <el-button size="small" @click="setOverlayMode('green', 'landscape-4-3')">横屏（4:3）</el-button>
        <el-button type="primary" size="small" @click="setOverlayMode('green', 'portrait-9-16')">竖屏（9:16）</el-button>
        <el-button type="danger" size="small" @click="setOverlayMode('green', 'fullscreen')">全屏</el-button>
        <el-button link size="small" @click="closeOverlay('green')">关闭</el-button>
      </div>
      <div v-if="statusOf('slot')?.visible" class="overlay-window-toolbar component-toolbar">
        <div class="overlay-window-label"><b>yapp</b><span>组件窗口</span><small>{{ statusOf('slot')?.width }} × {{ statusOf('slot')?.height }}</small></div>
        <el-button type="primary" size="small" @click="setOverlayMode('slot', 'landscape-16-9')">横屏</el-button>
        <el-button size="small" @click="setOverlayMode('slot', 'portrait-9-16')">竖屏</el-button>
        <el-button size="small" title="切换组件窗底板（Ctrl / ⌘ + F1）" aria-label="切换组件窗底板（Ctrl / ⌘ + F1）" @click="toggleSlotBackground">{{ statusOf('slot')?.backgroundTransparent ? '底板：透明' : '底板：深色' }}</el-button>
        <el-button type="danger" size="small" @click="setOverlayMode('slot', 'fullscreen')">全屏</el-button>
        <el-button link size="small" @click="closeOverlay('slot')">关闭</el-button>
      </div>
    </div>

    <div class="feature-grid">
      <article
        v-for="feature in FEATURE_DEFINITIONS"
        :key="feature.id"
        class="feature-card"
        :class="{ muted: feature.muted, enabled: configOf(feature.id).enabled }"
        :style="{ '--feature-accent': feature.accent }"
      >
        <div class="feature-icon"><el-icon><component :is="iconMap[feature.icon]" /></el-icon></div>
        <div class="feature-copy">
          <div class="feature-title">
            <h3>{{ feature.name }}</h3>
            <el-tag v-if="feature.badge" size="small" effect="dark">{{ feature.badge }}</el-tag>
          </div>
          <p>{{ feature.description }}</p>
          <el-button class="feature-run" link :loading="busyId === feature.id" @click.stop="runFeature(feature)">测试功能</el-button>
        </div>
        <div class="feature-controls">
          <el-switch :model-value="configOf(feature.id).enabled" :aria-label="`启用${feature.name}`" @update:model-value="toggleFeature(feature, Boolean($event))" />
          <el-button class="gear-button" link :aria-label="`打开${feature.name}设置`" @click.stop="openSettings(feature)"><el-icon><Setting /></el-icon></el-button>
        </div>
      </article>
    </div>

    <div class="feature-footnote">
      <span>用开关启停功能，点击齿轮设置；测试会演示当前功能。</span>
      <span>{{ assets.length }} 个可用素材</span>
    </div>

    <FeatureSettingsDialog
      v-model="settingsVisible"
      :feature="selectedFeature"
      :config="selectedFeature ? configOf(selectedFeature.id) : null"
      @save="saveFeature"
    />
  </div>
</template>
