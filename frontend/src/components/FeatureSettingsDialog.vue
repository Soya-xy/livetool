<script setup lang="ts">
import { ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import type { FeatureConfig, FeatureDefinition, FeatureField, FeatureGiftRule, FeatureValue } from '@shared/features'
import { createDefaultFeatureSettings } from '@shared/features'

const props = defineProps<{
  modelValue: boolean
  feature: FeatureDefinition | null
  config: FeatureConfig | null
}>()

const emit = defineEmits<{
  'update:modelValue': [value: boolean]
  save: [value: FeatureConfig]
}>()

const draft = ref<FeatureConfig | null>(null)
const expandedGiftRules = ref<string[]>([])

watch(() => props.modelValue, (visible) => {
  if (visible) loadDraft()
})

watch(() => props.config, () => {
  if (props.modelValue) loadDraft()
}, { deep: true })

function loadDraft(): void {
  if (!props.config && !props.feature) {
    draft.value = null
    return
  }
  const fallback = props.feature ? createDefaultFeatureSettings()[props.feature.id] : null
  const source = props.config ?? fallback
  draft.value = source ? JSON.parse(JSON.stringify(source)) as FeatureConfig : null
}

function fieldValue(field: FeatureField): FeatureValue {
  return draft.value?.values[field.key] ?? field.defaultValue
}

function setField(key: string, value: unknown): void {
  if (!draft.value) return
  if (typeof value === 'string' || typeof value === 'number' || typeof value === 'boolean') draft.value.values[key] = value
}

function numberValue(field: FeatureField): number {
  const value = fieldValue(field)
  return typeof value === 'number' && Number.isFinite(value) ? value : Number(field.defaultValue) || 0
}

function fieldsForGiftRules(): FeatureField[] {
  return props.feature?.fields.filter((field) => !field.giftOnly) ?? []
}

function inlineGiftFields(): FeatureField[] {
  return props.feature?.fields.filter((field) => field.giftOnly) ?? []
}

function giftRuleValue(rule: FeatureGiftRule, field: FeatureField): FeatureValue {
  return rule.values?.[field.key] ?? fieldValue(field)
}

function giftRuleNumberValue(rule: FeatureGiftRule, field: FeatureField): number {
  const value = giftRuleValue(rule, field)
  return typeof value === 'number' && Number.isFinite(value) ? value : Number(field.defaultValue) || 0
}

function setGiftRuleField(rule: FeatureGiftRule, field: FeatureField, value: unknown): void {
  if (typeof value !== 'string' && typeof value !== 'number' && typeof value !== 'boolean') return
  rule.values ??= {}
  rule.values[field.key] = value
}

function hasGiftRuleOverride(rule: FeatureGiftRule, field: FeatureField): boolean {
  return rule.values?.[field.key] !== undefined
}

function resetGiftRuleField(rule: FeatureGiftRule, field: FeatureField): void {
  if (!rule.values) return
  delete rule.values[field.key]
}

function isGiftRuleExpanded(id: string): boolean {
  return expandedGiftRules.value.includes(id)
}

function toggleGiftRuleExpanded(id: string): void {
  expandedGiftRules.value = isGiftRuleExpanded(id)
    ? expandedGiftRules.value.filter((item) => item !== id)
    : [...expandedGiftRules.value, id]
}

function addGiftRule(): void {
  if (!draft.value) return
  const rule: FeatureGiftRule = { id: crypto.randomUUID(), giftName: '', action: '增加', enabled: true, values: {} }
  draft.value.giftRules.push(rule)
  expandedGiftRules.value = [...expandedGiftRules.value, rule.id]
}

function removeGiftRule(id: string): void {
  if (!draft.value) return
  draft.value.giftRules = draft.value.giftRules.filter((rule) => rule.id !== id)
}

function save(): void {
  if (!draft.value) return
  const giftNames = new Set<string>()
  for (const rule of draft.value.giftRules) {
    rule.giftName = rule.giftName.trim()
    if (!rule.giftName) {
      ElMessage.warning('请填写每条礼物配置的礼物名称')
      return
    }
    const key = rule.giftName.toLocaleLowerCase()
    if (giftNames.has(key)) {
      ElMessage.warning(`礼物“${rule.giftName}”已有配置，请在同一行修改参数`)
      return
    }
    giftNames.add(key)
  }
  emit('save', JSON.parse(JSON.stringify(draft.value)) as FeatureConfig)
  emit('update:modelValue', false)
}
</script>

<template>
  <el-dialog
    :model-value="modelValue"
    :title="feature ? `${feature.name}设置` : '功能设置'"
    width="560px"
    class="feature-settings-dialog"
    :close-on-click-modal="false"
    destroy-on-close
    @update:model-value="emit('update:modelValue', $event)"
  >
    <div v-if="feature && draft" class="feature-settings-content">
      <div class="feature-settings-hero">
        <div class="feature-settings-mark" :style="{ background: feature.accent }">{{ feature.name.slice(0, 1) }}</div>
        <div>
          <b>{{ feature.name }}</b>
          <p>{{ feature.description }}</p>
        </div>
        <el-switch v-model="draft.enabled" active-text="启用" inactive-text="停用" />
      </div>

      <el-form label-position="top" class="feature-settings-form">
        <div class="feature-settings-grid">
          <el-form-item v-for="field in feature.fields.filter((item) => !item.giftOnly)" :key="field.key" :label="field.label">
            <el-input
              v-if="field.type === 'text'"
              :model-value="String(fieldValue(field))"
              :placeholder="field.placeholder"
              @update:model-value="setField(field.key, $event)"
            />
            <el-input-number
              v-else-if="field.type === 'number'"
              :model-value="numberValue(field)"
              :min="field.min"
              :max="field.max"
              :step="field.step ?? 1"
              controls-position="right"
              @update:model-value="setField(field.key, $event)"
            />
            <el-color-picker
              v-else-if="field.type === 'color'"
              :model-value="String(fieldValue(field))"
              show-alpha
              @update:model-value="setField(field.key, $event)"
            />
            <el-switch
              v-else-if="field.type === 'boolean'"
              :model-value="Boolean(fieldValue(field))"
              @update:model-value="setField(field.key, $event)"
            />
            <el-select
              v-else-if="field.type === 'select'"
              :model-value="String(fieldValue(field))"
              @update:model-value="setField(field.key, $event)"
            >
              <el-option v-for="option in field.options ?? []" :key="option.value" :label="option.label" :value="option.value" />
            </el-select>
            <small v-if="field.help" class="feature-field-help">{{ field.help }}</small>
          </el-form-item>
        </div>
      </el-form>

      <section v-if="feature.giftRules || draft.giftRules.length" class="gift-rule-panel">
        <div class="gift-rule-heading">
          <div>
            <b>礼物配置</b>
            <small>每种礼物一行，参数可单独设置；未覆盖时使用玩法默认值</small>
          </div>
          <el-button link type="primary" @click="addGiftRule">＋ 添加礼物</el-button>
        </div>
        <div v-if="draft.giftRules.length" class="gift-rule-list">
          <div v-for="rule in draft.giftRules" :key="rule.id" class="gift-rule-row">
            <div class="gift-rule-main">
              <el-input v-model="rule.giftName" :name="`gift-${rule.id}`" autocomplete="off" spellcheck="false" :aria-label="`礼物名称：${rule.giftName || '未命名'}`" placeholder="礼物名称…" />
              <el-select v-model="rule.action" class="gift-action-select" :aria-label="`礼物动作：${rule.giftName || '未命名'}`">
                <el-option label="增加" value="增加" />
                <el-option label="触发" value="触发" />
                <el-option label="减少" value="减少" />
                <el-option label="播放" value="播放" />
              </el-select>
              <div v-for="field in inlineGiftFields()" :key="field.key" class="gift-rule-inline-value">
                <span>{{ field.label }}</span>
                <el-input-number
                  :model-value="giftRuleNumberValue(rule, field)"
                  :aria-label="`礼物 ${rule.giftName || '未命名'} ${field.label}`"
                  :min="field.min"
                  :max="field.max"
                  :step="field.step ?? 1"
                  controls-position="right"
                  @update:model-value="setGiftRuleField(rule, field, $event)"
                />
                <el-button v-if="hasGiftRuleOverride(rule, field)" link type="info" :aria-label="`恢复礼物 ${rule.giftName || '未命名'} 的${field.label}默认值`" @click="resetGiftRuleField(rule, field)">默认</el-button>
              </div>
              <el-switch v-model="rule.enabled" :aria-label="`启用礼物规则：${rule.giftName || '未命名'}`" />
              <el-button link type="danger" :aria-label="`删除礼物规则：${rule.giftName || '未命名'}`" @click="removeGiftRule(rule.id)">删除</el-button>
              <el-button link type="primary" :aria-expanded="isGiftRuleExpanded(rule.id)" @click="toggleGiftRuleExpanded(rule.id)">
                {{ isGiftRuleExpanded(rule.id) ? '收起设置' : '更多设置' }}
              </el-button>
            </div>
            <div v-if="isGiftRuleExpanded(rule.id) && fieldsForGiftRules().length" class="gift-rule-settings">
              <div v-for="field in fieldsForGiftRules()" :key="field.key" class="gift-rule-setting">
                <div class="gift-rule-setting-label">
                  <span>{{ field.label }}</span>
                  <small>{{ hasGiftRuleOverride(rule, field) ? '本礼物单独设置' : '使用玩法默认值' }}</small>
                </div>
                <el-input
                  v-if="field.type === 'text'"
                  :model-value="String(giftRuleValue(rule, field))"
                  autocomplete="off"
                  :aria-label="`礼物 ${rule.giftName || '未命名'} ${field.label}`"
                  :placeholder="field.placeholder"
                  @update:model-value="setGiftRuleField(rule, field, $event)"
                />
                <el-input-number
                  v-else-if="field.type === 'number'"
                  :model-value="giftRuleNumberValue(rule, field)"
                  :aria-label="`礼物 ${rule.giftName || '未命名'} ${field.label}`"
                  :min="field.min"
                  :max="field.max"
                  :step="field.step ?? 1"
                  controls-position="right"
                  @update:model-value="setGiftRuleField(rule, field, $event)"
                />
                <el-color-picker
                  v-else-if="field.type === 'color'"
                  :model-value="String(giftRuleValue(rule, field))"
                  :aria-label="`礼物 ${rule.giftName || '未命名'} ${field.label}`"
                  show-alpha
                  @update:model-value="setGiftRuleField(rule, field, $event)"
                />
                <el-switch
                  v-else-if="field.type === 'boolean'"
                  :model-value="Boolean(giftRuleValue(rule, field))"
                  :aria-label="`礼物 ${rule.giftName || '未命名'} ${field.label}`"
                  @update:model-value="setGiftRuleField(rule, field, $event)"
                />
                <el-select
                  v-else-if="field.type === 'select'"
                  :model-value="String(giftRuleValue(rule, field))"
                  :aria-label="`礼物 ${rule.giftName || '未命名'} ${field.label}`"
                  @update:model-value="setGiftRuleField(rule, field, $event)"
                >
                  <el-option v-for="option in field.options ?? []" :key="option.value" :label="option.label" :value="option.value" />
                </el-select>
                <el-button v-if="hasGiftRuleOverride(rule, field)" link type="info" :aria-label="`恢复礼物 ${rule.giftName || '未命名'} 的${field.label}默认值`" @click="resetGiftRuleField(rule, field)">恢复默认</el-button>
              </div>
            </div>
          </div>
        </div>
        <el-empty v-else description="还没有礼物规则" :image-size="48" />
      </section>
    </div>
    <el-empty v-else description="请选择一个功能" />

    <template #footer>
      <el-button @click="emit('update:modelValue', false)">取消</el-button>
      <el-button type="primary" :disabled="!draft" @click="save">保存设置</el-button>
    </template>
  </el-dialog>
</template>
