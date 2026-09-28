<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { Refresh, UploadFilled } from '@element-plus/icons-vue'
import type { AssetKind } from '@shared/types'
import { api } from '../services/api'

/** 素材控件：从素材目录里挑选图片 / 视频 / 声音，也可以上传本地文件或手填相对路径。 */
const props = defineProps<{ modelValue?: string; kind: AssetKind; placeholder?: string; disabled?: boolean }>()
const emit = defineEmits<{ 'update:modelValue': [string] }>()

const extensions: Record<AssetKind, string[]> = {
  image: ['.png', '.jpg', '.jpeg', '.webp', '.gif', '.bmp', '.svg'],
  video: ['.mp4', '.webm', '.mov', '.m4v'],
  audio: ['.mp3', '.wav', '.ogg', '.m4a', '.aac', '.flac'],
}
const kindLabels: Record<AssetKind, string> = { image: '图片', video: '视频', audio: '声音' }

// 素材目录扫描结果在多个控件之间共享，避免每个动作都请求一次。
const assets = ref<string[]>([])
const loading = ref(false)
const uploading = ref(false)
let loaded = false

async function loadAssets(force = false): Promise<void> {
  if (loaded && !force) return
  loading.value = true
  try {
    assets.value = await api.diagnostics.assets()
    loaded = true
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : '读取素材目录失败')
  } finally {
    loading.value = false
  }
}

const options = computed(() => assets.value.filter((item) => extensions[props.kind].some((extension) => item.toLowerCase().endsWith(extension))))

// 上传：选择本地文件 → 复制进素材目录 → 直接把相对路径填回当前字段。
async function upload(): Promise<void> {
  if (props.disabled || uploading.value) return
  uploading.value = true
  try {
    const path = await api.diagnostics.importAsset(props.kind)
    if (!path) return
    emit('update:modelValue', path)
    await loadAssets(true)
    ElMessage.success(`已上传${kindLabels[props.kind]}素材：${path}`)
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : '上传素材失败')
  } finally {
    uploading.value = false
  }
}

onMounted(() => { void loadAssets() })
</script>

<template>
  <div class="asset-select">
    <el-select
      :model-value="props.modelValue || ''"
      filterable
      allow-create
      clearable
      :disabled="props.disabled"
      :placeholder="props.placeholder ?? `选择素材目录里的${kindLabels[props.kind]}，或点右侧上传`"
      @update:model-value="emit('update:modelValue', $event ?? '')"
    >
      <el-option v-for="item in options" :key="item" :label="item" :value="item" />
      <template #empty><span class="asset-select-empty">素材目录里还没有{{ kindLabels[props.kind] }}素材，点「上传」从本机导入</span></template>
    </el-select>
    <el-button size="small" :loading="uploading" :disabled="props.disabled" aria-label="上传素材" @click="upload"><el-icon><UploadFilled /></el-icon>上传</el-button>
    <el-button size="small" :loading="loading" :disabled="props.disabled" aria-label="刷新素材列表" @click="loadAssets(true)"><el-icon><Refresh /></el-icon></el-button>
  </div>
</template>
