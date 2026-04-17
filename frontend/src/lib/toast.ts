/**
 * Toast notification system.
 *
 * Usage:
 *   import { toast } from './toast'
 *   toast.success('Import thành công!')
 *   toast.error('Có lỗi xảy ra')
 *   toast.warning('Cảnh báo...')
 *   toast.info('Thông tin...')
 */

export type ToastType = 'success' | 'error' | 'warning' | 'info'

export interface Toast {
  id: string
  type: ToastType
  message: string
  duration: number  // ms, 0 = no auto-dismiss
  createdAt: number
}

class ToastManager {
  private toasts: Toast[] = []
  private listeners: Array<(toasts: Toast[]) => void> = []
  private idCounter = 0

  /**
   * Show a toast notification.
   * @returns Toast ID for manual dismissal
   */
  show(type: ToastType, message: string, duration = 5000): string {
    const id = `toast-${++this.idCounter}`
    const toast: Toast = {
      id,
      type,
      message,
      duration,
      createdAt: Date.now()
    }

    this.toasts = [...this.toasts, toast]
    this.notify()

    // Auto-dismiss
    if (duration > 0) {
      setTimeout(() => {
        this.dismiss(id)
      }, duration)
    }

    return id
  }

  success(message: string, duration = 5000): string {
    return this.show('success', message, duration)
  }

  error(message: string, duration = 8000): string {
    return this.show('error', message, duration)
  }

  warning(message: string, duration = 6000): string {
    return this.show('warning', message, duration)
  }

  info(message: string, duration = 4000): string {
    return this.show('info', message, duration)
  }

  /**
   * Dismiss a toast by ID.
   */
  dismiss(id: string): void {
    this.toasts = this.toasts.filter(t => t.id !== id)
    this.notify()
  }

  /**
   * Dismiss all toasts.
   */
  clear(): void {
    this.toasts = []
    this.notify()
  }

  /**
   * Get current toasts.
   */
  getToasts(): Toast[] {
    return [...this.toasts]
  }

  /**
   * Subscribe to toast changes.
   * @returns Unsubscribe function
   */
  subscribe(listener: (toasts: Toast[]) => void): () => void {
    this.listeners.push(listener)
    return () => {
      this.listeners = this.listeners.filter(l => l !== listener)
    }
  }

  private notify(): void {
    for (const listener of this.listeners) {
      listener([...this.toasts])
    }
  }
}

// Singleton instance
export const toast = new ToastManager()
