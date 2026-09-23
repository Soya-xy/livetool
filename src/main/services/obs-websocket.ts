import { createHash, randomUUID } from 'node:crypto'

interface ObsMessage {
  op: number
  d: Record<string, unknown>
}

interface PendingRequest {
  resolve: (value: Record<string, unknown>) => void
  reject: (error: Error) => void
  timeout: ReturnType<typeof setTimeout>
}

export interface ObsStatus {
  connected: boolean
  url: string
  message: string
  virtualCameraActive: boolean
}

export class ObsWebSocketClient {
  private socket?: WebSocket
  private connected = false
  private url = ''
  private password = ''
  private message = 'OBS 未连接'
  private virtualCameraActive = false
  private identified?: { resolve: () => void; reject: (error: Error) => void; timeout: ReturnType<typeof setTimeout> }
  private readonly pending = new Map<string, PendingRequest>()

  async connect(url: string, password = ''): Promise<ObsStatus> {
    if (this.connected && this.url === url) return this.status()
    this.disconnect()
    this.url = url.trim()
    this.password = password
    this.message = '正在连接 OBS WebSocket…'

    return new Promise<ObsStatus>((resolve, reject) => {
      const timeout = setTimeout(() => fail(new Error('连接 OBS 超时，请确认 OBS WebSocket 已启用且地址、端口正确。')), 8000)
      const fail = (error: Error): void => {
        clearTimeout(timeout)
        this.connected = false
        this.message = error.message
        this.socket?.close()
        reject(error)
      }
      try {
        const socket = new WebSocket(this.url, 'obswebsocket.json')
        this.socket = socket
        this.identified = { resolve: () => { clearTimeout(timeout); this.connected = true; this.message = 'OBS WebSocket 已连接'; resolve(this.status()) }, reject: fail, timeout }
        socket.onmessage = (event) => this.receive(String(event.data))
        socket.onerror = () => { if (!this.connected) fail(new Error('无法连接 OBS WebSocket，请检查 OBS 是否已启动和密码是否正确。')) }
        socket.onclose = () => {
          if (!this.connected) fail(new Error('OBS WebSocket 已关闭'))
          this.connected = false
          this.virtualCameraActive = false
          this.message = 'OBS WebSocket 已断开'
          for (const pending of this.pending.values()) { clearTimeout(pending.timeout); pending.reject(new Error(this.message)) }
          this.pending.clear()
        }
      } catch (error) {
        fail(error instanceof Error ? error : new Error(String(error)))
      }
    })
  }

  async command(requestType: string, requestData: Record<string, unknown> = {}): Promise<Record<string, unknown>> {
    if (!this.connected || !this.socket || this.socket.readyState !== WebSocket.OPEN) throw new Error('请先连接 OBS WebSocket')
    const requestId = randomUUID()
    return new Promise((resolve, reject) => {
      const timeout = setTimeout(() => {
        this.pending.delete(requestId)
        reject(new Error(`OBS 请求 ${requestType} 超时`))
      }, 8000)
      this.pending.set(requestId, { resolve, reject, timeout })
      this.socket?.send(JSON.stringify({ op: 6, d: { requestType, requestId, requestData } }))
    })
  }

  async startVirtualCamera(): Promise<ObsStatus> {
    try {
      await this.command('StartVirtualCam')
    } catch (error) {
      if (!(error instanceof Error) || !/already running|output is active|already active/i.test(error.message)) throw error
    }
    this.virtualCameraActive = true
    this.message = 'OBS 虚拟摄像头已启动（输出当前 OBS 场景）'
    return this.status()
  }

  async stopVirtualCamera(): Promise<ObsStatus> {
    try {
      await this.command('StopVirtualCam')
    } catch (error) {
      if (!(error instanceof Error) || !/not running|not active/i.test(error.message)) throw error
    }
    this.virtualCameraActive = false
    this.message = 'OBS 虚拟摄像头已停止'
    return this.status()
  }

  disconnect(): void {
    const socket = this.socket
    this.socket = undefined
    this.connected = false
    this.virtualCameraActive = false
    if (this.identified) {
      clearTimeout(this.identified.timeout)
      this.identified.reject(new Error('OBS 连接已取消'))
      this.identified = undefined
    }
    for (const pending of this.pending.values()) { clearTimeout(pending.timeout); pending.reject(new Error('OBS 连接已关闭')) }
    this.pending.clear()
    if (socket && socket.readyState < WebSocket.CLOSING) socket.close()
  }

  status(): ObsStatus { return { connected: this.connected, url: this.url, message: this.message, virtualCameraActive: this.virtualCameraActive } }

  private receive(text: string): void {
    let message: ObsMessage
    try { message = JSON.parse(text) as ObsMessage } catch { return }
    if (message.op === 0) {
      const auth = message.d.authentication as { salt?: string; challenge?: string } | undefined
      const identify: Record<string, unknown> = { rpcVersion: Number(message.d.rpcVersion) || 1 }
      if (auth?.salt && auth.challenge) identify.authentication = createAuthentication(this.password, auth.salt, auth.challenge)
      this.socket?.send(JSON.stringify({ op: 1, d: identify }))
      return
    }
    if (message.op === 2) {
      this.identified?.resolve()
      this.identified = undefined
      return
    }
    if (message.op !== 7) return
    const requestId = String(message.d.requestId ?? '')
    const pending = this.pending.get(requestId)
    if (!pending) return
    this.pending.delete(requestId)
    clearTimeout(pending.timeout)
    const status = message.d.requestStatus as { result?: boolean; comment?: string } | undefined
    if (!status?.result) pending.reject(new Error(String(status?.comment ?? 'OBS 请求失败')))
    else pending.resolve((message.d.responseData as Record<string, unknown>) ?? {})
  }
}

function createAuthentication(password: string, salt: string, challenge: string): string {
  const secret = createHash('sha256').update(`${password}${salt}`).digest('base64')
  return createHash('sha256').update(`${secret}${challenge}`).digest('base64')
}
