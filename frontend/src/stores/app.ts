import { computed, ref } from 'vue'
import { defineStore } from 'pinia'
import type { AppSettings, ConnectorStatus, DanmakuRecord, LogEntry, Rule } from '@shared/types'
import { api } from '@/services/api'

export const useAppStore = defineStore('app', () => {
  const rules = ref<Rule[]>([])
  const liveRecords = ref<DanmakuRecord[]>([])
  const logs = ref<LogEntry[]>([])
  const status = ref<ConnectorStatus>({ platform: 'simulator', state: 'disconnected', mode: 'simulator', reconnects: 0, dropped: 0 })
  const settings = ref<AppSettings | null>(null)
  const initialized = ref(false)
  const unread = computed(() => liveRecords.value.filter((record) => record.actionResult !== 'none').length)

  let removeRecordListener: (() => void) | undefined
  let removeStatusListener: (() => void) | undefined
  let removeLogListener: (() => void) | undefined

  async function init(): Promise<void> {
    if (initialized.value) return
    rules.value = await api.rules.list()
    status.value = await api.conn.status()
    settings.value = await api.diagnostics.settings()
    logs.value = await api.diagnostics.logs(100)
    removeRecordListener = api.danmaku.onAppend((record) => {
      const index = liveRecords.value.findIndex((item) => item.id === record.id)
      if (index >= 0) liveRecords.value.splice(index, 1, record)
      else liveRecords.value.unshift(record)
      liveRecords.value = liveRecords.value.slice(0, 500)
    })
    removeStatusListener = api.conn.onStatus((next) => { status.value = next })
    removeLogListener = api.diagnostics.onLog((entry) => { logs.value.unshift(entry); logs.value = logs.value.slice(0, 100) })
    initialized.value = true
  }

  async function refreshRules(): Promise<void> { rules.value = await api.rules.list() }
  async function saveRule(rule: Rule): Promise<void> { await api.rules.save(rule); await refreshRules() }
  async function removeRule(id: string): Promise<void> { await api.rules.remove(id); await refreshRules() }
  async function cloneRule(id: string): Promise<void> { await api.rules.clone(id); await refreshRules() }
  async function clearRules(): Promise<void> { await api.rules.clear(); rules.value = [] }
  async function connect(platform: AppSettings['platform'], roomId: string): Promise<void> { status.value = await api.conn.connect(platform, roomId) }
  async function simulate(payload: Parameters<typeof api.conn.simulate>[0]): Promise<void> { await api.conn.simulate(payload) }
  async function saveSettings(patch: Partial<AppSettings>): Promise<void> { settings.value = await api.diagnostics.saveSettings(patch) }
  function dispose(): void { removeRecordListener?.(); removeStatusListener?.(); removeLogListener?.() }

  return { rules, liveRecords, logs, status, settings, initialized, unread, init, refreshRules, saveRule, removeRule, cloneRule, clearRules, connect, simulate, saveSettings, dispose }
})
