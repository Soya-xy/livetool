<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { api } from '../services/api'
const audio = ref<HTMLAudioElement>()
const queue = ref<Array<{ path: string; volume?: number; interrupt?: boolean; loop?: boolean }>>([])
let remove: (() => void) | undefined
onMounted(() => { remove = api.audio.onMessage(handle); void api.audio.ready() })
onBeforeUnmount(() => remove?.())
function handle(event: { type: string; payload: { path?: string; volume?: number; interrupt?: boolean; loop?: boolean } }): void {
  if (!audio.value) return
  if (event.type === 'stop-audio') { queue.value = []; audio.value.pause(); audio.value.currentTime = 0; return }
  if (event.type !== 'play-audio' || !event.payload.path) return
  const payload = { path: event.payload.path, volume: event.payload.volume, interrupt: event.payload.interrupt, loop: event.payload.loop }
  if (!payload.interrupt && !audio.value.paused && !audio.value.ended) { queue.value.push(payload); return }
  queue.value = []
  play(payload)
}
function play(payload: { path: string; volume?: number; loop?: boolean }): void {
  if (!audio.value) return
  audio.value.src = payload.path
  audio.value.volume = payload.volume ?? 0.8
  audio.value.loop = Boolean(payload.loop)
  void audio.value.play()
}
function playNext(): void { const next = queue.value.shift(); if (next?.path) play(next) }
</script>

<template><audio ref="audio" preload="auto" @ended="playNext" /></template>
