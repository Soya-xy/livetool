<script setup lang="ts">
import { onBeforeUnmount, ref } from 'vue'

/**
 * 热键录入控件（控制中心规则编辑器使用）。
 * 可录范围与 Wails 全局快捷键一致：主键盘字母 A-Z、数字 0-9、F1-F12，
 * 必须搭配 Ctrl / Shift / Alt 中的至少一个修饰键；修饰键本身不触发录入。
 */
const props = defineProps<{ modelValue?: string; placeholder?: string; disabled?: boolean }>()
const emit = defineEmits<{ 'update:modelValue': [string] }>()

const recording = ref(false)
const hint = ref('')

const modifierKeys = new Set(['Shift', 'Control', 'Alt', 'Meta', 'AltGraph', 'CapsLock', 'NumLock', 'ScrollLock', 'ContextMenu'])
const mainKeyPattern = /^(Key[A-Z]|Digit[0-9]|F([1-9]|1[0-2]))$/

function keyLabel(code: string): string {
  if (code.startsWith('Key')) return code.slice(3)
  if (code.startsWith('Digit')) return code.slice(5)
  return code
}

function start(): void {
  if (props.disabled || recording.value) return
  recording.value = true
  hint.value = ''
  window.addEventListener('keydown', handleKeydown, true)
}
function stop(): void {
  recording.value = false
  window.removeEventListener('keydown', handleKeydown, true)
}
function clear(): void {
  emit('update:modelValue', '')
  hint.value = '已解除热键'
}
function handleKeydown(event: KeyboardEvent): void {
  event.preventDefault()
  event.stopPropagation()
  if (event.key === 'Escape') { stop(); return }
  if (event.key === 'Backspace' || event.key === 'Delete') { emit('update:modelValue', ''); stop(); hint.value = '已解除热键'; return }
  if (modifierKeys.has(event.key)) return
  if (!mainKeyPattern.test(event.code)) {
    hint.value = '只支持字母、主键盘数字和 F1-F12（小键盘不可用）'
    return
  }
  const parts: string[] = []
  if (event.ctrlKey) parts.push('Ctrl')
  if (event.shiftKey) parts.push('Shift')
  if (event.altKey) parts.push('Alt')
  if (!parts.length) {
    hint.value = '至少需要一个修饰键（Ctrl / Shift / Alt）'
    return
  }
  parts.push(keyLabel(event.code))
  emit('update:modelValue', parts.join(' + '))
  hint.value = ''
  stop()
}

onBeforeUnmount(stop)
</script>

<template>
  <div class="hotkey-input">
    <button
      type="button"
      class="hotkey-field"
      :class="{ recording, empty: !props.modelValue }"
      :disabled="props.disabled"
      :aria-label="recording ? '正在录入热键' : '点击录入热键'"
      @click="start"
      @blur="stop"
    >
      <span v-if="recording">请按下组合键…</span>
      <span v-else-if="props.modelValue">{{ props.modelValue }}</span>
      <span v-else class="hotkey-placeholder">{{ props.placeholder ?? '点击录入热键' }}</span>
    </button>
    <el-button size="small" text :disabled="props.disabled || !props.modelValue" @click="clear">解除热键</el-button>
    <small v-if="hint" class="hotkey-hint">{{ hint }}</small>
    <small v-else class="hotkey-hint">支持 Ctrl / Shift / Alt + 字母、数字、F1-F12</small>
  </div>
</template>
