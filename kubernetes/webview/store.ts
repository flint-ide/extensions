// The panel's state and the requests that fill it (Flint's store.ts). State is
// immutable: every change makes a new object; a ref holds the latest one so
// answers to requests can check whether the user switched away meanwhile.
import { useEffect, useMemo, useRef, useState } from 'react'
import { hasFlint, onPanelMessage, req, toast } from './bridge'
import { errorText, shortContext } from './kube'
import type { AwsProfile, KubeContext, Overview, Saved, View } from './types'

export type State = {
  readonly contexts: readonly KubeContext[]
  readonly contextsError: string | null
  readonly context: string
  readonly namespaces: readonly string[]
  readonly namespace: string
  readonly overview: Overview | null
  readonly loading: boolean
  readonly error: string | null
  readonly query: string
  readonly busy: ReadonlySet<string>
  readonly view: View
  readonly closed: ReadonlySet<string>
  readonly chosen: Readonly<Record<string, boolean>>
  readonly moreProblems: ReadonlySet<string>
  readonly now: number
  readonly aws: AwsProfile | null
  readonly awsRefreshing: ReadonlySet<string>
  readonly awsNeedsInput: ReadonlySet<string>
  readonly saved: Saved
  readonly refreshSeconds: number
  readonly visible: boolean
}

const initial: State = {
  contexts: [], contextsError: null, context: '', namespaces: [], namespace: '', overview: null,
  loading: false, error: null, query: '', busy: new Set(), view: 'apps',
  closed: new Set(), chosen: {}, moreProblems: new Set(), now: Date.now(),
  aws: null, awsRefreshing: new Set(), awsNeedsInput: new Set(),
  saved: { context: '', namespaces: {}, view: 'apps' }, refreshSeconds: 5, visible: true,
}

const AWS_EVERY_MS = 30000

export const withKey = (set: ReadonlySet<string>, key: string, on: boolean): ReadonlySet<string> => {
  const next = new Set(set)
  if (on) next.add(key)
  else next.delete(key)
  return next
}

type Update = (fn: (s: State) => State) => void

/** The panel's operations; get() is always the latest state. */
function createKube(get: () => State, update: Update) {
  const patch = (p: Partial<State>) => update((s) => ({ ...s, ...p }))
  const selectedContext = () => get().contexts.find((c) => c.name === get().context)
  const scope = () => ({ context: get().context, namespace: get().namespace })
  let inFlight: { key: string; id: number; done: Promise<void> } | null = null
  let requests = 0
  let awsReadAt = 0

  const saveState = (p: Partial<Saved>) => {
    const saved = { ...get().saved, ...p }
    patch({ saved })
    req('saveState', { state: saved }).catch(() => undefined) // not remembered: nothing breaks
  }

  /** Re-reads the shown namespace; an answer for a context or namespace switched away from is dropped. */
  const refreshOverview = (): Promise<void> => {
    const { context, namespace } = get()
    if (!context) return Promise.resolve()
    const key = `${context}\n${namespace}`
    if (inFlight && inFlight.key === key) return inFlight.done
    const id = ++requests
    const done = (async () => {
      patch({ loading: true })
      const current = () => get().context === context && get().namespace === namespace
      try {
        const overview = await req<Overview>('overview', { context, namespace })
        if (current()) patch({ overview, error: null })
      } catch (err) {
        if (current()) patch({ error: errorText(err) })
      } finally {
        if (inFlight?.id === id) {
          inFlight = null
          patch({ loading: false })
        }
      }
    })()
    inFlight = { key, id, done }
    return done
  }

  /** The AWS profile of an EKS context (its credential status), re-read now and then. */
  const loadAws = async () => {
    const profile = selectedContext()?.awsProfile
    awsReadAt = Date.now()
    if (!profile) return patch({ aws: null })
    try {
      const aws = await req<AwsProfile | null>('awsProfile', { profile })
      if (selectedContext()?.awsProfile === profile) patch({ aws: aws || null })
    } catch {
      patch({ aws: null })
    }
  }

  /** Shows another context (the kubeconfig's current context is left alone). */
  const selectContext = async (name: string) => {
    const ctx = get().contexts.find((c) => c.name === name)
    const namespace = get().saved.namespaces[name] || ctx?.namespace || 'default'
    patch({ context: name, namespace, namespaces: [], overview: null, error: null, aws: null })
    saveState({ context: name })
    if (!name) return
    void refreshOverview()
    void loadAws()
    try {
      const namespaces = (await req<string[] | null>('namespaces', { context: name })) || []
      if (get().context === name) patch({ namespaces })
    } catch {
      // listing namespaces may be forbidden: the context's own namespace still works
    }
  }

  /** Reads the kubeconfig; keeps the chosen context when it still exists, else the current one. */
  const loadContexts = async () => {
    try {
      const contexts = (await req<KubeContext[] | null>('contexts')) || []
      const s = get()
      const keep = contexts.some((c) => c.name === s.context) ? s.context : (contexts.find((c) => c.current) || contexts[0])?.name || ''
      patch({ contexts, contextsError: null })
      if (keep !== s.context || !s.namespace) await selectContext(keep)
      else void loadAws()
    } catch (err) {
      patch({ contextsError: errorText(err), contexts: [] })
    }
  }

  const selectNamespace = (namespace: string) => {
    saveState({ namespaces: { ...get().saved.namespaces, [get().context]: namespace } })
    patch({ namespace, overview: null, error: null })
    void refreshOverview()
  }

  const setBusy = (key: string, on: boolean) => update((s) => ({ ...s, busy: withKey(s.busy, key, on) }))

  /** Runs an action request with the row's busy mark, a toast and a refresh after it. */
  const act = async (key: string, method: string, params: object, done: string) => {
    setBusy(key, true)
    try {
      await req(method, params)
      toast(done)
    } catch (err) {
      toast(errorText(err), 'error')
    } finally {
      setBusy(key, false)
      void refreshOverview()
    }
  }

  /** Terminal tabs: the process builds and checks the kubectl argv and opens the tab. */
  const terminal = (method: string, params: object) => {
    req(method, { ...scope(), ...params }).catch((err) => toast(errorText(err), 'error'))
  }

  /** Makes the shown context the kubeconfig's current one (for every terminal and tool). */
  const useAsCurrent = async () => {
    const { context } = get()
    try {
      await req('useContext', { context })
      toast(`Current context: ${shortContext(context)}`)
      await loadContexts()
    } catch (err) {
      toast(errorText(err), 'error')
    }
  }

  const awsLogin = (p: AwsProfile) => {
    req('awsLogin', { profile: p.name }).catch((err) => toast(errorText(err), 'error'))
  }

  const awsRefresh = async (p: AwsProfile) => {
    if (get().awsRefreshing.has(p.name)) return
    update((s) => ({ ...s, awsNeedsInput: withKey(s.awsNeedsInput, p.name, false), awsRefreshing: withKey(s.awsRefreshing, p.name, true) }))
    try {
      await req('awsRefresh', { profile: p.name }) // ends with a { type: 'aws' } message
    } catch (err) {
      update((s) => ({ ...s, awsRefreshing: withKey(s.awsRefreshing, p.name, false) }))
      toast(errorText(err), 'error')
    }
  }

  const onAwsDone = (m: { profile: string; ok: boolean; error?: string; needsInput?: boolean }) => {
    update((s) => ({
      ...s,
      awsRefreshing: withKey(s.awsRefreshing, m.profile, false),
      awsNeedsInput: !m.ok && m.needsInput ? withKey(s.awsNeedsInput, m.profile, true) : s.awsNeedsInput,
    }))
    if (m.ok) {
      toast(`Refreshed ${m.profile}`)
      void refreshOverview()
    } else {
      toast(m.error || `Refreshing ${m.profile} failed`, 'error')
    }
    void loadAws()
  }

  /** Reloads everything now (the refresh button, the Refresh command). */
  const refreshAll = () => {
    patch({ now: Date.now() })
    void loadContexts()
    void refreshOverview()
  }

  const shown = () => get().visible && document.visibilityState === 'visible'
  const tick = () => {
    if (!shown()) return
    patch({ now: Date.now() })
    if (Date.now() - awsReadAt >= AWS_EVERY_MS) void loadAws()
    void refreshOverview()
  }

  const start = async () => {
    if (!hasFlint) return
    try {
      const [saved, settings] = await Promise.all([req<Partial<Saved> | null>('state'), req<{ refreshSeconds?: number } | null>('settings')])
      const view: View = saved?.view === 'resources' ? 'resources' : 'apps'
      const restored: Saved = { context: saved?.context || '', namespaces: saved?.namespaces || {}, view }
      const seconds = settings?.refreshSeconds
      patch({ saved: restored, context: restored.context, view, ...(Number.isInteger(seconds) && seconds! > 0 ? { refreshSeconds: seconds } : {}) })
    } catch (err) {
      toast(errorText(err), 'error')
    }
    await loadContexts()
  }

  return {
    patch, update, saveState, refreshOverview, loadAws, selectContext, loadContexts, selectNamespace, setBusy, act, terminal,
    useAsCurrent, awsLogin, awsRefresh, onAwsDone, refreshAll, shown, tick, start, scope, selectedContext,
  }
}

export type Kube = ReturnType<typeof createKube>

/** The state, the operations, and the timers and messages that drive them. */
export function useKube(): { state: State; kube: Kube } {
  const ref = useRef<State>(initial)
  const [state, setState] = useState<State>(initial)
  const kube = useMemo(
    () =>
      createKube(
        () => ref.current,
        (fn) => {
          ref.current = fn(ref.current)
          setState(ref.current)
        },
      ),
    [],
  )

  useEffect(() => {
    void kube.start()
    const onVisibility = () => {
      if (kube.shown()) kube.tick()
    }
    // Back from a login terminal: the AWS credentials may have changed.
    const onFocus = () => {
      if (ref.current.aws) void kube.loadAws()
    }
    document.addEventListener('visibilitychange', onVisibility)
    window.addEventListener('focus', onFocus)
    const off = onPanelMessage((m) => {
      switch (m.type) {
        case 'refresh':
          kube.refreshAll()
          break
        case 'settings':
          if (Number.isInteger(m.refreshSeconds) && m.refreshSeconds > 0) kube.patch({ refreshSeconds: m.refreshSeconds })
          break
        case 'visible': {
          const was = kube.shown()
          kube.patch({ visible: !!m.visible })
          if (!was && kube.shown()) kube.tick()
          break
        }
        case 'aws':
          kube.onAwsDone(m)
          break
      }
    })
    return () => {
      document.removeEventListener('visibilitychange', onVisibility)
      window.removeEventListener('focus', onFocus)
      off()
    }
  }, [kube])

  // Refresh while visible, every refreshSeconds (restarted when the setting changes).
  useEffect(() => {
    if (!hasFlint) return undefined
    const timer = setInterval(kube.tick, state.refreshSeconds * 1000)
    return () => clearInterval(timer)
  }, [kube, state.refreshSeconds])

  return { state, kube }
}
