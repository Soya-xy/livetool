<script setup lang="ts">
import { computed, nextTick, onMounted, ref, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Download, Filter, Refresh, Search, VideoPause, VideoPlay } from '@element-plus/icons-vue'
import type { ActionResult, DanmakuFilter, DanmakuRecord, EventKind } from '@shared/types'
import { api } from '../services/api'
import { useAppStore } from '../stores/app'

const store = useAppStore()
const tab = ref('live')
const paused = ref(false)
const autoScroll = ref(true)
const liveKeyword = ref('')
const liveKind = ref<EventKind | ''>('')
const liveSource = ref('')
const history = ref<DanmakuRecord[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = 100
const filters = ref<DanmakuFilter>({ kind: '', actionResult: '', matched: '', limit: pageSize, offset: 0 })
const historyRange = ref<[Date, Date] | null>(null)
const clearRange = ref<'all' | 'day' | 'filter'>('all')
const liveList = ref<HTMLElement>()

const liveRecords = computed(() => store.liveRecords.filter((record) => (
  (!liveKeyword.value || `${record.text ?? ''}${record.giftName ?? ''}${record.userName ?? ''}`.toLocaleLowerCase().includes(liveKeyword.value.toLocaleLowerCase()))
  && (!liveKind.value || record.kind === liveKind.value)
  && (!liveSource.value || record.source === liveSource.value)
)))

onMounted(() => { void store.init(); void loadHistory() })
watch(() => store.liveRecords.length, async () => {
  if (!autoScroll.value || paused.value) return
  await nextTick()
  liveList.value?.scrollTo({ top: 0, behavior: 'smooth' })
})

async function loadHistory(): Promise<void> {
  filters.value.offset = (page.value - 1) * pageSize
  filters.value.limit = pageSize
  history.value = await api.danmaku.query(filters.value)
  total.value = await api.danmaku.count(filters.value)
}

function applyHistoryFilters(): void {
  filters.value.from = historyRange.value?.[0]?.getTime()
  filters.value.to = historyRange.value?.[1]?.getTime()
  page.value = 1
  void loadHistory()
}

async function exportData(format: 'json' | 'csv' | 'txt'): Promise<void> {
  const result = await api.danmaku.export(filters.value, format)
  result.path ? ElMessage.success(`已导出到 ${result.path}`) : ElMessage.info('已生成导出内容')
}

async function clearHistory(): Promise<void> {
  if (clearRange.value === 'filter' && filters.value.from === undefined && filters.value.to === undefined) {
    ElMessage.warning('请先选择时间范围并查询')
    return
  }
  const range = clearRange.value === 'all'
    ? undefined
    : clearRange.value === 'day'
      ? { from: Date.now() - 24 * 60 * 60 * 1000 }
      : { from: filters.value.from, to: filters.value.to }
  await ElMessageBox.confirm(
    clearRange.value === 'all' ? '将清空全部弹幕记录，配置和规则不会受影响。' : '将清理选定时间范围内的记录。',
    '清理弹幕记录',
    { type: 'warning' },
  )
  const count = await api.danmaku.clear(range)
  ElMessage.success(`已清理 ${count} 条记录`)
  await loadHistory()
}

function typeLabel(kind: string): string {
  return ({ gift: '礼物', chat: '弹幕', like: '点赞', follow: '关注', enter: '进场', system: '系统' }[kind] ?? kind)
}

function recordContent(record: DanmakuRecord): string {
  if (record.kind === 'gift') return `${record.giftName ?? '未知礼物'} × ${record.giftCount ?? 1}`
  if (record.kind === 'like') return `连击 ${record.repeatCount ?? 1}`
  return record.text ?? '—'
}

function resultType(result: ActionResult): 'success' | 'warning' | 'danger' | 'info' {
  return result === 'ok' ? 'success' : result === 'failed' ? 'danger' : result === 'skipped' ? 'warning' : 'info'
}
</script>

<template>
  <div class="page-stack diary-page">
    <div class="page-heading">
      <div>
        <div class="section-kicker">EVENT STREAM / 02</div>
        <h1>弹幕日记</h1>
        <p>实时抓取、历史检索与规则结果都在这里留痕。</p>
      </div>
      <div class="heading-metric live-metric"><span>{{ store.liveRecords.length }}</span><small>当前缓存</small></div>
    </div>

    <div class="diary-card data-card">
      <el-tabs v-model="tab" class="diary-tabs" @tab-change="tab === 'history' && loadHistory()">
        <el-tab-pane name="live">
          <template #label><span class="tab-label"><i class="live-pulse" />实时弹幕</span></template>
          <div class="diary-toolbar">
            <div class="filter-line">
              <el-input v-model="liveKeyword" clearable placeholder="搜索昵称 / 内容" class="diary-search"><template #prefix><el-icon><Search /></el-icon></template></el-input>
              <el-select v-model="liveKind" clearable placeholder="事件类型" class="filter-select"><el-option label="礼物" value="gift" /><el-option label="弹幕" value="chat" /><el-option label="点赞" value="like" /><el-option label="关注" value="follow" /></el-select>
              <el-select v-model="liveSource" clearable placeholder="平台" class="filter-select"><el-option label="模拟器" value="simulator" /><el-option label="抖音" value="douyin" /><el-option label="B站" value="bilibili" /></el-select>
            </div>
            <div class="filter-actions">
              <el-switch v-model="autoScroll" active-text="自动滚动" />
              <el-button size="small" @click="paused = !paused"><el-icon><VideoPlay v-if="paused" /><VideoPause v-else /></el-icon>{{ paused ? '继续' : '暂停' }}</el-button>
              <el-button size="small" @click="store.liveRecords.splice(0)"><el-icon><Refresh /></el-icon>清空显示</el-button>
            </div>
          </div>
          <div ref="liveList" class="live-list" :class="{ paused }">
            <div v-for="record in liveRecords" :key="`${record.id}-${record.actionResult}`" class="live-row">
              <time>{{ new Date(record.ts).toLocaleTimeString('zh-CN', { hour12: false }) }}</time>
              <el-tag size="small" effect="plain" :type="record.kind === 'gift' ? 'warning' : record.kind === 'chat' ? 'primary' : 'success'">{{ typeLabel(record.kind) }}</el-tag>
              <b class="live-user">{{ record.userName || '匿名用户' }}</b>
              <span class="live-content">{{ recordContent(record) }}</span>
              <span v-if="record.matchedRuleName" class="matched-rule">→ {{ record.matchedRuleName }}</span>
              <el-tag v-if="record.actionResult !== 'none'" size="small" :type="resultType(record.actionResult)">{{ record.actionResult === 'ok' ? '成功' : record.actionResult === 'failed' ? '失败' : '跳过' }}</el-tag>
            </div>
            <div v-if="!liveRecords.length" class="diary-empty"><span>⌁</span><p>等待事件进入队列</p><small>可以在控制中心使用“模拟事件”开始自测</small></div>
          </div>
        </el-tab-pane>

        <el-tab-pane name="history">
          <template #label><span class="tab-label"><el-icon><Filter /></el-icon>历史记录</span></template>
          <div class="history-filters">
            <el-date-picker v-model="historyRange" type="datetimerange" range-separator="至" start-placeholder="开始时间" end-placeholder="结束时间" clearable />
            <el-input v-model="filters.userName" clearable placeholder="昵称" />
            <el-input v-model="filters.keyword" clearable placeholder="关键词" />
            <el-select v-model="filters.source" clearable placeholder="平台"><el-option label="模拟器" value="simulator" /><el-option label="抖音" value="douyin" /><el-option label="B站" value="bilibili" /></el-select>
            <el-select v-model="filters.kind" clearable placeholder="类型"><el-option label="礼物" value="gift" /><el-option label="弹幕" value="chat" /><el-option label="点赞" value="like" /><el-option label="关注" value="follow" /></el-select>
            <el-select v-model="filters.matched" clearable placeholder="命中规则"><el-option label="已命中" value="yes" /><el-option label="未命中" value="no" /></el-select>
            <el-button type="primary" @click="applyHistoryFilters"><el-icon><Search /></el-icon>查询</el-button>
          </div>
          <div class="history-actions">
            <span>共 {{ total }} 条记录 · 每页 {{ pageSize }} 条</span>
            <div>
              <el-button size="small" @click="exportData('json')"><el-icon><Download /></el-icon>JSON</el-button>
              <el-button size="small" @click="exportData('csv')">CSV</el-button>
              <el-button size="small" @click="exportData('txt')">TXT</el-button>
              <el-select v-model="clearRange" size="small" class="clear-select"><el-option label="清空全部" value="all" /><el-option label="清理近 24 小时" value="day" /><el-option label="清理查询时间" value="filter" /></el-select>
              <el-button size="small" type="danger" plain @click="clearHistory">清理</el-button>
            </div>
          </div>
          <el-table :data="history" class="history-table" height="410">
            <el-table-column label="时间" width="164"><template #default="scope">{{ new Date(scope.row.ts).toLocaleString('zh-CN', { hour12: false }) }}</template></el-table-column>
            <el-table-column label="平台" width="100" prop="source" />
            <el-table-column label="类型" width="90"><template #default="scope">{{ typeLabel(scope.row.kind) }}</template></el-table-column>
            <el-table-column label="昵称" width="140" prop="userName" />
            <el-table-column label="内容" min-width="230"><template #default="scope">{{ recordContent(scope.row) }}</template></el-table-column>
            <el-table-column label="命中规则" min-width="150" prop="matchedRuleName" />
            <el-table-column label="结果" width="90"><template #default="scope"><el-tag size="small" :type="resultType(scope.row.actionResult)">{{ scope.row.actionResult }}</el-tag></template></el-table-column>
          </el-table>
          <div class="pagination-line"><el-pagination v-model:current-page="page" background layout="prev, pager, next" :page-size="pageSize" :total="total" @current-change="loadHistory" /></div>
        </el-tab-pane>
      </el-tabs>
    </div>
  </div>
</template>
