import type { ConnectorState, ConnectorStatus } from './types'

/**
 * 连接状态文案与配色，取自原版 i18n（app.webcast.*）：
 * connect=正在连接.. / reConnect=重新连接到直播间.. / success=已连接到直播间
 * / error=连接到直播间失败 / break=直播间连接断开 / timeout=连接超时。
 */
export function connectorStateText(state: ConnectorState): string {
  switch (state) {
    case 'connecting': return '正在连接..'
    case 'reconnecting': return '重新连接到直播间..'
    case 'connected': return '已连接到直播间'
    case 'error': return '连接到直播间失败'
    case 'break': return '直播间连接断开'
    case 'timeout': return '连接超时'
    default: return '未连接直播间'
  }
}

export function connectorStateTone(state: ConnectorState): 'success' | 'warning' | 'danger' | 'info' {
  if (state === 'connected') return 'success'
  if (state === 'connecting' || state === 'reconnecting') return 'warning'
  if (state === 'disconnected') return 'info'
  return 'danger'
}

/** 连接中：按钮显示进度且不可重复点击。 */
export function connectorBusy(status: ConnectorStatus): boolean {
  return status.state === 'connecting' || status.state === 'reconnecting'
}

/** 侧栏按钮文案：已连接时是「断开连接」，其余情况是「连接到直播间」。 */
export function connectorActionText(status: ConnectorStatus): string {
  if (connectorBusy(status)) return connectorStateText(status.state)
  if (status.state === 'connected') return '断开连接'
  return '连接到直播间'
}

export function connectorPlatformText(platform: string): string {
  switch (platform) {
    case 'douyin': return '抖音'
    case 'kuaishou': return '快手'
    case 'shipinhao': return '视频号'
    case 'bilibili': return 'B站'
    case 'tiktok': return 'TIKTOK'
    case 'douyu': return '斗鱼'
    case 'xiaohongshu': return '小红书'
    case 'simulator': return '本地模拟器'
    default: return platform || '未选择平台'
  }
}
