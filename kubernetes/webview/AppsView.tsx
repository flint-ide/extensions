// The Apps view: each app with its health, problems, ingresses, services,
// workloads and their pods; unhealthy apps start open.
import { Badge, Button, Tooltip, TooltipContent, TooltipTrigger } from '@flint-ide/ui'
import { Sparkles } from '@flint-ide/ui/icons'
import type { Actions } from './actions'
import { appSourceLabel, healthTone, matches, plural, podTone, rolloutBadge, shortRevision, workloadKind } from './kube'
import { appPodMenu, appWorkloadMenu, ingressMenu, kustomizationMenu, serviceMenu } from './menus'
import { Notice } from './Notices'
import type { State } from './store'
import type { App, Ingress, Kustomization, Overview, Pod, Service, Tone, Workload } from './types'
import { Dot, Hint, Kind, Name, Problem, Row, Status, Twisty } from './Tree'

const MAX_PROBLEMS = 6
// Badge's colors are fixed hex values; the source tag follows Flint's accent instead.
const SOURCE_STYLE = { color: 'var(--accent)', background: 'color-mix(in srgb, var(--accent) 14%, transparent)', borderColor: 'transparent' }

type Toggles = { readonly onApp: (key: string, open: boolean) => void; readonly onMore: (key: string) => void }
type Props = { readonly ov: Overview; readonly state: State; readonly actions: Actions } & Toggles

type Index = {
  readonly workloads: ReadonlyMap<string, Workload>
  readonly services: ReadonlyMap<string, Service>
  readonly ingresses: ReadonlyMap<string, Ingress>
  readonly pods: ReadonlyMap<string, readonly Pod[]>
}

function index(ov: Overview): Index {
  const pods = (ov.pods || []).reduce((m, p) => (p.owner ? m.set(p.owner, [...(m.get(p.owner) || []), p]) : m), new Map<string, readonly Pod[]>())
  return {
    workloads: new Map((ov.workloads || []).map((w) => [`${w.kind}/${w.name}`, w])),
    services: new Map((ov.services || []).map((s) => [s.name, s])),
    ingresses: new Map((ov.ingresses || []).map((i) => [i.name, i])),
    pods,
  }
}

export function AppsView({ ov, state, actions, onApp, onMore }: Props) {
  const q = state.query
  const apps = (ov.apps || []).filter((a) => matches(q, a.name, ...(a.workloads || []), ...(a.services || []), ...(a.ingresses || [])))
  if (apps.length === 0) return <Notice>{q ? 'Nothing matches.' : 'No workloads in this namespace.'}</Notice>
  const idx = index(ov)
  return (
    <>
      {apps.map((a) => (
        <AppNode key={a.key} app={a} idx={idx} state={state} actions={actions} onApp={onApp} onMore={onMore} />
      ))}
    </>
  )
}

type NodeProps = { readonly app: App; readonly idx: Index; readonly state: State; readonly actions: Actions } & Toggles

function AppNode({ app, idx, state, actions: a, onApp, onMore }: NodeProps) {
  const open = app.key in state.chosen ? state.chosen[app.key] : app.health !== 'ok'
  const problemsAll = app.problems || []
  // A pod's own problem is shown under the pod; the summary keeps the rest.
  const podsWithProblems = (app.workloads || []).flatMap((ref) => idx.pods.get(ref.slice(ref.indexOf('/') + 1)) || []).filter((p) => p.problem)
  const listed = problemsAll
    .map((text, i) => ({ text, critical: i < app.critical }))
    .filter((p) => !podsWithProblems.some((pod) => p.text.startsWith(`${pod.name}: `) || p.text.startsWith(`Pod ${pod.name}: `)))
  const more = state.moreProblems.has(app.key)
  const problems = more ? listed : listed.slice(0, MAX_PROBLEMS)
  const source = appSourceLabel(app.source)
  const aiBusy = state.busy.has(`app:${app.key}`)

  return (
    <div>
      <Row depth={0} className="k-app" expanded={open} onClick={() => onApp(app.key, !open)} title={problemsAll.join('\n') || 'Healthy'}>
        <Twisty open={open} />
        <Dot tone={healthTone(app.health)} />
        <Name strong>{app.name}</Name>
        {source && (
          <Badge size="sm" badgeStyle="light" className="k-source" style={SOURCE_STYLE}>
            {source}
          </Badge>
        )}
        <Hint>{problemsAll.length ? plural(problemsAll.length, 'issue') : 'healthy'}</Hint>
        {app.health !== 'ok' && <AiButton busy={aiBusy} onClick={() => void a.diagnose(app)} />}
      </Row>
      {open && (
        <div className="k-app-body">
          {app.kustomization && <KustomizationRow k={app.kustomization} a={a} />}
          {problems.map((p, i) => (
            <Problem key={i} tone={p.critical ? 'bad' : 'warn'}>
              {p.text}
            </Problem>
          ))}
          {listed.length > MAX_PROBLEMS && (
            <Button variant="link" size="sm" className="k-more-problems" onClick={() => onMore(app.key)}>
              {more ? 'Show less' : `${listed.length - MAX_PROBLEMS} more…`}
            </Button>
          )}
          {(app.ingresses || []).map((name) => {
            const ing = idx.ingresses.get(name)
            return ing ? <IngressRows key={name} ing={ing} a={a} /> : null
          })}
          {(app.services || []).map((name) => {
            const s = idx.services.get(name)
            return s ? <AppServiceRow key={name} s={s} a={a} /> : null
          })}
          {(app.workloads || []).map((ref) => {
            const w = idx.workloads.get(ref)
            return w ? <AppWorkload key={ref} w={w} app={app} pods={idx.pods.get(w.name) || []} state={state} a={a} /> : null
          })}
        </div>
      )}
    </div>
  )
}

function AiButton({ busy, onClick }: { busy: boolean; onClick: () => void }) {
  return (
    <Tooltip>
      <TooltipTrigger
        render={
          <Button
            variant="ghost"
            size="sm"
            className="k-ai"
            pending={busy}
            onClick={(e) => {
              e.stopPropagation()
              if (!busy) onClick()
            }}
          />
        }
      >
        {!busy && <Sparkles data-icon="inline-start" />}AI
      </TooltipTrigger>
      <TooltipContent maxWidth={240}>Diagnose with AI: gathers the state and failing pods' logs (secrets redacted) into the agent chat for you to review and send</TooltipContent>
    </Tooltip>
  )
}

function KustomizationRow({ k, a }: { k: Kustomization; a: Actions }) {
  const tone: Tone = k.suspended ? 'warn' : k.ready === 'True' ? 'ok' : k.ready === 'False' ? 'bad' : 'warn'
  const text = k.suspended ? 'suspended' : k.ready === 'True' ? 'ready' : k.ready === 'False' ? 'not ready' : 'reconciling'
  return (
    <Row depth={1} menu={kustomizationMenu(a, k)} title={`${k.namespace}/${k.name}\n${k.message || ''}`}>
      <Kind>KS</Kind>
      <Name>{k.name}</Name>
      <Hint>{k.revision ? shortRevision(k.revision) : k.namespace}</Hint>
      <Status tone={tone}>{text}</Status>
    </Row>
  )
}

function IngressRows({ ing, a }: { ing: Ingress; a: Actions }) {
  const title = `Ingress ${ing.name}${ing.class ? ` (${ing.class})` : ''}${ing.address ? `\n${ing.address}` : ''}`
  return (
    <>
      {(ing.routes || []).map((r, i) => (
        <Row key={i} depth={1} menu={ingressMenu(a, ing)} title={title}>
          <Kind>ING</Kind>
          <Name>{`${r.host}${r.path || ''}`}</Name>
          <Hint>{`→ ${r.service}:${r.port}`}</Hint>
          <Status tone="muted">{ing.tls ? '🔒' : 'http'}</Status>
        </Row>
      ))}
    </>
  )
}

function AppServiceRow({ s, a }: { s: Service; a: Actions }) {
  const tone: Tone = s.backends === 0 ? 'muted' : s.ready === 0 ? 'bad' : s.ready < s.backends ? 'warn' : 'ok'
  return (
    <Row depth={1} menu={serviceMenu(a, s, false)} title={`${s.type} ${s.clusterIP}\n${s.ready} of ${s.backends} backing pods ready`}>
      <Kind>SVC</Kind>
      <Name>{s.name}</Name>
      <Hint>{(s.ports || []).map((p) => p.port).join(', ')}</Hint>
      <Status tone={tone}>{`${s.ready}/${s.backends}`}</Status>
    </Row>
  )
}

type WorkloadProps = { readonly w: Workload; readonly app: App; readonly pods: readonly Pod[]; readonly state: State; readonly a: Actions }

function AppWorkload({ w, app, pods, state, a }: WorkloadProps) {
  const badge = rolloutBadge(w)
  const images = w.images || []
  return (
    <>
      <Row depth={1} menu={appWorkloadMenu(a, w, app)} onDoubleClick={() => a.rolloutStatus(w)} title={`${w.kind} ${w.name}\n${badge.title}\n${images.join('\n')}`}>
        <Kind>{workloadKind(w.kind, false)}</Kind>
        <Name>{w.name}</Name>
        <Hint>{`${w.ready}/${w.desired} · ${images.map((i) => i.split('/').pop()).join(', ')}`}</Hint>
        <Status tone={badge.tone} busy={state.busy.has(`${w.kind}/${w.name}`)}>
          {badge.text}
        </Status>
      </Row>
      {pods.map((p) => (
        <AppPod key={p.name} p={p} state={state} a={a} />
      ))}
    </>
  )
}

function AppPod({ p, state, a }: { p: Pod; state: State; a: Actions }) {
  const tone = podTone(p)
  return (
    <>
      <Row depth={2} menu={appPodMenu(a, p)} onDoubleClick={() => a.podLogs(p)} title={`${p.name}${p.problem ? `\n${p.problem}` : ''}\nDouble-click: logs`}>
        <Dot tone={tone} />
        <Name>{p.name}</Name>
        <Hint>{[p.old && 'old revision', p.restarts ? `↺${p.restarts}` : ''].filter(Boolean).join(' · ')}</Hint>
        <Status tone={tone} busy={state.busy.has(`Pod/${p.name}`)}>
          {p.status === 'Running' ? `${p.ready}/${p.total}` : p.status}
        </Status>
      </Row>
      {p.problem && (
        <Problem tone="bad" pod>
          {p.problem}
        </Problem>
      )}
    </>
  )
}
