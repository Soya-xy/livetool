import { createServer } from 'node:http'
import { createHmac, randomBytes, randomUUID, timingSafeEqual } from 'node:crypto'
import { mkdir, readFile, rename, unlink, writeFile } from 'node:fs/promises'
import { dirname, resolve } from 'node:path'

const host = process.env.CARD_HOST || '127.0.0.1'
const port = Number(process.env.CARD_PORT || 8787)
const dataFile = resolve(process.env.CARD_DATA_FILE || 'license-data/cards.json')
const adminUser = process.env.CARD_ADMIN_USERNAME || ''
const adminPassword = process.env.CARD_ADMIN_PASSWORD || ''
const pepper = process.env.CARD_KEY_PEPPER || ''
const SESSION_TTL_MS = 3 * 60 * 1000
const ADMIN_TTL_MS = 8 * 60 * 60 * 1000
const BODY_LIMIT = 64 * 1024
const PLATFORMS = new Set(['simulator', 'douyin', 'kuaishou', 'shipinhao', 'bilibili', 'tiktok', 'douyu', 'xiaohongshu'])
const ENTITLEMENTS = new Set(['overlay', 'slot', 'serial'])

if (!Number.isInteger(port) || port < 1 || port > 65535) throw new Error('CARD_PORT must be between 1 and 65535')
if (adminUser.length < 1 || adminPassword.length < 12) throw new Error('Set CARD_ADMIN_USERNAME and a CARD_ADMIN_PASSWORD of at least 12 characters')
if (Buffer.byteLength(pepper, 'utf8') < 32) throw new Error('CARD_KEY_PEPPER must contain at least 32 bytes')

await mkdir(dirname(dataFile), { recursive: true })
let data = await loadData()
let writeQueue = Promise.resolve()
const adminSessions = new Map()
const userSessions = new Map()
const adminFailures = new Map()
const adminPasswordDigest = digest(`admin\0${adminUser}\0${adminPassword}`)

const server = createServer((request, response) => {
  void route(request, response).catch((error) => {
    const status = error instanceof HttpError ? error.status : 500
    if (status === 500) console.error(`[license-api] ${request.method} ${new URL(request.url || '/', 'http://local').pathname} failed`)
    json(response, status, { ok: false, message: status === 500 ? '授权服务内部错误' : error.message })
  })
})

server.listen(port, host, () => {
  console.log(`Card authorization service listening on ${host}:${port}`)
  if (host !== '127.0.0.1' && host !== '::1' && host !== 'localhost') console.warn('Expose this service only behind HTTPS and a trusted reverse proxy.')
})

async function loadData() {
  try {
    const parsed = JSON.parse(await readFile(dataFile, 'utf8'))
    if (parsed?.version !== 1 || !Array.isArray(parsed.cards) || !Array.isArray(parsed.audit)) throw new Error('invalid data shape')
    return parsed
  } catch (error) {
    if (error?.code === 'ENOENT') return { version: 1, cards: [], audit: [] }
    throw new Error(`Cannot read card data file: ${dataFile}`)
  }
}

function persist() {
  const snapshot = JSON.stringify(data)
  const write = writeQueue.then(async () => {
    const temporary = `${dataFile}.${process.pid}.${randomBytes(4).toString('hex')}.tmp`
    try {
      await writeFile(temporary, snapshot, { encoding: 'utf8', mode: 0o600 })
      await rename(temporary, dataFile)
    } catch (error) {
      await unlink(temporary).catch(() => undefined)
      throw error
    }
  })
  writeQueue = write.catch(() => undefined)
  return write
}

async function route(request, response) {
  response.setHeader('Cache-Control', 'no-store')
  response.setHeader('X-Content-Type-Options', 'nosniff')
  const url = new URL(request.url || '/', `http://${request.headers.host || 'localhost'}`)
  const path = url.pathname

  if (request.method === 'GET' && path === '/health') return json(response, 200, { ok: true })
  if (request.method === 'POST' && path === '/v1/admin/login') return adminLogin(request, response)
  if (request.method === 'POST' && path === '/v1/auth/login') return authLogin(request, response)
  if (request.method === 'GET' && path === '/v1/auth/status') return authStatus(request, response)
  if (request.method === 'POST' && path === '/v1/auth/logout') return authLogout(request, response)
  if (request.method === 'POST' && path === '/v1/auth/safe-code') return setSafeCode(request, response)
  if (request.method === 'POST' && path === '/v1/auth/unbind') return unbindDevice(request, response)

  if (!path.startsWith('/v1/admin/')) throw new HttpError(404, '接口不存在')
  const admin = requireAdmin(request)
  if (request.method === 'GET' && path === '/v1/admin/me') return json(response, 200, { ok: true, expires_at: new Date(admin.expiresAt).toISOString() })
  if (request.method === 'GET' && path === '/v1/admin/cards') return listCards(url, response)
  if (request.method === 'POST' && path === '/v1/admin/cards') return createCards(request, response, admin)
  if (request.method === 'GET' && path === '/v1/admin/audit') {
    const limit = clampInteger(url.searchParams.get('limit'), 100, 1, 500)
    return json(response, 200, { items: data.audit.slice(0, limit) })
  }

  const cardRoute = path.match(/^\/v1\/admin\/cards\/([a-f0-9-]+)(?:\/(status|bindings|extend))?$/i)
  if (cardRoute) {
    const [, id, operation] = cardRoute
    const card = data.cards.find((item) => item.id === id)
    if (!card) throw new HttpError(404, '卡密记录不存在')
    if (request.method === 'GET' && !operation) return json(response, 200, { item: detailCard(card) })
    if (request.method === 'PATCH' && operation === 'status') return changeCardStatus(request, response, card, admin)
    if (request.method === 'DELETE' && operation === 'bindings') return resetBindings(response, card, admin)
    if (request.method === 'POST' && operation === 'extend') return extendCard(request, response, card, admin)
  }
  throw new HttpError(404, '接口不存在')
}

async function adminLogin(request, response) {
  const ip = request.socket.remoteAddress || 'unknown'
  const failures = (adminFailures.get(ip) || []).filter((time) => Date.now() - time < 5 * 60_000)
  if (failures.length >= 5) throw new HttpError(429, '登录尝试过多，请 5 分钟后重试')
  const body = await readBody(request)
  const suppliedUser = typeof body.username === 'string' ? body.username : ''
  const suppliedPassword = typeof body.password === 'string' ? body.password : ''
  const suppliedDigest = digest(`admin\0${suppliedUser}\0${suppliedPassword}`)
  if (suppliedUser !== adminUser || !constantTimeEqual(suppliedDigest, adminPasswordDigest)) {
    failures.push(Date.now())
    adminFailures.set(ip, failures)
    throw new HttpError(401, '管理员账号或密码错误')
  }
  adminFailures.delete(ip)
  const token = randomBytes(32).toString('base64url')
  const expiresAt = Date.now() + ADMIN_TTL_MS
  adminSessions.set(token, { expiresAt, username: adminUser })
  audit('admin.login')
  await persist()
  return json(response, 200, { ok: true, token, expires_at: new Date(expiresAt).toISOString() })
}

async function authLogin(request, response) {
  const body = await readBody(request)
  const code = typeof body.code === 'string' ? normalizeCode(body.code) : ''
  const clientId = typeof body.client_id === 'string' ? body.client_id.trim() : ''
  const platform = typeof body.platform === 'string' ? body.platform : ''
  if (code.length < 16 || clientId.length < 16 || clientId.length > 128 || !PLATFORMS.has(platform)) throw new HttpError(400, '卡密、设备标识或平台参数无效')
  const codeHash = digest(`card\0${code}`)
  const card = data.cards.find((item) => constantTimeEqual(item.codeHash, codeHash))
  if (!card) throw new HttpError(401, '卡密无效')
  if (card.status === 'disabled') throw new HttpError(403, '卡密已停用')
  if (card.status === 'revoked') throw new HttpError(403, '卡密已撤销')
  if (card.platforms.length && !card.platforms.includes(platform)) throw new HttpError(403, '此卡密不支持当前平台')

  const now = Date.now()
  if (card.expiresAt && now >= card.expiresAt) throw new HttpError(410, '卡密已过期')
  const clientHash = digest(`device\0${clientId}`)
  let binding = card.bindings.find((item) => item.clientHash === clientHash)
  if (!binding && card.bindings.length >= card.maxDevices) throw new HttpError(409, '卡密绑定设备数量已达上限')
  if (!binding) {
    binding = { clientHash, boundAt: now }
    card.bindings.push(binding)
    audit('device.bound', card.id)
  }
  if (!card.activatedAt) {
    card.activatedAt = now
    card.expiresAt = now + card.durationDays * 24 * 60 * 60 * 1000
    audit('card.activated', card.id)
  }
  await persist()

  const token = randomBytes(32).toString('base64url')
  const sessionExpiresAt = now + SESSION_TTL_MS
  userSessions.set(token, { cardId: card.id, clientHash, platform, expiresAt: sessionExpiresAt })
  return json(response, 200, {
    ok: true,
    session: token,
    expires_at: new Date(card.expiresAt).toISOString(),
    session_expires_at: new Date(sessionExpiresAt).toISOString(),
    features: card.features,
    safe_code_set: Boolean(card.safeCodeHash),
  })
}

async function authStatus(request, response) {
  const session = requireUser(request)
  const card = cardForSession(session)
  session.expiresAt = Date.now() + SESSION_TTL_MS
  return json(response, 200, {
    ok: true,
    expires_at: new Date(card.expiresAt).toISOString(),
    session_expires_at: new Date(session.expiresAt).toISOString(),
    features: card.features,
    safe_code_set: Boolean(card.safeCodeHash),
  })
}

async function authLogout(request, response) {
  const token = bearer(request)
  if (token) userSessions.delete(token)
  return json(response, 200, { ok: true })
}

async function setSafeCode(request, response) {
  const session = requireUser(request)
  const card = cardForSession(session)
  const body = await readBody(request)
  const safeCode = typeof body.safe_code === 'string' ? body.safe_code.trim() : ''
  if (safeCode.length < 6 || safeCode.length > 64) throw new HttpError(400, '安全码长度需要为 6 到 64 个字符')
  card.safeCodeHash = digest(`safe\0${safeCode}`)
  audit('safe-code.changed', card.id)
  await persist()
  return json(response, 200, { ok: true })
}

async function unbindDevice(request, response) {
  const token = bearer(request)
  const session = requireUser(request)
  const card = cardForSession(session)
  const body = await readBody(request)
  const safeCode = typeof body.safe_code === 'string' ? body.safe_code.trim() : ''
  if (!card.safeCodeHash || !constantTimeEqual(digest(`safe\0${safeCode}`), card.safeCodeHash)) throw new HttpError(403, '安全码不正确或尚未设置安全码')
  card.bindings = card.bindings.filter((item) => item.clientHash !== session.clientHash)
  for (const [sessionToken, active] of userSessions) {
    if (active.cardId === card.id && active.clientHash === session.clientHash) userSessions.delete(sessionToken)
  }
  audit('device.unbound', card.id)
  await persist()
  return json(response, 200, { ok: true, unbound: Boolean(token) })
}

async function listCards(url, response) {
  const query = (url.searchParams.get('q') || '').trim().toLocaleLowerCase()
  const status = url.searchParams.get('status') || ''
  const page = clampInteger(url.searchParams.get('page'), 1, 1, 1_000_000)
  const pageSize = clampInteger(url.searchParams.get('pageSize'), 20, 1, 100)
  let cards = data.cards.filter((card) => !query || `${card.id} ${card.suffix} ${card.note}`.toLocaleLowerCase().includes(query))
  if (status) cards = cards.filter((card) => displayStatus(card) === status)
  const start = (page - 1) * pageSize
  return json(response, 200, { items: cards.slice(start, start + pageSize).map(summaryCard), total: cards.length, page, pageSize })
}

async function createCards(request, response, admin) {
  const body = await readBody(request)
  const count = integerField(body.count, '生成数量', 1, 1000)
  const durationDays = integerField(body.durationDays, '有效天数', 1, 36500)
  const maxDevices = integerField(body.maxDevices, '设备数上限', 1, 100000)
  const platforms = uniqueValues(body.platforms, PLATFORMS, '平台')
  const features = uniqueValues(body.features, ENTITLEMENTS, '功能权限')
  if (!features.length) throw new HttpError(400, '至少选择一项功能权限')
  const note = typeof body.note === 'string' ? body.note.trim().slice(0, 200) : ''
  const issued = []
  for (let index = 0; index < count; index += 1) {
    let code = ''
    let codeHash = ''
    do {
      code = randomBytes(16).toString('hex').toUpperCase().match(/.{1,4}/g).join('-')
      codeHash = digest(`card\0${normalizeCode(code)}`)
    } while (data.cards.some((card) => card.codeHash === codeHash))
    const card = {
      id: randomUUID(), codeHash, suffix: code.slice(-4), status: 'active', createdAt: Date.now(),
      activatedAt: null, expiresAt: null, durationDays, maxDevices, bindings: [], platforms, features, note,
      safeCodeHash: null,
    }
    data.cards.unshift(card)
    issued.push({ id: card.id, code, suffix: card.suffix })
  }
  audit('cards.created', undefined, `count=${count}; admin=${admin.username}`)
  await persist()
  return json(response, 201, { items: issued })
}

async function changeCardStatus(request, response, card, admin) {
  const body = await readBody(request)
  const next = body.status
  if (!['active', 'disabled', 'revoked'].includes(next)) throw new HttpError(400, '卡密状态无效')
  if (card.status === 'revoked' && next !== 'revoked') throw new HttpError(409, '已撤销的卡密不可恢复，请重新生成')
  card.status = next
  if (next !== 'active') invalidateCardSessions(card.id)
  audit(`card.${next}`, card.id, `admin=${admin.username}`)
  await persist()
  return json(response, 200, { ok: true })
}

async function resetBindings(response, card, admin) {
  const count = card.bindings.length
  card.bindings = []
  invalidateCardSessions(card.id)
  audit('devices.reset', card.id, `count=${count}; admin=${admin.username}`)
  await persist()
  return json(response, 200, { ok: true, removed: count })
}

async function extendCard(request, response, card, admin) {
  const body = await readBody(request)
  const days = integerField(body.days, '续期天数', 1, 36500)
  if (card.status !== 'active') throw new HttpError(409, '只有有效卡密可以续期')
  if (card.activatedAt) {
    const base = card.expiresAt && card.expiresAt > Date.now() ? card.expiresAt : Date.now()
    card.expiresAt = base + days * 24 * 60 * 60 * 1000
  }
  card.durationDays += days
  audit('card.extended', card.id, `days=${days}; admin=${admin.username}`)
  await persist()
  return json(response, 200, { ok: true, expires_at: card.expiresAt ? new Date(card.expiresAt).toISOString() : undefined })
}

function requireAdmin(request) {
  const token = bearer(request)
  const session = token && adminSessions.get(token)
  if (!session || session.expiresAt <= Date.now()) {
    if (token) adminSessions.delete(token)
    throw new HttpError(401, '管理员会话已失效，请重新登录')
  }
  return session
}

function requireUser(request) {
  const token = bearer(request)
  const session = token && userSessions.get(token)
  if (!session || session.expiresAt <= Date.now()) {
    if (token) userSessions.delete(token)
    throw new HttpError(401, '授权会话已过期，请重新登录')
  }
  return session
}

function cardForSession(session) {
  const card = data.cards.find((item) => item.id === session.cardId)
  if (!card || card.status !== 'active') throw new HttpError(403, '卡密已停用或撤销')
  if (!card.bindings.some((item) => item.clientHash === session.clientHash)) throw new HttpError(403, '当前设备已解绑')
  if (!card.expiresAt || Date.now() >= card.expiresAt) throw new HttpError(410, '卡密已过期')
  return card
}

function invalidateCardSessions(cardId) {
  for (const [token, session] of userSessions) if (session.cardId === cardId) userSessions.delete(token)
}

function summaryCard(card) {
  return {
    id: card.id, suffix: card.suffix, status: displayStatus(card), createdAt: card.createdAt,
    activatedAt: card.activatedAt || undefined, expiresAt: card.expiresAt || undefined,
    durationDays: card.durationDays, maxDevices: card.maxDevices, boundDevices: card.bindings.length,
    platforms: card.platforms, features: card.features, note: card.note,
  }
}

function detailCard(card) {
  return {
    ...summaryCard(card),
    bindings: card.bindings.map((binding) => ({ fingerprint: binding.clientHash.slice(0, 12).toUpperCase(), boundAt: binding.boundAt })),
  }
}

function displayStatus(card) {
  if (card.status !== 'active') return card.status
  if (card.expiresAt && Date.now() >= card.expiresAt) return 'expired'
  if (!card.activatedAt) return 'pending'
  return 'active'
}

function audit(action, cardId, detail) {
  data.audit.unshift({ id: randomUUID(), action, cardId, at: Date.now(), detail })
  if (data.audit.length > 10000) data.audit.length = 10000
}

function digest(value) { return createHmac('sha256', pepper).update(value).digest('hex') }
function normalizeCode(value) { return value.toUpperCase().replace(/[^A-Z0-9]/g, '') }
function bearer(request) {
  const match = /^Bearer\s+([A-Za-z0-9_-]{32,})$/i.exec(request.headers.authorization || '')
  return match?.[1] || ''
}
function constantTimeEqual(left, right) {
  const a = Buffer.from(String(left))
  const b = Buffer.from(String(right))
  return a.length === b.length && timingSafeEqual(a, b)
}
function clampInteger(value, fallback, min, max) {
  if (value === null || value === undefined || String(value).trim() === '') return fallback
  const number = Number(value)
  return Number.isSafeInteger(number) ? Math.min(max, Math.max(min, number)) : fallback
}
function integerField(value, label, min, max) {
  if (!Number.isSafeInteger(value) || value < min || value > max) throw new HttpError(400, `${label}需要为 ${min} 到 ${max} 的整数`)
  return value
}
function uniqueValues(value, allowed, label) {
  if (value === undefined) return []
  if (!Array.isArray(value) || value.some((item) => typeof item !== 'string' || !allowed.has(item))) throw new HttpError(400, `${label}参数无效`)
  return [...new Set(value)]
}
async function readBody(request) {
  let size = 0
  const chunks = []
  for await (const chunk of request) {
    size += chunk.length
    if (size > BODY_LIMIT) throw new HttpError(413, '请求内容过大')
    chunks.push(chunk)
  }
  if (!size) return {}
  try {
    const body = JSON.parse(Buffer.concat(chunks).toString('utf8'))
    if (!body || typeof body !== 'object' || Array.isArray(body)) throw new Error('shape')
    return body
  } catch {
    throw new HttpError(400, '请求 JSON 格式无效')
  }
}

function json(response, status, body) {
  response.writeHead(status, { 'Content-Type': 'application/json; charset=utf-8' })
  response.end(JSON.stringify(body))
}

class HttpError extends Error {
  constructor(status, message) { super(message); this.status = status }
}
