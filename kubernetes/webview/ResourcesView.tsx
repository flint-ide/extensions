// The Resources view: workloads, pods, services and jobs, filtered by the query.
import type { ReactNode } from 'react'
import type { Actions } from './actions'
import { age, matches, podTone, workloadKind, workloadTone } from './kube'
import { jobMenu, resourcesPodMenu, resourcesWorkloadMenu, serviceMenu } from './menus'
import type { State } from './store'
import { Dot, Hint, Kind, Name, Row, Section, Status } from './Tree'
import { Notice } from './Notices'
import type { Job, Overview, Pod, Service, Workload } from './types'

type Props = { readonly ov: Overview; readonly state: State; readonly actions: Actions; readonly onToggle: (key: string) => void }

const jobTone = (status: string) => (/fail/.test(status) ? 'bad' : status === 'running' ? 'warn' : 'muted')

export function ResourcesView({ ov, state, actions: a, onToggle }: Props) {
  const q = state.query
  const workloads = (ov.workloads || []).filter((w) => matches(q, w.name, w.kind))
  const pods = (ov.pods || []).filter((p) => matches(q, p.name, p.status, p.owner))
  const services = (ov.services || []).filter((s) => matches(q, s.name, s.type))
  const jobs = (ov.jobs || []).filter((j) => matches(q, j.name, j.status))
  const open = (key: string) => !state.closed.has(key)
  const section = (key: string, title: string, count: number, rows: () => ReactNode) => (
    <Section title={title} count={count} open={open(key)} onToggle={() => onToggle(key)}>
      {rows()}
    </Section>
  )
  const empty = workloads.length + pods.length + services.length + jobs.length === 0

  return (
    <>
      {section('workloads', 'Workloads', workloads.length, () => workloads.map((w) => <WorkloadRow key={`${w.kind}/${w.name}`} w={w} state={state} a={a} />))}
      {section('pods', 'Pods', pods.length, () => pods.map((p) => <PodRow key={p.name} p={p} state={state} a={a} />))}
      {section('services', 'Services', services.length, () => services.map((s) => <ServiceRow key={s.name} s={s} a={a} />))}
      {section('jobs', 'Jobs', jobs.length, () => jobs.map((j) => <JobRow key={`${j.kind}/${j.name}`} j={j} now={state.now} a={a} />))}
      {!state.error && empty && <Notice>{q ? 'Nothing matches.' : `Namespace ${state.namespace} is empty.`}</Notice>}
    </>
  )
}

function WorkloadRow({ w, state, a }: { w: Workload; state: State; a: Actions }) {
  return (
    <Row
      depth={1}
      menu={resourcesWorkloadMenu(a, w)}
      onDoubleClick={() => a.workloadLogs(w)}
      title={`${w.kind} ${w.name}\n${w.ready}/${w.desired} ready, ${w.updated} updated\nDouble-click: logs`}
    >
      <Kind>{workloadKind(w.kind, true)}</Kind>
      <Name>{w.name}</Name>
      <Hint>{age(w.created, state.now)}</Hint>
      <Status tone={workloadTone(w)} busy={state.busy.has(`${w.kind}/${w.name}`)}>{`${w.ready}/${w.desired}`}</Status>
    </Row>
  )
}

function PodRow({ p, state, a }: { p: Pod; state: State; a: Actions }) {
  const tone = podTone(p)
  return (
    <Row
      depth={1}
      menu={resourcesPodMenu(a, p)}
      onDoubleClick={() => a.podLogs(p)}
      title={`${p.name}\n${p.status}, ${p.ready}/${p.total} ready, ${p.restarts} restarts${p.node ? `\nnode ${p.node}` : ''}\nDouble-click: logs`}
    >
      <Dot tone={tone} />
      <Name>{p.name}</Name>
      <Hint>{[p.restarts ? `↺${p.restarts}` : '', age(p.created, state.now)].filter(Boolean).join(' ')}</Hint>
      <Status tone={tone} busy={state.busy.has(`Pod/${p.name}`)}>{p.status === 'Running' ? `${p.ready}/${p.total}` : p.status}</Status>
    </Row>
  )
}

function ServiceRow({ s, a }: { s: Service; a: Actions }) {
  const ports = s.ports || []
  return (
    <Row depth={1} menu={serviceMenu(a, s, true)} title={`${s.type} ${s.clusterIP}\n${ports.map((p) => `${p.port} → ${p.target}/${p.protocol}`).join('\n')}`}>
      <Kind>SVC</Kind>
      <Name>{s.name}</Name>
      <Hint>{ports.map((p) => p.port).join(', ')}</Hint>
      <Status tone="muted">{s.type}</Status>
    </Row>
  )
}

function JobRow({ j, now, a }: { j: Job; now: number; a: Actions }) {
  return (
    <Row depth={1} menu={jobMenu(a, j)} title={j.schedule ? `schedule ${j.schedule}` : j.name}>
      <Kind>{j.kind === 'CronJob' ? 'CJ' : 'J'}</Kind>
      <Name>{j.name}</Name>
      <Hint>{j.schedule || age(j.created, now)}</Hint>
      <Status tone={jobTone(j.status)}>{j.status}</Status>
    </Row>
  )
}
