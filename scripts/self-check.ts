import assert from 'node:assert/strict'
import { matchRule, RuleEngine } from '../src/main/core/rule-engine'
import type { LiveEvent, Rule } from '../src/shared/types'

const giftEvent: LiveEvent = {
  id: 'event-1',
  source: 'simulator',
  kind: 'gift',
  user: { id: 'user-1', name: '小明' },
  gift: { name: '小星星', count: 3 },
  timestamp: 1_000,
}

const rule: Rule = {
  id: 'rule-1',
  name: '礼物触发',
  enabled: true,
  priority: 10,
  trigger: {
    kinds: ['gift'],
    giftNames: ['小星星'],
    minCount: 2,
    users: ['小明'],
  },
  cooldownMs: 1_000,
  probability: 1,
  concurrency: 'parallel',
  actions: [{ id: 'action-1', kind: 'audio', path: 'voices/test.mp3' }],
}

assert.equal(matchRule(rule, giftEvent), true)
assert.equal(matchRule(rule, { ...giftEvent, gift: { name: '小星星', count: 1 } }), false)
assert.equal(matchRule({ ...rule, trigger: { kinds: ['chat'], keywords: ['hello'], keywordMode: 'exact' } }, {
  ...giftEvent,
  kind: 'chat',
  gift: undefined,
  text: 'hello',
}), true)
assert.equal(matchRule({ ...rule, trigger: { kinds: ['chat'], keywords: ['^hello\\s+world$'], keywordMode: 'regex' } }, {
  ...giftEvent,
  kind: 'chat',
  gift: undefined,
  text: 'hello world',
}), true)

let now = 1_000
let executions = 0
const engine = new RuleEngine({
  executeAction: async () => {
    executions += 1
    return { ok: true }
  },
  log: () => undefined,
}, { now: () => now, random: () => 0 })

const first = await engine.process(giftEvent, [rule])
assert.equal(first.result, 'ok')
assert.equal(executions, 1)

now = 1_500
const duringCooldown = await engine.process(giftEvent, [rule])
assert.equal(duringCooldown.result, 'none')
assert.equal(executions, 1)

now = 2_100
const afterCooldown = await engine.process(giftEvent, [rule])
assert.equal(afterCooldown.result, 'ok')
assert.equal(executions, 2)

console.log('self-check passed: rule matching, regex, min-count, cooldown, and action execution')
