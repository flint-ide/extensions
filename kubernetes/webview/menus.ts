// The rows' context menus (Flint's KubePanel.tsx and KubeApps.tsx).
import type { Actions } from './actions'
import { resourceRef } from './kube'
import type { App, Ingress, Job, Kustomization, MenuEntry, Pod, Service, Workload } from './types'

type Entries = readonly (MenuEntry | false)[]

const scaleEntries = (a: Actions, w: Workload, withZero: boolean): Entries => [
  { label: `Scale to ${w.desired + 1}`, onSelect: () => void a.scaleWorkload(w, w.desired + 1) },
  w.desired > 0 && { label: `Scale to ${w.desired - 1}`, onSelect: () => void a.scaleWorkload(w, w.desired - 1) },
  withZero && w.desired > 1 && { label: 'Scale to 0', danger: true, onSelect: () => void a.scaleWorkload(w, 0) },
]

const portForwards = (a: Actions, p: Pod): Entries =>
  (p.ports || []).map((port) => ({ label: `Port Forward :${port}`, onSelect: () => a.portForward(`pod/${p.name}`, port) }))

export const resourcesWorkloadMenu = (a: Actions, w: Workload): Entries => [
  { label: 'Logs', hint: resourceRef(w.kind, w.name), onSelect: () => a.workloadLogs(w) },
  { label: 'Describe', onSelect: () => a.describe(w.kind, w.name) },
  { label: 'Rollout Restart', onSelect: () => void a.restartWorkload(w) },
  ...(w.kind === 'DaemonSet' ? [] : ['sep' as const, ...scaleEntries(a, w, true)]),
]

export function resourcesPodMenu(a: Actions, p: Pod): Entries {
  const containers = p.containers || []
  const shells: Entries =
    containers.length > 1
      ? containers.map((c) => ({ label: `Shell: ${c}`, onSelect: () => a.openShell(p, c) }))
      : [{ label: 'Shell', onSelect: () => a.openShell(p) }]
  return [
    { label: 'Logs', onSelect: () => a.podLogs(p) },
    ...shells,
    { label: 'Describe', onSelect: () => a.describe('Pod', p.name) },
    ...portForwards(a, p),
    'sep',
    { label: 'Delete Pod', danger: true, onSelect: () => void a.deletePod(p) },
  ]
}

/** withTargets: the Resources view shows where each port goes. */
export const serviceMenu = (a: Actions, s: Service, withTargets: boolean): Entries => [
  ...(s.ports || []).map((port) => ({
    label: `Port Forward :${port.port}`,
    hint: withTargets ? `→ ${port.target}` : undefined,
    onSelect: () => a.portForward(`service/${s.name}`, port.port),
  })),
  { label: 'Describe', onSelect: () => a.describe('Service', s.name) },
]

export const jobMenu = (a: Actions, j: Job): Entries => [{ label: 'Describe', onSelect: () => a.describe(j.kind, j.name) }]

export const ingressMenu = (a: Actions, ing: Ingress): Entries => [{ label: 'Describe', onSelect: () => a.describe('Ingress', ing.name) }]

export const kustomizationMenu = (a: Actions, k: Kustomization): Entries => [
  { label: 'Reconcile', hint: 'flux reconcile --with-source', onSelect: () => a.reconcile(k) },
  { label: 'Describe', onSelect: () => a.describe('kustomizations.kustomize.toolkit.fluxcd.io', k.name) },
]

export const appWorkloadMenu = (a: Actions, w: Workload, app: App): Entries => [
  ...(app.health !== 'ok' ? [{ label: 'Diagnose with AI', hint: 'agent chat', onSelect: () => void a.diagnose(app) }, 'sep' as const] : []),
  { label: 'Rollout Status', hint: 'kubectl rollout status', onSelect: () => a.rolloutStatus(w) },
  { label: 'Rollout History', onSelect: () => a.rolloutHistory(w) },
  { label: 'Logs', onSelect: () => a.workloadLogs(w) },
  { label: 'Describe', onSelect: () => a.describe(w.kind, w.name) },
  'sep',
  { label: 'Rollout Restart', onSelect: () => void a.restartWorkload(w) },
  { label: 'Undo Rollout', hint: 'previous revision', danger: true, onSelect: () => void a.undoRollout(w) },
  ...(w.kind === 'DaemonSet' ? [] : scaleEntries(a, w, false)),
]

export const appPodMenu = (a: Actions, p: Pod): Entries => [
  { label: 'Logs', onSelect: () => a.podLogs(p) },
  { label: 'Shell', onSelect: () => a.openShell(p) },
  { label: 'Describe', onSelect: () => a.describe('Pod', p.name) },
  ...portForwards(a, p),
  'sep',
  { label: 'Delete Pod', danger: true, onSelect: () => void a.deletePod(p) },
]
