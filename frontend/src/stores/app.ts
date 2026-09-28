import { computed, ref } from 'vue'
import { defineStore } from 'pinia'
import type { AppSettings, ConnectorStatus, DanmakuRecord, LogEntry, OperationResult, Rule, RulesRuntimeState } from '@shared/types'
import { api } from '@/services/api'

export const useAppStore = defineStore('app', () => {
  const rules = ref<Rule[]>([])
  const liveRecords = ref<DanmakuRecord[]>([])
  const logs = ref<LogEntry[]>([])
  const status = ref<ConnectorStatus>({ platform: 'simulator', state: 'disconnected', mode: 'simulator', reconnects: 0, dropped: 0 })
  const settings = ref<AppSettings | null>(null)
  const runtime = ref<RulesRuntimeState>({ enabled: true, paused: false, debugKeys: false, openKey: 'Ctrl + 0', closeKey: 'Ctrl + F12' })
  const initialized = ref(false)
  const unread = computed(() => liveRecords.value.filter((record) => record.actionResult !== 'none').length)

  let removeRecordListener: (() => void) | undefined
  let removeStatusListener: (() => void) | undefined
  let removeLogListener: (() => void) | undefined
  let removeRulesStateListener: (() => void) | undefined

  async function init(): Promise<void> {
    if (initialized.value) return
    rules.value = await api.rules.list()
    status.value = await api.conn.status()
    settings.value = await api.diagnostics.settings()
    runtime.value = await api.rules.runtimeState()
    logs.value = await api.diagnostics.logs(100)
    removeRecordListener = api.danmaku.onAppend((record) => {
      const index = liveRecords.value.findIndex((item) => item.id === record.id)
      if (index >= 0) liveRecords.value.splice(index, 1, record)
      else liveRecords.value.unshift(record)
      liveRecords.value = liveRecords.value.slice(0, 500)
    })
    removeStatusListener = api.conn.onStatus((next) => { status.value = next })
    removeLogListener = api.diagnostics.onLog((entry) => { logs.value.unshift(entry); logs.value = logs.value.slice(0, 100) })
    removeRulesStateListener = api.rules.onState((next) => { runtime.value = next })
    initialized.value = true
  }

  async function refreshRules(): Promise<void> { rules.value = await api.rules.list() }
  async function saveRule(rule: Rule): Promise<void> { await api.rules.save(rule); await refreshRules() }
  async function removeRule(id: string): Promise<void> { await api.rules.remove(id); await refreshRules() }
  async function cloneRule(id: string, count = 1): Promise<void> { await api.rules.clone(id, count); await refreshRules() }
  async function setRulePinned(id: string, pinned: boolean): Promise<void> { await api.rules.setPinned(id, pinned); await refreshRules() }
  async function triggerRule(id: string): Promise<OperationResult> { return api.rules.trigger(id) }
  async function setRulesEnabled(enabled: boolean): Promise<void> { runtime.value = await api.rules.setEnabled(enabled) }
  async function setRulesPaused(paused: boolean): Promise<void> { runtime.value = await api.rules.setPaused(paused) }
  async function clearRules(): Promise<void> { await api.rules.clear(); rules.value = [] }
  async function connect(platform: AppSettings['platform'], roomId: string): Promise<void> { status.value = await api.conn.connect(platform, roomId) }
  async function disconnect(): Promise<void> { status.value = await api.conn.disconnect() }
  async function simulate(payload: Parameters<typeof api.conn.simulate>[0]): Promise<void> { await api.conn.simulate(payload) }
  async function saveSettings(patch: Partial<AppSettings>): Promise<void> { settings.value = await api.diagnostics.saveSettings(patch) }
  function dispose(): void { removeRecordListener?.(); removeStatusListener?.(); removeLogListener?.(); removeRulesStateListener?.() }

  return { rules, liveRecords, logs, status, settings, runtime, initialized, unread, init, refreshRules, saveRule, removeRule, cloneRule, setRulePinned, triggerRule, setRulesEnabled, setRulesPaused, clearRules, connect, disconnect, simulate, saveSettings, dispose }
})
