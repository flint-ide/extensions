import type { ContextMenuEntry } from '@flint-ide/ui'
// The shapes of the process's answers (internal/kube, internal/awsprof, state.go).

export type KubeContext = {
  readonly name: string
  readonly cluster: string
  readonly user: string
  readonly namespace?: string
  readonly current: boolean
  readonly auth: string
  readonly awsProfile?: string
}

export type Workload = {
  readonly kind: string
  readonly name: string
  readonly ready: number
  readonly desired: number
  readonly updated: number
  readonly available: number
  readonly created: string
  readonly images: readonly string[] | null
  readonly revision?: string
  readonly rollout: string
  readonly rolloutMessage?: string
  readonly app: string
}

export type Pod = {
  readonly name: string
  readonly status: string
  readonly healthy: boolean
  readonly ready: number
  readonly total: number
  readonly restarts: number
  readonly node?: string
  readonly owner?: string
  readonly ownerKind?: string
  readonly containers: readonly string[] | null
  readonly ports: readonly number[] | null
  readonly created: string
  readonly problem?: string
  readonly old?: boolean
}

export type ServicePort = { readonly port: number; readonly target: string; readonly protocol: string; readonly nodePort?: number }

export type Service = {
  readonly name: string
  readonly type: string
  readonly clusterIP: string
  readonly ports: readonly ServicePort[] | null
  readonly ready: number
  readonly backends: number
  readonly app: string
}

export type IngressRoute = { readonly host: string; readonly path: string; readonly service: string; readonly port: string }

export type Ingress = {
  readonly name: string
  readonly class?: string
  readonly routes: readonly IngressRoute[] | null
  readonly tls: boolean
  readonly address?: string
  readonly app: string
}

export type Job = { readonly kind: string; readonly name: string; readonly status: string; readonly schedule?: string; readonly created: string }

export type Kustomization = {
  readonly name: string
  readonly namespace: string
  readonly ready: string
  readonly revision?: string
  readonly message?: string
  readonly suspended?: boolean
}

export type App = {
  readonly key: string
  readonly name: string
  readonly source: string
  readonly kustomization?: Kustomization
  readonly ingresses: readonly string[] | null
  readonly services: readonly string[] | null
  readonly workloads: readonly string[] | null
  readonly health: string
  readonly problems: readonly string[] | null
  readonly critical: number
}

export type Overview = {
  readonly workloads: readonly Workload[] | null
  readonly pods: readonly Pod[] | null
  readonly services: readonly Service[] | null
  readonly ingresses: readonly Ingress[] | null
  readonly jobs: readonly Job[] | null
  readonly apps: readonly App[] | null
  readonly partial?: readonly string[] | null
}

export type AwsProfile = {
  readonly name: string
  readonly kind: string
  readonly status: string
  readonly region?: string
  readonly account?: string
  readonly source?: string
  readonly expiresAt?: string
  readonly login?: readonly string[]
  readonly refreshable: boolean
}

export type View = 'apps' | 'resources'

/** What the panel remembers (state.go). */
export type Saved = {
  readonly context: string
  readonly namespaces: Readonly<Record<string, string>>
  readonly view: View
}

export type Tone = 'ok' | 'warn' | 'bad' | 'muted'

/** A context menu entry, or a separator. */
/** A row menu entry: Flint's UI kit's context menu entry ('separator' between groups). */
export type MenuEntry = ContextMenuEntry
