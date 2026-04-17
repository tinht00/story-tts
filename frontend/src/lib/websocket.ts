/**
 * WebSocket với auto-reconnect (exponential backoff).
 *
 * Features:
 * - Tự động reconnect khi connection bị drop
 * - Exponential backoff: 1s → 2s → 4s → 8s → 16s (max 30s)
 * - Maximum retry attempts (default: 10)
 * - Callbacks: onMessage, onBinary, onStateChange, onError
 * - Manual stop/resume controls
 */

type WebSocketState = 'connecting' | 'open' | 'closed' | 'error'

interface ReconnectWebSocketOptions {
  maxRetries?: number
  baseDelayMs?: number
  maxDelayMs?: number
  onMessage?: (data: string) => void
  onBinary?: (data: ArrayBuffer) => void
  onStateChange?: (state: WebSocketState) => void
  onError?: (error: Event) => void
}

export class ReconnectWebSocket {
  private ws: WebSocket | null = null
  private url: string
  private options: Required<Omit<ReconnectWebSocketOptions, 'onMessage' | 'onBinary' | 'onStateChange' | 'onError'>> &
    Pick<ReconnectWebSocketOptions, 'onMessage' | 'onBinary' | 'onStateChange' | 'onError'>

  private retryCount = 0
  private state: WebSocketState = 'closed'
  private manualClose = false
  private reconnectTimer: ReturnType<typeof setTimeout> | null = null

  constructor(url: string, options: ReconnectWebSocketOptions = {}) {
    this.url = url
    this.options = {
      maxRetries: options.maxRetries ?? 10,
      baseDelayMs: options.baseDelayMs ?? 1000,
      maxDelayMs: options.maxDelayMs ?? 30000,
      onMessage: options.onMessage,
      onBinary: options.onBinary,
      onStateChange: options.onStateChange,
      onError: options.onError
    }
  }

  connect() {
    this.manualClose = false
    this.openConnection()
  }

  private openConnection() {
    this.setState('connecting')

    try {
      this.ws = new WebSocket(this.url)
      this.ws.binaryType = 'arraybuffer'
    } catch (error) {
      this.handleError(error as Event)
      return
    }

    this.ws.onopen = () => {
      this.retryCount = 0
      this.setState('open')
    }

    this.ws.onmessage = (event) => {
      if (typeof event.data === 'string') {
        this.options.onMessage?.(event.data)
      } else {
        const buffer = event.data instanceof Blob ? event.data.arrayBuffer().then(buf => {
          this.options.onBinary?.(buf)
        }) : event.data as ArrayBuffer
        if (buffer instanceof ArrayBuffer) {
          this.options.onBinary?.(buffer)
        }
      }
    }

    this.ws.onerror = (error) => {
      this.handleError(error)
    }

    this.ws.onclose = () => {
      if (!this.manualClose && this.retryCount < this.options.maxRetries!) {
        this.scheduleReconnect()
      } else {
        this.setState(this.manualClose ? 'closed' : 'error')
      }
    }
  }

  private scheduleReconnect() {
    if (this.reconnectTimer) {
      clearTimeout(this.reconnectTimer)
    }

    const delay = Math.min(
      this.options.baseDelayMs! * Math.pow(2, this.retryCount),
      this.options.maxDelayMs!
    )

    console.log(`[ReconnectWebSocket] Reconnect attempt ${this.retryCount + 1}/${this.options.maxRetries} in ${delay}ms`)

    this.reconnectTimer = setTimeout(() => {
      this.retryCount++
      this.openConnection()
    }, delay)
  }

  private handleError(error: Event) {
    this.options.onError?.(error)
  }

  private setState(state: WebSocketState) {
    this.state = state
    this.options.onStateChange?.(state)
  }

  send(data: string | ArrayBuffer | Blob | ArrayBufferView) {
    if (this.ws && this.ws.readyState === WebSocket.OPEN) {
      this.ws.send(data)
    } else {
      console.warn('[ReconnectWebSocket] Cannot send: socket not open')
    }
  }

  close() {
    this.manualClose = true
    if (this.reconnectTimer) {
      clearTimeout(this.reconnectTimer)
      this.reconnectTimer = null
    }
    if (this.ws) {
      this.ws.close()
      this.ws = null
    }
    this.setState('closed')
  }

  getState(): WebSocketState {
    return this.state
  }

  getRetryCount(): number {
    return this.retryCount
  }

  isManualClose(): boolean {
    return this.manualClose
  }

  reset() {
    this.close()
    this.retryCount = 0
    this.manualClose = false
  }
}

/**
 * Helper để tạo WebSocket URL từ base URL và session ID.
 * Chuyển http(s):// thành ws(s)://.
 * Giữ path WebSocket đồng bộ với namespace /sessions của realtime service.
 */
export function buildWebSocketUrl(baseUrl: string, sessionId: string): string {
  const url = new URL(baseUrl)
  url.pathname = `/sessions/${sessionId}/stream`

  if (url.protocol === 'https:') {
    url.protocol = 'wss:'
  } else {
    url.protocol = 'ws:'
  }

  return url.toString()
}


