<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import type { FeatureConfig, FeatureDefinition, FeatureField, FeatureGiftRule, FeatureValue, GiftMenu } from '@shared/features'
import { createDefaultFeatureSettings, createGiftMenu, readGiftMenus } from '@shared/features'
import type { AssetKind } from '@shared/types'
import { api } from '../services/api'
import AssetPathSelect from './AssetPathSelect.vue'

const props = defineProps<{
  modelValue: boolean
  feature: FeatureDefinition | null
  config: FeatureConfig | null
}>()

const emit = defineEmits<{
  'update:modelValue': [value: boolean]
  save: [value: FeatureConfig]
}>()

// 素材类字段（imagePath / videoPath / audioPath 等）改用素材选择器。
function assetKindFor(key: string): AssetKind | null {
  if (key === 'lockMediaPath') return null
  if (key.endsWith('videoPath') || key.endsWith('VideoPath')) return 'video'
  if (key.endsWith('audioPath') || key.endsWith('AudioPath')) return 'audio'
  if (key.endsWith('imagePath') || key.endsWith('ImagePath')) return 'image'
  return null
}

const draft = ref<FeatureConfig | null>(null)
const expandedGiftRules = ref<string[]>([])
const selectingLockMedia = ref(false)

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

function currentLockMediaPath(): string {
  const value = draft.value?.values.lockMediaPath
  return typeof value === 'string' ? value : ''
}

function lockMediaFilename(path: string): string {
  return path.split(/[\\/]/).pop() || path
}

function isLockMediaVideo(path: string): boolean {
  return /\.(mp4|webm|mov|m4v)$/i.test(path)
}

async function chooseLockMedia(): Promise<void> {
  selectingLockMedia.value = true
  try {
    const path = await api.features.selectLockMedia()
    if (!path) return
    setField('lockMediaPath', path)
    ElMessage.success('素材已添加，保存设置后生效')
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : String(error))
  } finally {
    selectingLockMedia.value = false
  }
}

function removeLockMedia(): void {
  setField('lockMediaPath', '')
}

function fieldsForGiftRules(): FeatureField[] {
  return props.feature?.fields.filter((field) => !field.giftOnly) ?? []
}

function fieldsForGeneralSettings(): FeatureField[] {
  return props.feature?.fields.filter((field) => !field.giftOnly && !(props.feature?.id === 'screen-lock' && field.key === 'lockMediaPath')) ?? []
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

// ── 礼物菜单（礼物咖）：菜单/样式/礼物列表的编辑逻辑 ──
const giftMenus = computed<GiftMenu[]>(() => (draft.value ? readGiftMenus(draft.value.values) : []))
const collapsedMenus = ref<string[]>([])

function writeGiftMenus(menus: GiftMenu[]): void {
  if (!draft.value) return
  draft.value.values.menus = menus
}

function isMenuCollapsed(id: string): boolean {
  return collapsedMenus.value.includes(id)
}

function toggleMenuCollapsed(id: string): void {
  collapsedMenus.value = isMenuCollapsed(id) ? collapsedMenus.value.filter((item) => item !== id) : [...collapsedMenus.value, id]
}

function addGiftMenu(): void {
  writeGiftMenus([...giftMenus.value, createGiftMenu(giftMenus.value.length + 1)])
}

function removeGiftMenu(id: string): void {
  writeGiftMenus(giftMenus.value.filter((menu) => menu.id !== id))
}

function addMenuGift(menu: GiftMenu): void {
  menu.gifts.push({ id: `gift-${crypto.randomUUID()}`, title: '', giftName: '' })
}

function removeMenuGift(menu: GiftMenu, id: string): void {
  menu.gifts = menu.gifts.filter((gift) => gift.id !== id)
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
  if (props.feature?.id === 'gift-pool') {
    const menus = readGiftMenus(draft.value.values)
    for (const menu of menus) {
      if (!menu.gifts.length) {
        ElMessage.warning(`「${menu.title}」还没有礼物，请先添加礼物`)
        return
      }
      for (const gift of menu.gifts) {
        if (!gift.giftName.trim()) {
          ElMessage.warning(`「${menu.title}」里有礼物没填礼物名称`)
          return
        }
      }
    }
  }
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
    width="min(1080px, calc(100vw - 40px))"
    top="3vh"
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
          <el-form-item v-for="field in fieldsForGeneralSettings()" :key="field.key" :label="field.label">
            <AssetPathSelect
              v-if="assetKindFor(field.key)"
              :model-value="String(fieldValue(field))"
              :kind="assetKindFor(field.key)!"
              @update:model-value="setField(field.key, $event)"
            />
            <el-input
              v-else-if="field.type === 'text'"
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

      <section v-if="feature.id === 'screen-lock'" class="lock-style-panel" aria-label="锁屏背景">
        <div class="lock-style-heading">
          <div>
            <b>锁屏背景（可选）</b>
            <small>图片或视频会铺在 3D 锁链特效后面，锁链、提示和剩余次数仍会显示。</small>
          </div>
          <span class="lock-style-status" :class="{ selected: currentLockMediaPath() }">
            {{ currentLockMediaPath() ? (isLockMediaVideo(currentLockMediaPath()) ? '已选视频' : '已选图片') : '仅显示 3D 锁链' }}
          </span>
        </div>
        <div class="lock-style-row">
          <div class="lock-style-file">
            <span class="lock-style-file-mark">{{ currentLockMediaPath() ? (isLockMediaVideo(currentLockMediaPath()) ? '视频' : '图片') : '锁链' }}</span>
            <div>
              <b>{{ currentLockMediaPath() ? lockMediaFilename(currentLockMediaPath()) : '未选择背景素材' }}</b>
              <small>{{ currentLockMediaPath() ? '锁定时自动铺满窗口' : '锁定时只显示 3D 锁链特效' }}</small>
            </div>
          </div>
          <div class="lock-style-actions">
            <el-button type="primary" plain :loading="selectingLockMedia" @click="chooseLockMedia">
              {{ currentLockMediaPath() ? '更换素材' : '选择图片或视频' }}
            </el-button>
            <el-button v-if="currentLockMediaPath()" link type="danger" @click="removeLockMedia">移除</el-button>
          </div>
        </div>
        <small class="lock-style-help">支持 PNG、JPG、WebP、GIF、BMP 图片及 MP4、WebM、MOV、M4V 视频；留空时锁定时只显示 3D 锁链特效。</small>
      </section>

      <section v-if="feature.id === 'gift-pool'" class="gift-menu-panel" aria-label="礼物菜单配置">
        <div class="gift-menu-heading">
          <div>
            <b>礼物菜单配置</b>
            <small>组件窗里显示这些菜单；点一下礼物就等于收到该礼物，规则照常触发</small>
          </div>
          <el-button link type="primary" @click="addGiftMenu">＋ 添加菜单</el-button>
        </div>
        <div v-for="(menu, index) in giftMenus" :key="menu.id" class="gift-menu-card">
          <div class="gift-menu-card-head">
            <b>{{ menu.title || `礼物菜单${index + 1}` }}</b>
            <el-switch v-model="menu.enabled" :aria-label="`启用${menu.title || `礼物菜单${index + 1}`}`" />
            <el-button link type="danger" :aria-label="`删除${menu.title || `礼物菜单${index + 1}`}`" @click="removeGiftMenu(menu.id)">删除</el-button>
            <el-button link type="primary" :aria-expanded="!isMenuCollapsed(menu.id)" @click="toggleMenuCollapsed(menu.id)">{{ isMenuCollapsed(menu.id) ? '展开' : '收起' }} ⌄</el-button>
          </div>
          <div v-show="!isMenuCollapsed(menu.id)" class="gift-menu-card-body">
            <div class="gift-menu-row">
              <span>左边标题</span>
              <el-switch v-model="menu.showLeftTitle" :aria-label="`显示左边标题：${menu.title}`" />
            </div>
            <el-form-item label="菜单标题">
              <el-input v-model="menu.title" autocomplete="off" placeholder="礼物菜单1" />
            </el-form-item>
            <el-form-item label="透明度">
              <el-slider v-model="menu.opacity" :min="0.1" :max="1" :step="0.05" show-input />
            </el-form-item>
            <div class="gift-menu-grid">
              <el-form-item label="字体大小">
                <el-input-number v-model="menu.fontSize" :min="10" :max="48" controls-position="right" />
              </el-form-item>
              <el-form-item label="字体颜色">
                <el-color-picker v-model="menu.fontColor" />
              </el-form-item>
              <el-form-item label="图片大小">
                <el-input-number v-model="menu.imageSize" :min="16" :max="128" controls-position="right" />
              </el-form-item>
            </div>
            <div class="gift-menu-gifts">
              <div class="gift-menu-gifts-head">
                <b>礼物列表</b>
                <el-button type="primary" size="small" @click="addMenuGift(menu)">添加礼物</el-button>
              </div>
              <div v-for="gift in menu.gifts" :key="gift.id" class="gift-menu-gift">
                <el-form-item label="标题">
                  <el-input v-model="gift.title" autocomplete="off" placeholder="菜单上显示的名字" />
                </el-form-item>
                <el-form-item label="礼物名称">
                  <el-input v-model="gift.giftName" autocomplete="off" placeholder="触发规则用的礼物名称" />
                </el-form-item>
                <el-button link type="danger" :aria-label="`删除礼物：${gift.title || '未命名'}`" @click="removeMenuGift(menu, gift.id)">删除</el-button>
              </div>
              <p v-if="!menu.gifts.length" class="gift-menu-empty">还没有礼物，点「添加礼物」加一条。</p>
            </div>
          </div>
        </div>
        <p v-if="!giftMenus.length" class="gift-menu-empty">还没有菜单，点「添加菜单」加一个。</p>
      </section>

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
                <el-option v-if="feature.id !== 'screen-lock'" label="触发" value="触发" />
                <el-option label="减少" value="减少" />
                <el-option v-if="feature.id !== 'screen-lock'" label="播放" value="播放" />
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
                <small v-if="field.help" class="gift-rule-inline-help">{{ field.help }}</small>
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
