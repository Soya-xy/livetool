import type { Action, LiveEvent, Rule } from '@shared/types'
import type { AppLogger } from './logger'
import type { WindowManager } from './windows'

export class ActionExecutor {
  constructor(private readonly windows: WindowManager, private readonly logger: AppLogger) {}

  async execute(action: Action, event: LiveEvent, rule: Rule): Promise<{ ok: boolean; message?: string }> {
    switch (action.kind) {
      case 'video':
        this.logger.info(`[action:video] lane=${action.lane ?? 1} started path=${action.path}`)
        await this.windows.playVideo({ path: action.path, lane: action.lane, durationMs: action.durationMs, loop: action.loop, chroma: action.chroma })
        return { ok: true }
      case 'drop':
        this.logger.info(`[action:drop] count=${action.count ?? 1} image=${action.image}`)
        await this.windows.drop({ image: action.image, count: action.count, gravity: action.gravity, bounce: action.bounce, durationMs: action.durationMs })
        return { ok: true }
      case 'slot':
        this.logger.info('[action:slot] start weighted result')
        await this.windows.slot({ theme: action.theme, pool: action.pool, weights: action.weights })
        return { ok: true }
      case 'audio':
        this.logger.info(`[action:audio] path=${action.path} interrupt=${Boolean(action.interrupt)}`)
        await this.windows.playAudio({ path: action.path, volume: action.volume, interrupt: action.interrupt, loop: action.loop })
        return { ok: true }
      case 'key':
        this.logger.info(`[action:key] target=${action.target?.processName ?? action.target?.title ?? 'foreground'} steps=${action.steps.length} event=${event.kind}`)
        return { ok: true, message: '本地演示已记录；Windows 原生 SendInput 适配可在 input-helper 中替换' }
      case 'mouse':
        this.logger.info(`[action:mouse] target=${action.target?.processName ?? action.target?.title ?? 'foreground'} steps=${action.steps.length}`)
        return { ok: true, message: '本地演示已记录；Windows 坐标/查图适配可在 input-helper 中替换' }
      case 'serial':
        this.logger.info(`[action:serial] ${action.port} pulse=${action.pulseMs}ms bytes=${action.onBytes.join(',')}`)
        return { ok: true, message: '虚拟串口模式：已记录脉冲，未向物理端口写入' }
      case 'obs':
        this.logger.info(`[action:obs] command=${action.command}`)
        return { ok: true, message: 'OBS 未连接时降级为日志动作' }
    }
  }
}
