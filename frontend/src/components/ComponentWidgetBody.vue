<script setup lang="ts">
// 组件窗里一个组件的内容。单独拆成子组件的原因：vue3-draggable-resizable
// 在拖动 / 拉伸时每一帧都会重跑外层插槽的渲染函数，插槽内容放在父组件里就会
// 跟着每帧重建、比对一遍（转盘还会因此每帧重画 canvas）。拆成子组件后，
// 拖动期间父组件只是重新创建一个「props 完全相同」的子组件 vnode，Vue 会跳过
// 它的更新，组件内容在拖动 / 拉伸时完全不参与渲染。
import { computed } from 'vue'
import type { FeatureValue, GiftMenu, GiftMenuItem } from '@shared/features'
import { readGiftMenus } from '@shared/features'
import type { OverlayWidgetPayload } from '@shared/types'

export interface ActiveOverlayWidget extends OverlayWidgetPayload { data: Record<string, FeatureValue> }

const props = defineProps<{
  widget: ActiveOverlayWidget
  giftIcon: (name: string) => string
  menuGift: (gift: GiftMenuItem) => void
}>()

const menus = computed<GiftMenu[]>(() => readGiftMenus(props.widget.values).filter((menu) => menu.enabled))

function timerText(seconds: number): string {
  const safe = Math.max(0, Math.floor(seconds))
  const hours = Math.floor(safe / 3600)
  const minutes = Math.floor((safe % 3600) / 60)
  const secs = safe % 60
  return hours ? `${String(hours).padStart(2, '0')}:${String(minutes).padStart(2, '0')}:${String(secs).padStart(2, '0')}` : `${String(minutes).padStart(2, '0')}:${String(secs).padStart(2, '0')}`
}
</script>

<template>
<template v-if="widget.kind === 'acceleration'">
  <strong class="widget-large-number">{{ widget.data.currentCount ?? 0 }}<small> / {{ widget.data.targetCount }}</small></strong>
  <div class="widget-progress"><i :style="{ width: `${widget.data.progress ?? 0}%` }" /></div>
  <p v-if="widget.data.preview">组件已启用 · 收到绑定礼物后开始处理</p>
  <p v-else>排队 {{ widget.data.pendingCount ?? 0 }} · {{ widget.featureId === 'speed-iba' ? `已处理批次 ${widget.data.batchCount ?? 0} · 每批 ${widget.values.batchSize}` : `曲线：${widget.values.curve}` }}</p>
</template>
<template v-else-if="widget.kind === 'health'">
  <strong class="widget-large-number">{{ widget.data.value }}<small> / {{ widget.data.maxValue }}</small></strong>
  <div class="health-track"><i :style="{ width: `${Math.max(0, Math.min(100, Number(widget.data.value) / Math.max(1, Number(widget.data.maxValue)) * 100))}%` }" /></div>
</template>
<template v-else-if="widget.kind === 'timer'">
  <div v-if="widget.values.style === 'ring'" class="timer-ring" :style="{ '--timer-progress': `${Math.max(0, Number(widget.data.remainingSeconds) / Math.max(1, Number(widget.data.totalSeconds)) * 100)}%` }"><span>{{ timerText(Number(widget.data.remainingSeconds ?? 0)) }}</span></div>
  <strong v-else class="timer-digital">{{ timerText(Number(widget.data.remainingSeconds ?? 0)) }}</strong>
  <p>{{ widget.data.preview ? '组件已启用 · 等待礼物调整倒计时' : widget.data.completed ? '倒计时结束' : '倒计时进行中' }}</p>
</template>
<template v-else-if="widget.kind === 'menu'">
  <div class="gift-menu-stage">
    <div v-for="menu in menus" :key="menu.id" class="gift-menu-block" :style="{ opacity: menu.opacity }">
      <b v-if="menu.showLeftTitle" class="gift-menu-title">{{ menu.title }}</b>
      <div class="gift-menu-items">
        <button v-for="gift in menu.gifts" :key="gift.id" type="button" class="gift-menu-item" :aria-label="`发送礼物 ${gift.title || gift.giftName}`" @click="menuGift(gift)">
          <img v-if="giftIcon(gift.giftName)" :src="giftIcon(gift.giftName)" alt="" :style="{ width: `${menu.imageSize}px`, height: `${menu.imageSize}px` }">
          <span v-else class="gift-menu-mark" :style="{ width: `${menu.imageSize}px`, height: `${menu.imageSize}px`, fontSize: `${Math.max(10, Math.round(menu.imageSize * 0.5))}px` }">{{ (gift.title || gift.giftName || '礼').slice(0, 1) }}</span>
          <span class="gift-menu-label" :style="{ fontSize: `${menu.fontSize}px`, color: menu.fontColor }">{{ gift.title || gift.giftName }}</span>
        </button>
      </div>
    </div>
  </div>
  <small>{{ widget.data.preview ? '组件已启用 · 点击菜单里的礼物即可触发玩法' : '礼物菜单' }}</small>
</template>
<template v-else-if="widget.kind === 'sticker'">
  <div class="sticker-value"><img v-if="widget.data.imagePath" :src="String(widget.data.imagePath)" :alt="String(widget.data.sticker)" @error="($event.target as HTMLImageElement).style.display = 'none'"><span>{{ widget.data.sticker }}</span></div>
  <small>{{ widget.data.preview ? '组件已启用 · 收到礼物后展示贴纸' : '礼物咖 · 贴纸展示' }}</small>
</template>
<template v-else-if="widget.kind === 'reply'">
  <div class="reply-bubble">{{ widget.data.text }}</div>
  <small>{{ widget.data.preview ? '组件已启用 · 匹配弹幕后显示回复' : '本地回复预览；未向直播平台发送' }}</small>
</template>
<template v-else-if="widget.kind === 'notice'">
  <div class="reply-bubble">{{ widget.data.text }}</div>
  <small>{{ widget.data.preview ? '等待礼物触发' : '玩法效果状态' }}</small>
</template>
<template v-else-if="widget.kind === 'countdown'">
  <div
    class="countdown-panel"
    :style="{
      background: `linear-gradient(160deg, ${widget.values.bgColor1 ?? '#1a1a2e'}, ${widget.values.bgColor2 ?? '#16213e'})`,
      backgroundImage: widget.values.bgImage ? `url(${String(widget.values.bgImage)})` : undefined,
      backgroundSize: 'cover',
      backgroundPosition: 'center',
      borderColor: String(widget.data.completed ? (widget.values.closeColor ?? '#ff4650') : (widget.values.openColor ?? '#00ff88')),
    }"
  >
    <strong class="countdown-digital" :style="{ color: String(widget.values.countdownColor ?? '#ffffff') }">{{ timerText(Number(widget.data.remainingSeconds ?? 0)) }}</strong>
    <div class="countdown-track"><i :style="{ width: `${Math.max(0, Math.min(100, Number(widget.data.remainingSeconds) / Math.max(1, Number(widget.data.totalSeconds)) * 100))}%`, background: String(widget.values.openColor ?? '#00ff88') }" /></div>
  </div>
  <small v-if="widget.values.tempEnabled">温度 {{ widget.values.tempMin }} – {{ widget.values.tempMax }}</small>
  <small>{{ widget.data.preview ? '组件已启用 · 收到礼物后开始倒计时' : widget.data.completed ? '时间到' : '倒计时进行中' }}</small>
</template>
<template v-else-if="widget.kind === 'trash'">
  <div class="trash-pile">
    <span v-for="index in Math.min(60, Number(widget.data.items ?? 0))" :key="index" class="trash-item">
      <img v-if="widget.values.imagePath" :src="String(widget.values.imagePath)" alt="" @error="($event.target as HTMLImageElement).style.display = 'none'">
    </span>
  </div>
  <div class="trash-bin">
    <img v-if="widget.values.binPath" :src="String(widget.values.binPath)" alt="垃圾桶" @error="($event.target as HTMLImageElement).style.display = 'none'">
    <strong v-else>垃圾桶 {{ widget.data.items ?? 0 }}</strong>
  </div>
  <small>{{ widget.data.preview ? '组件已启用 · 收到礼物后掉落垃圾' : `已掉落 ${widget.data.items ?? 0} 个` }}</small>
</template>
</template>
