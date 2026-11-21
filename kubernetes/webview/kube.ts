// Pure helpers (Flint's frontend/src/features/kubernetes/kube.ts and lib/aws.ts).
import type { AwsProfile, Pod, Tone, Workload } from './types'

/** A readable context name: EKS ARNs become "cluster (account)", GKE names "cluster (project)". */
export function shortContext(name: string): string {
  const eks = /^arn:aws:eks:[^:]+:(\d+):cluster\/(.+)$/.exec(name)
  if (eks) return `${eks[2]} (${eks[1]})`
  const gke = /^gke_([^_]+)_[^_]+_(.+)$/.exec(name)
  if (gke) return `${gke[2]} (${gke[1]})`
  return name
}

/** "40s", "12m", "3h", "5d" since an RFC 3339 time. */
export function age(created: string, now: number): string {
  const t = Date.parse(created)
  if (Number.isNaN(t)) return ''
  const s = Math.max(0, Math.round((now - t) / 1000))
  if (s < 60) return `${s}s`
  if (s < 3600) return `${Math.floor(s / 60)}m`
  if (s < 86400) return `${Math.floor(s / 3600)}h`
  return `${Math.floor(s / 86400)}d`
}

export const resourceRef = (kind: string, name: string): string => `${kind.toLowerCase()}/${name}`

export function podTone(p: Pod): Tone {
  if (p.healthy) return 'ok'
  if (/BackOff|Err|Error|Failed|OOMKilled|Evicted|Invalid/i.test(p.status)) return 'bad'
  if (p.status === 'Completed' || p.status === 'Succeeded') return 'muted'
  return 'warn'
}

export function workloadTone(w: Workload): Tone {
  if (w.desired === 0) return 'muted'
  if (w.ready >= w.desired) return 'ok'
  return w.ready === 0 ? 'bad' : 'warn'
}

/** Kubectl failures that mean the cluster login (e.g. AWS) expired. */
export const isAuthError = (m: string): boolean =>
  /expired|ExpiredToken|InvalidClientTokenId|Unauthorized|You must be logged in|get-token|credentials|SSO session/i.test(m)

export function matches(query: string, ...fields: readonly (string | undefined | null)[]): boolean {
  const q = query.trim().toLowerCase()
  return !q || fields.some((f) => !!f && f.toLowerCase().includes(q))
}

export type Badge = { readonly text: string; readonly tone: Tone; readonly title: string }

export function rolloutBadge(w: Workload): Badge {
  const rev = w.revision ? `rev ${w.revision}` : ''
  switch (w.rollout) {
    case 'complete':
      return { text: rev ? `${rev} ✓` : '✓', tone: 'ok', title: 'Rolled out' }
    case 'progressing':
      return { text: 'rolling out', tone: 'warn', title: w.rolloutMessage || 'Rollout in progress' }
    case 'failed':
      return { text: 'failed', tone: 'bad', title: w.rolloutMessage || 'Rollout failed' }
    case 'paused':
      return { text: 'paused', tone: 'warn', title: 'Rollout paused' }
    case 'scaled-down':
      return { text: 'scaled to 0', tone: 'muted', title: 'No replicas' }
  }
  return { text: w.rollout || '', tone: 'muted', title: '' }
}

export const healthTone = (h: string): Tone => (h === 'ok' ? 'ok' : h === 'bad' ? 'bad' : 'warn')

const SOURCE_LABELS: Readonly<Record<string, string>> = { flux: 'Flux', argo: 'Argo CD', helm: 'Helm', label: 'label', workload: '' }
export const appSourceLabel = (s: string): string => (s in SOURCE_LABELS ? SOURCE_LABELS[s] : s)

export function shortRevision(r: string): string {
  const m = /^(?:(.+)@)?(?:sha1|sha256):([0-9a-f]{7})[0-9a-f]*$/.exec(r)
  return m ? (m[1] ? `${m[1]}@${m[2]}` : m[2]) : r
}

const SOON_MS = 15 * 60000

export function timeLeft(ms: number): string {
  const s = Math.max(0, Math.round(ms / 1000))
  if (s < 60) return `${s}s`
  const m = Math.floor(s / 60)
  if (m < 60) return `${m}m`
  const h = Math.floor(m / 60)
  if (h < 48) return `${h}h ${String(m % 60).padStart(2, '0')}m`
  return `${Math.floor(h / 24)}d`
}

export function profileStatus(p: AwsProfile, now: number): Badge {
  const expires = p.expiresAt ? Date.parse(p.expiresAt) : NaN
  if (!Number.isNaN(expires)) {
    const left = expires - now
    const at = new Date(expires).toLocaleString()
    if (left <= 0) return { text: 'expired', tone: 'bad', title: `Expired ${at}` }
    return { text: timeLeft(left), tone: left < SOON_MS ? 'warn' : 'ok', title: `Valid until ${at}` }
  }
  switch (p.status) {
    case 'logged-out':
      return { text: 'logged out', tone: 'bad', title: 'No cached login: log in first' }
    case 'static':
      return { text: 'keys', tone: 'muted', title: 'Long-term access keys (no expiry)' }
    default:
      return { text: '—', tone: 'muted', title: p.source ? `Uses the credentials of ${p.source}` : 'Checked when used' }
  }
}

export const errorText = (err: unknown): string =>
  String((err instanceof Error ? err.message : (err as { message?: string } | null)?.message) || err).slice(0, 2000)

export const plural = (n: number, word: string): string => `${n} ${word}${n === 1 ? '' : 's'}`

/** "D", "STS"… : the short kind shown before a row's name. */
export const workloadKind = (kind: string, short: boolean): string =>
  kind === 'Deployment' ? 'D' : kind === 'StatefulSet' ? (short ? 'S' : 'STS') : 'DS'
