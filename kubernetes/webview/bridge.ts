// Talking to the process through window.flint (site/extensions/protocol.md ▸ Webviews).
import { flint } from '@flint-ide/ui'

export const hasFlint = typeof window.flint === 'object' && window.flint !== null

/** flint.request, or a rejection when the page runs outside Flint. */
export function req<T = unknown>(method: string, params?: unknown): Promise<T> {
  return hasFlint ? flint().request<T>(method, params) : Promise.reject(new Error('Open this panel in Flint.'))
}

/** A toast in Flint. */
export function toast(text: string, type: 'info' | 'warning' | 'error' = 'info'): void {
  if (hasFlint) flint().showMessage(type, String(text).slice(0, 1000))
}

/** Messages the process posts to the panel (main.go, aws.go). */
export type PanelMessage =
  | { readonly type: 'refresh' }
  | { readonly type: 'settings'; readonly refreshSeconds: number }
  | { readonly type: 'visible'; readonly visible: boolean }
  | { readonly type: 'aws'; readonly profile: string; readonly ok: boolean; readonly error?: string; readonly needsInput?: boolean }

export function onPanelMessage(fn: (m: PanelMessage) => void): () => void {
  if (!hasFlint) return () => undefined
  return flint().onMessage((m) => {
    if (m && typeof m === 'object' && typeof (m as { type?: unknown }).type === 'string') fn(m as PanelMessage)
  })
}
