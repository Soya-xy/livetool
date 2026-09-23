import type { Action, ActionResult, EventOutcome, LiveEvent, Rule } from '@shared/types'

export interface RuleExecutionContext {
  executeAction: (action: Action, event: LiveEvent, rule: Rule) => Promise<{ ok: boolean; message?: string }>
  log: (level: 'info' | 'warn' | 'error' | 'debug', message: string, detail?: string) => void
}

export interface RuleEngineOptions {
  random?: () => number
  now?: () => number
}

export function eventContent(event: LiveEvent): string {
  if (event.kind === 'gift') return event.gift?.name ?? ''
  if (event.kind === 'chat') return event.text ?? ''
  return event.text ?? event.gift?.name ?? ''
}

export function matchRule(rule: Rule, event: LiveEvent): boolean {
  if (!rule.enabled || !rule.trigger.kinds.includes(event.kind)) return false
  if (rule.trigger.source?.length && !rule.trigger.source.includes(event.source)) return false

  const userName = event.user?.name ?? ''
  if (rule.trigger.users?.length && !rule.trigger.users.includes(userName)) return false
  if (rule.trigger.minCount && (event.gift?.count ?? event.count ?? 0) < rule.trigger.minCount) return false
  if (rule.trigger.giftNames?.length && !rule.trigger.giftNames.includes(event.gift?.name ?? '')) return false

  const keywords = rule.trigger.keywords?.filter(Boolean) ?? []
  if (!keywords.length) return true
  const content = eventContent(event)
  const mode = rule.trigger.keywordMode ?? 'contains'
  return keywords.some((keyword) => {
    if (mode === 'exact') return content === keyword
    if (mode === 'regex') {
      try {
        return new RegExp(keyword, 'i').test(content)
      } catch {
        return false
      }
    }
    return content.toLocaleLowerCase().includes(keyword.toLocaleLowerCase())
  })
}

export class RuleEngine {
  private readonly nextAllowed = new Map<string, number>()
  private readonly active = new Map<string, number>()
  private readonly queues = new Map<string, Promise<unknown>>()
  private readonly random: () => number
  private readonly now: () => number

  constructor(private readonly context: RuleExecutionContext, options: RuleEngineOptions = {}) {
    this.random = options.random ?? Math.random
    this.now = options.now ?? Date.now
  }

  async process(event: LiveEvent, rules: Rule[]): Promise<EventOutcome> {
    const candidates = rules
      .filter((rule) => matchRule(rule, event))
      .sort((a, b) => b.priority - a.priority || (a.updatedAt ?? 0) - (b.updatedAt ?? 0))

    for (const rule of candidates) {
      const skip = this.checkSkip(rule)
      if (skip) {
        this.context.log('debug', `[rule:${rule.name}] skipped: ${skip}`)
        continue
      }
      const result = await this.schedule(rule, event)
      if (result.result !== 'skipped') return result
    }
    return { result: 'none', reason: candidates.length ? '所有匹配规则均被跳过' : undefined }
  }

  private checkSkip(rule: Rule): string | undefined {
    const now = this.now()
    const next = this.nextAllowed.get(rule.id) ?? 0
    if (now < next) return `冷却中，还需 ${next - now}ms`
    if (rule.probability !== undefined && this.random() > rule.probability) return '概率未命中'
    if (rule.concurrency === 'exclusive' && (this.active.get(rule.id) ?? 0) > 0) return '互斥动作正在执行'
    return undefined
  }

  private schedule(rule: Rule, event: LiveEvent): Promise<EventOutcome> {
    const run = async (): Promise<EventOutcome> => {
      const now = this.now()
      if (rule.cooldownMs) this.nextAllowed.set(rule.id, now + rule.cooldownMs)
      this.active.set(rule.id, (this.active.get(rule.id) ?? 0) + 1)
      this.context.log('info', `[rule:${rule.name}] matched priority=${rule.priority}`)
      try {
        for (const action of rule.actions) {
          const repeats = Math.max(1, action.repeat ?? 1)
          for (let i = 0; i < repeats; i += 1) {
            if (action.delayMs) await delay(action.delayMs)
            const result = await this.context.executeAction(action, event, rule)
            if (!result.ok) {
              this.context.log('warn', `[rule:${rule.name}] action failed`, result.message)
              return { ruleId: rule.id, ruleName: rule.name, result: 'failed', reason: result.message }
            }
          }
        }
        return { ruleId: rule.id, ruleName: rule.name, result: 'ok' }
      } catch (error) {
        const reason = error instanceof Error ? error.message : String(error)
        this.context.log('error', `[rule:${rule.name}] action error`, reason)
        return { ruleId: rule.id, ruleName: rule.name, result: 'failed', reason }
      } finally {
        this.active.set(rule.id, Math.max(0, (this.active.get(rule.id) ?? 1) - 1))
      }
    }

    if (rule.concurrency === 'queue') {
      const previous = this.queues.get(rule.id) ?? Promise.resolve()
      const current = previous.then(run, run)
      this.queues.set(rule.id, current.finally(() => {
        if (this.queues.get(rule.id) === current) this.queues.delete(rule.id)
      }))
      return current
    }
    if (rule.concurrency === 'replace') {
      this.context.log('debug', `[rule:${rule.name}] replace policy: start latest action`)
    }
    return run()
  }
}

export function describeRuleTrigger(rule: Rule): string {
  const kinds = rule.trigger.kinds.map((kind) => ({ gift: '礼物', chat: '弹幕', like: '点赞', follow: '关注', enter: '进场', system: '系统' })[kind]).join(' / ')
  const details = [...(rule.trigger.giftNames ?? []), ...(rule.trigger.keywords ?? [])]
  return details.length ? `${kinds} · ${details.join('、')}` : kinds || '未设置触发'
}

export function actionLabel(action: Action): string {
  return ({ key: '键盘', mouse: '鼠标', video: '视频', audio: '声音', drop: '砸落物', slot: '水果机', serial: '串口', obs: 'OBS' })[action.kind]
}

export function delay(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms))
}

export function resultToActionResult(ok: boolean): ActionResult {
  return ok ? 'ok' : 'failed'
}
