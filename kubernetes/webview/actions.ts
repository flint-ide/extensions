// What the rows do: requests to the process, confirmed first where they hurt.
import { useMemo } from 'react'
import { req, toast } from './bridge'
import { useConfirm } from './Confirm'
import { errorText, resourceRef, shortContext } from './kube'
import type { Kube } from './store'
import type { App, Kustomization, Pod, Workload } from './types'

export function useActions(kube: Kube) {
  const confirm = useConfirm()
  return useMemo(() => {
    const { scope, act, terminal } = kube
    const openLogs = (target: string) => terminal('logs', { target })
    const openShell = (p: Pod, container?: string) => terminal('exec', { pod: p.name, container: container || '' })
    const describe = (kind: string, name: string) => terminal('describe', { kind, name })
    const portForward = (target: string, port: number) => terminal('portForward', { target, port })
    const rolloutStatus = (w: Workload) => terminal('rolloutStatus', { kind: w.kind, name: w.name })
    const rolloutHistory = (w: Workload) => terminal('rolloutHistory', { kind: w.kind, name: w.name })
    const contextTerminal = () => terminal('shell', {})
    // reconcile: the namespace is the Kustomization's own (often flux-system).
    const reconcile = (k: Kustomization) => {
      req('reconcile', { context: scope().context, name: k.name, namespace: k.namespace }).catch((err) => toast(errorText(err), 'error'))
    }

    const restartWorkload = async (w: Workload) => {
      const s = scope()
      if (!(await confirm(`Restart all pods of ${w.kind} ${w.name} in ${shortContext(s.context)}?`, 'Restart'))) return
      await act(`${w.kind}/${w.name}`, 'restart', { ...s, kind: w.kind, name: w.name }, `Restarting ${w.name}`)
    }
    const scaleWorkload = async (w: Workload, replicas: number) => {
      const s = scope()
      if (replicas === 0 && !(await confirm(`Scale ${w.name} to 0? All its pods stop.`, 'Scale to 0', true))) return
      await act(`${w.kind}/${w.name}`, 'scale', { ...s, kind: w.kind, name: w.name, replicas }, `${w.name} scaled to ${replicas}`)
    }
    const deletePod = async (p: Pod) => {
      const s = scope()
      if (!(await confirm(`Delete pod ${p.name}?${p.owner ? ` ${p.owner} starts a new one.` : ''}`, 'Delete', true))) return
      await act(`Pod/${p.name}`, 'deletePod', { ...s, name: p.name }, `Deleting ${p.name}`)
    }
    const undoRollout = async (w: Workload) => {
      const s = scope()
      const from = w.revision ? ` from revision ${w.revision}` : ''
      if (!(await confirm(`Roll ${w.kind} ${w.name} back${from} to the previous revision in ${shortContext(s.context)}?`, 'Undo Rollout', true))) return
      await act(`${w.kind}/${w.name}`, 'undo', { ...s, kind: w.kind, name: w.name }, `Rolling back ${w.name}`)
    }

    /** Gathers the app's state and failing pods' logs (redacted by the process) into the agent chat's draft. */
    const diagnose = async (app: App) => {
      const key = `app:${app.key}`
      kube.setBusy(key, true)
      try {
        await req('diagnose', { ...scope(), app: app.key, name: app.name })
        toast('Diagnosis ready in the agent chat — review it and press Enter to send')
      } catch (err) {
        toast(errorText(err), 'error')
      } finally {
        kube.setBusy(key, false)
      }
    }

    return {
      openLogs, openShell, describe, portForward, rolloutStatus, rolloutHistory, contextTerminal, reconcile,
      restartWorkload, scaleWorkload, deletePod, undoRollout, diagnose,
      workloadLogs: (w: Workload) => openLogs(resourceRef(w.kind, w.name)),
      podLogs: (p: Pod) => openLogs(`pod/${p.name}`),
    }
  }, [kube, confirm])
}

export type Actions = ReturnType<typeof useActions>
