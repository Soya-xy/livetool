import type {
  LicenseCardCreateInput,
  LicenseCardDetail,
  LicenseCardPage,
  LicenseCardQuery,
  LicenseCardStatus,
  LicenseEntitlement,
  LicenseAuditEntry,
  Platform,
} from '@shared/types'

interface RemoteAuthResponse {
  session: string
  expires_at: string
  session_expires_at: string
  features: LicenseEntitlement[]
  safe_code_set: boolean
}

export interface RemoteAuthSession {
  expiresAt: string
  sessionExpiresAt: string
  features: LicenseEntitlement[]
  safeCodeSet: boolean
}

interface ApiEnvelope {
  ok?: boolean
  message?: string
}

export class LicenseClient {
  private userSession = ''
  private userServerUrl = ''
  private adminToken = ''
  private adminServerUrl = ''
  private adminExpiresAt = ''

  async login(serverUrl: string, code: string, platform: Platform, clientId: string): Promise<RemoteAuthSession> {
    await this.logout().catch(() => undefined)
    const baseUrl = normalizeServerUrl(serverUrl)
    const result = await this.request<RemoteAuthResponse>(baseUrl, '/v1/auth/login', {
      method: 'POST',
      body: JSON.stringify({ code, platform, client_id: clientId }),
    })
    if (!result.session || !isValidDate(result.expires_at) || !isValidDate(result.session_expires_at) || !isEntitlementList(result.features)) {
      throw new Error('授权服务返回的数据不完整')
    }
    this.userSession = result.session
    this.userServerUrl = baseUrl
    return {
      expiresAt: result.expires_at,
      sessionExpiresAt: result.session_expires_at,
      features: result.features,
      safeCodeSet: Boolean(result.safe_code_set),
    }
  }

  async status(serverUrl: string): Promise<RemoteAuthSession> {
    const baseUrl = normalizeServerUrl(serverUrl)
    if (!this.userSession || this.userServerUrl !== baseUrl) throw new Error('当前没有有效的远程授权会话')
    try {
      const result = await this.request<RemoteAuthResponse>(baseUrl, '/v1/auth/status', { method: 'GET' }, this.userSession)
      if (!isValidDate(result.expires_at) || !isValidDate(result.session_expires_at) || !isEntitlementList(result.features)) {
        throw new Error('授权服务返回的数据不完整')
      }
      return {
        expiresAt: result.expires_at,
        sessionExpiresAt: result.session_expires_at,
        features: result.features,
        safeCodeSet: Boolean(result.safe_code_set),
      }
    } catch (error) {
      this.userSession = ''
      this.userServerUrl = ''
      throw error
    }
  }

  async logout(): Promise<void> {
    const token = this.userSession
    const baseUrl = this.userServerUrl
    this.userSession = ''
    this.userServerUrl = ''
    if (token && baseUrl) await this.request(baseUrl, '/v1/auth/logout', { method: 'POST' }, token)
  }

  async setSafeCode(serverUrl: string, code: string): Promise<void> {
    const baseUrl = this.requireUserSession(serverUrl)
    await this.request(baseUrl, '/v1/auth/safe-code', { method: 'POST', body: JSON.stringify({ safe_code: code }) }, this.userSession)
  }

  async unbind(serverUrl: string, safeCode: string): Promise<void> {
    const baseUrl = this.requireUserSession(serverUrl)
    await this.request(baseUrl, '/v1/auth/unbind', { method: 'POST', body: JSON.stringify({ safe_code: safeCode }) }, this.userSession)
    this.userSession = ''
    this.userServerUrl = ''
  }

  async adminLogin(serverUrl: string, username: string, password: string): Promise<{ expiresAt: string }> {
    this.clearAdminSession()
    const baseUrl = normalizeServerUrl(serverUrl)
    const result = await this.request<{ token: string; expires_at: string }>(baseUrl, '/v1/admin/login', {
      method: 'POST',
      body: JSON.stringify({ username, password }),
    })
    if (!result.token || !isValidDate(result.expires_at) || Date.parse(result.expires_at) <= Date.now()) throw new Error('管理员登录响应缺少有效会话信息')
    this.adminToken = result.token
    this.adminServerUrl = baseUrl
    this.adminExpiresAt = result.expires_at
    return { expiresAt: result.expires_at }
  }

  async adminStatus(serverUrl: string): Promise<{ loggedIn: boolean; expiresAt?: string }> {
    const baseUrl = normalizeServerUrl(serverUrl)
    if (!this.adminToken || this.adminServerUrl !== baseUrl || Date.now() >= Date.parse(this.adminExpiresAt)) {
      this.clearAdminSession()
      return { loggedIn: false }
    }
    try {
      const result = await this.request<{ expires_at: string }>(baseUrl, '/v1/admin/me', { method: 'GET' }, this.adminToken)
      if (!isValidDate(result.expires_at) || Date.parse(result.expires_at) <= Date.now()) throw new Error('管理员会话已失效')
      this.adminExpiresAt = result.expires_at
      return { loggedIn: true, expiresAt: result.expires_at }
    } catch {
      this.clearAdminSession()
      return { loggedIn: false }
    }
  }

  adminLogout(): void { this.clearAdminSession() }

  async listCards(serverUrl: string, query: LicenseCardQuery): Promise<LicenseCardPage> {
    const baseUrl = this.requireAdminSession(serverUrl)
    const params = new URLSearchParams()
    if (query.query) params.set('q', query.query)
    if (query.status) params.set('status', query.status)
    params.set('page', String(query.page ?? 1))
    params.set('pageSize', String(query.pageSize ?? 20))
    return this.request<LicenseCardPage>(baseUrl, `/v1/admin/cards?${params.toString()}`, { method: 'GET' }, this.adminToken)
  }

  async createCards(serverUrl: string, input: LicenseCardCreateInput): Promise<Array<{ id: string; code: string; suffix: string }>> {
    const baseUrl = this.requireAdminSession(serverUrl)
    const result = await this.request<{ items: Array<{ id: string; code: string; suffix: string }> }>(baseUrl, '/v1/admin/cards', {
      method: 'POST', body: JSON.stringify(input),
    }, this.adminToken)
    return result.items
  }

  async cardDetail(serverUrl: string, id: string): Promise<LicenseCardDetail> {
    const baseUrl = this.requireAdminSession(serverUrl)
    const result = await this.request<{ item: LicenseCardDetail }>(baseUrl, `/v1/admin/cards/${encodeURIComponent(id)}`, { method: 'GET' }, this.adminToken)
    return result.item
  }

  async setCardStatus(serverUrl: string, id: string, status: Extract<LicenseCardStatus, 'active' | 'disabled' | 'revoked'>): Promise<void> {
    const baseUrl = this.requireAdminSession(serverUrl)
    await this.request(baseUrl, `/v1/admin/cards/${encodeURIComponent(id)}/status`, { method: 'PATCH', body: JSON.stringify({ status }) }, this.adminToken)
  }

  async resetCardBindings(serverUrl: string, id: string): Promise<number> {
    const baseUrl = this.requireAdminSession(serverUrl)
    const result = await this.request<{ removed: number }>(baseUrl, `/v1/admin/cards/${encodeURIComponent(id)}/bindings`, { method: 'DELETE' }, this.adminToken)
    return result.removed
  }

  async extendCard(serverUrl: string, id: string, days: number): Promise<string | undefined> {
    const baseUrl = this.requireAdminSession(serverUrl)
    const result = await this.request<{ expires_at: string }>(baseUrl, `/v1/admin/cards/${encodeURIComponent(id)}/extend`, { method: 'POST', body: JSON.stringify({ days }) }, this.adminToken)
    return result.expires_at
  }

  async auditLog(serverUrl: string, limit = 100): Promise<LicenseAuditEntry[]> {
    const baseUrl = this.requireAdminSession(serverUrl)
    const result = await this.request<{ items: LicenseAuditEntry[] }>(baseUrl, `/v1/admin/audit?limit=${encodeURIComponent(String(limit))}`, { method: 'GET' }, this.adminToken)
    return result.items
  }

  private requireUserSession(serverUrl: string): string {
    const baseUrl = normalizeServerUrl(serverUrl)
    if (!this.userSession || baseUrl !== this.userServerUrl) throw new Error('当前没有有效的远程授权会话')
    return baseUrl
  }

  private requireAdminSession(serverUrl: string): string {
    const baseUrl = normalizeServerUrl(serverUrl)
    if (!this.adminToken || baseUrl !== this.adminServerUrl || Date.now() >= Date.parse(this.adminExpiresAt)) {
      this.clearAdminSession()
      throw new Error('管理员会话已失效，请重新登录')
    }
    return baseUrl
  }

  private clearAdminSession(): void {
    this.adminToken = ''
    this.adminServerUrl = ''
    this.adminExpiresAt = ''
  }

  private async request<T = ApiEnvelope>(baseUrl: string, path: string, init: RequestInit, token?: string): Promise<T> {
    const headers = new Headers(init.headers)
    headers.set('Accept', 'application/json')
    if (init.body) headers.set('Content-Type', 'application/json')
    if (token) headers.set('Authorization', `Bearer ${token}`)
    let response: Response
    try {
      response = await fetch(new URL(path, `${baseUrl}/`), { ...init, headers, signal: AbortSignal.timeout(10000) })
    } catch {
      throw new Error('无法连接授权服务器，请检查地址和网络')
    }
    let payload: T & ApiEnvelope
    try { payload = await response.json() as T & ApiEnvelope } catch { throw new Error('授权服务器返回了无效响应') }
    if (!response.ok || payload.ok === false) throw new Error(typeof payload.message === 'string' ? payload.message : `授权服务请求失败 (${response.status})`)
    return payload as T
  }
}

export function normalizeServerUrl(input: string): string {
  let url: URL
  try { url = new URL(input.trim()) } catch { throw new Error('请输入有效的授权服务器地址') }
  if (!['http:', 'https:'].includes(url.protocol) || url.username || url.password || url.search || url.hash || (url.pathname !== '/' && url.pathname !== '')) throw new Error('授权服务器地址只能使用 http 或 https 根地址，且不能含账号、路径、参数或片段')
  const loopback = ['localhost', '127.0.0.1', '[::1]'].includes(url.hostname.toLowerCase())
  if (url.protocol !== 'https:' && !loopback) throw new Error('远程授权服务器必须使用 HTTPS')
  return url.origin
}

function isValidDate(value: unknown): value is string {
  return typeof value === 'string' && Number.isFinite(Date.parse(value))
}

function isEntitlementList(value: unknown): value is LicenseEntitlement[] {
  return Array.isArray(value) && value.every((item) => item === 'overlay' || item === 'slot' || item === 'serial')
}
