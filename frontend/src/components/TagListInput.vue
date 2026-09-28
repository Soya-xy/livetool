<script setup lang="ts">
// 触发内容 / 关键词 / 指定用户这类「一串值」的编辑控件，对应原版编辑弹窗里
// 「触发内容 ［啤酒 ×］［+ 添加］」的形态：平时只显示标签和「+ 添加」按钮，
// 点添加后才出现输入框（回车也能加），输入框空着失焦会自动收起。
import { nextTick, ref } from 'vue'
import { Plus } from '@element-plus/icons-vue'

const props = defineProps<{ modelValue: string[]; placeholder?: string; ariaLabel?: string }>()
const emit = defineEmits<{ 'update:modelValue': [string[]] }>()

const adding = ref(false)
const draft = ref('')
const entry = ref<InstanceType<typeof import('element-plus')['ElInput']> | null>(null)

function beginAdd(): void {
  adding.value = true
  void nextTick(() => (entry.value as unknown as { focus?: () => void } | null)?.focus?.())
}

function add(): void {
  const value = draft.value.trim()
  if (!value) return
  if (!props.modelValue.includes(value)) emit('update:modelValue', [...props.modelValue, value])
  draft.value = ''
  adding.value = false
}

function cancel(): void {
  if (draft.value.trim()) return
  adding.value = false
}

function remove(index: number): void {
  emit('update:modelValue', props.modelValue.filter((_, item) => item !== index))
}
</script>

<template>
  <div class="tag-list-input">
    <el-tag v-for="(tag, index) in modelValue" :key="`${tag}-${index}`" closable type="primary" effect="light" @close="remove(index)">{{ tag }}</el-tag>
    <el-input
      v-if="adding"
      ref="entry"
      v-model="draft"
      class="tag-list-entry"
      :placeholder="placeholder ?? '输入后回车'"
      :aria-label="ariaLabel ?? '新增内容'"
      @keyup.enter="add"
      @blur="cancel"
    />
    <el-button v-else size="small" plain @click="beginAdd"><el-icon><Plus /></el-icon>添加</el-button>
  </div>
</template>
