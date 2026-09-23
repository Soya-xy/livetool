<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { api } from '../services/api'

interface VideoItem { id: string; path: string; lane: number; loop: boolean; paused: boolean }
interface DropItem { id: string; image: string; left: number; size: number; top: number; velocity: number; gravity: number; bounce: number; rotate: number; expiresAt: number }

const videos = ref<VideoItem[]>([])
const drops = ref<DropItem[]>([])
const controlsVisible = ref(false)
const videoElements = new Map<string, HTMLVideoElement>()
const videoTimers = new Map<string, ReturnType<typeof setTimeout>>()
let remove: (() => void) | undefined
let animationFrame = 0
let previousFrame = 0

onMounted(() => {
  remove = api.overlay.onMessage((message) => {
    if (message.target !== 'green') return
    if (message.type === 'play-video') playVideo(message.payload as { path: string; lane?: number; durationMs?: number; loop?: boolean })
    if (message.type === 'drop') drop(message.payload as { image: string; count?: number; gravity?: number; bounce?: number; durationMs?: number; maxVisible?: number })
  })
  window.addEventListener('keydown', onKeyDown)
})

onBeforeUnmount(() => {
  remove?.()
  window.removeEventListener('keydown', onKeyDown)
  cancelAnimationFrame(animationFrame)
  for (const timer of videoTimers.values()) clearTimeout(timer)
})

function onKeyDown(event: KeyboardEvent): void {
  if (event.key === 'Tab') {
    event.preventDefault()
    controlsVisible.value = !controlsVisible.value
  }
}

function playVideo(payload: { path: string; lane?: number; durationMs?: number; loop?: boolean }): void {
  const id = crypto.randomUUID()
  videos.value.push({ id, path: payload.path, lane: payload.lane ?? 1, loop: Boolean(payload.loop), paused: false })
  if (!payload.loop) videoTimers.set(id, setTimeout(() => stopVideo(id), Math.max(100, payload.durationMs ?? 8000)))
}

function setVideoElement(id: string, element: Element | null): void {
  if (element instanceof HTMLVideoElement) videoElements.set(id, element)
  else videoElements.delete(id)
}

function toggleVideo(id: string): void {
  const video = videoElements.get(id)
  if (!video) return
  const item = videos.value.find((entry) => entry.id === id)
  if (!item) return
  if (video.paused) { void video.play(); item.paused = false }
  else { video.pause(); item.paused = true }
}

function stopVideo(id: string): void {
  const timer = videoTimers.get(id)
  if (timer) clearTimeout(timer)
  videoTimers.delete(id)
  videoElements.get(id)?.pause()
  videoElements.delete(id)
  videos.value = videos.value.filter((item) => item.id !== id)
}

function drop(payload: { image: string; count?: number; gravity?: number; bounce?: number; durationMs?: number; maxVisible?: number }): void {
  const count = Math.min(Math.max(1, payload.count ?? 1), 100)
  const gravity = Math.max(0, Math.min(payload.gravity ?? 1800, 5000))
  const bounce = Math.max(0, Math.min(payload.bounce ?? 0, 1))
  const duration = Math.max(200, payload.durationMs ?? 3500)
  const created: DropItem[] = Array.from({ length: count }, () => ({
    id: crypto.randomUUID(), image: payload.image, left: 4 + Math.random() * 92,
    size: 40 + Math.random() * 76, top: -140, velocity: 0, gravity, bounce,
    rotate: Math.random() * 720 - 360, expiresAt: performance.now() + duration,
  }))
  const maxVisible = Math.max(1, Math.min(200, payload.maxVisible ?? 200))
  drops.value = [...drops.value, ...created].slice(-maxVisible)
  if (!animationFrame) animationFrame = requestAnimationFrame(animateDrops)
}

function animateDrops(timestamp: number): void {
  const seconds = previousFrame ? Math.min((timestamp - previousFrame) / 1000, 0.05) : 0
  previousFrame = timestamp
  const floor = window.innerHeight
  const now = performance.now()
  drops.value = drops.value.filter((item) => item.expiresAt > now)
  for (const item of drops.value) {
    item.velocity += item.gravity * seconds
    item.top += item.velocity * seconds
    const ground = floor - item.size
    if (item.top >= ground) {
      item.top = ground
      if (item.bounce > 0 && Math.abs(item.velocity * item.bounce) > 85) item.velocity = -item.velocity * item.bounce
      else item.velocity = 0
    }
  }
  if (drops.value.length) animationFrame = requestAnimationFrame(animateDrops)
  else { animationFrame = 0; previousFrame = 0 }
}
</script>

<template>
  <div class="green-overlay">
    <div class="green-status-bar">
      <span>ylm · 绿幕输出</span>
      <span>{{ videos.length }} 个视频 · {{ drops.length }} 个落物</span>
      <button type="button" @click="controlsVisible = !controlsVisible">视频管理（Tab）</button>
    </div>
    <div v-if="controlsVisible" class="green-video-panel">
      <strong>视频列表</strong>
      <p v-if="!videos.length">暂无播放视频。可通过玩法规则或绿幕功能设置播放视频。</p>
      <div v-for="(item, index) in videos" :key="item.id" class="green-video-row">
        <span>通道 {{ item.lane }} · {{ item.path.split(/[\\/]/).pop() }}</span>
        <button type="button" @click="toggleVideo(item.id)">{{ item.paused ? '继续' : '暂停' }}</button>
        <button type="button" @click="stopVideo(item.id)">停止</button>
      </div>
    </div>
    <div v-for="item in videos" :key="item.id" class="overlay-video" :style="{ zIndex: item.lane }">
      <video :ref="(element) => setVideoElement(item.id, element as Element | null)" :src="item.path" :autoplay="!item.paused" muted :loop="item.loop" playsinline @play="item.paused = false" @pause="item.paused = true" @ended="stopVideo(item.id)" />
    </div>
    <div v-for="item in drops" :key="item.id" class="falling-item" :style="{ left: `${item.left}%`, top: `${item.top}px`, '--size': `${item.size}px`, '--rotate': `${item.rotate}deg` }">
      <img :src="item.image" @error="($event.target as HTMLImageElement).style.display = 'none'"><span>✦</span>
    </div>
  </div>
</template>
