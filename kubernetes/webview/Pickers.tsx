// The top of the panel: the header's buttons, the context and namespace
// pickers, the EKS login status, the view switch and the filter.
import {
  Button,
  Combobox,
  ComboboxEmpty,
  ComboboxInput,
  ComboboxInputGroup,
  ComboboxItem,
  ComboboxList,
  ComboboxPopup,
  ComboboxTrigger,
  IconButton,
  Input,
  Segment,
  Spinner,
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@flint-ide/ui'
import { RotateCw, Search, SquareTerminal, Star } from '@flint-ide/ui/icons'
import type { ReactElement } from 'react'
import { matches, profileStatus, shortContext } from './kube'
import type { Kube, State } from './store'
import type { KubeContext, View } from './types'

/** An icon button with a tooltip. */
function Tip({ tip, children }: { tip: string; children: ReactElement }) {
  return (
    <Tooltip>
      <TooltipTrigger render={children} />
      <TooltipContent maxWidth={240}>{tip}</TooltipContent>
    </Tooltip>
  )
}

export function Header({ state, kube, onTerminal }: { state: State; kube: Kube; onTerminal: () => void }) {
  return (
    <div className="k-header">
      <span className="k-title">Kubernetes</span>
      <Tip tip="Terminal in this context and namespace">
        <IconButton variant="ghost" size="sm" aria-label="Terminal in this context and namespace" disabled={!state.context} onClick={onTerminal}>
          <SquareTerminal />
        </IconButton>
      </Tip>
      <Tip tip="Refresh">
        <IconButton variant="ghost" size="sm" aria-label="Refresh" disabled={state.loading} onClick={kube.refreshAll}>
          <RotateCw />
        </IconButton>
      </Tip>
    </div>
  )
}

const contextHint = (c: KubeContext): string => [c.current && 'current', c.awsProfile && `aws:${c.awsProfile}`].filter(Boolean).join(' · ')

function ContextPicker({ state, kube }: { state: State; kube: Kube }) {
  const selected = state.contexts.find((c) => c.name === state.context) ?? null
  return (
    <Combobox<KubeContext>
      items={state.contexts}
      value={selected}
      onValueChange={(c: KubeContext | null) => {
        if (c && c.name !== state.context) void kube.selectContext(c.name)
      }}
      itemToStringLabel={(c) => shortContext(c.name)}
      isItemEqualToValue={(a, b) => a.name === b.name}
      filter={(c, q) => matches(q, shortContext(c.name), c.name, contextHint(c))}
    >
      <ComboboxInputGroup className="k-picker" title={state.context}>
        <ComboboxInput placeholder="Context" aria-label="Context" spellCheck={false} />
        <ComboboxTrigger />
      </ComboboxInputGroup>
      <ComboboxPopup className="k-picker-popup">
        <ComboboxEmpty>No matches</ComboboxEmpty>
        <ComboboxList>
          {(c: KubeContext) => (
            <ComboboxItem key={c.name} value={c} title={c.name}>
              <span className={`k-opt-label${c.name === state.context ? ' k-strong' : ''}`}>{shortContext(c.name)}</span>
              {contextHint(c) && <span className="k-opt-hint">{contextHint(c)}</span>}
            </ComboboxItem>
          )}
        </ComboboxList>
      </ComboboxPopup>
    </Combobox>
  )
}

/** A searchable list when the namespaces could be listed, else a field (Enter applies). */
function NamespacePicker({ state, kube }: { state: State; kube: Kube }) {
  if (state.namespaces.length === 0) {
    return (
      <Input
        key={`${state.context}\n${state.namespace}`}
        size="sm"
        className="k-ns-input"
        defaultValue={state.namespace}
        aria-label="Namespace"
        title="Namespace"
        spellCheck={false}
        onKeyDown={(e) => {
          if (e.key === 'Enter') kube.selectNamespace(e.currentTarget.value.trim())
        }}
      />
    )
  }
  return (
    <Combobox<string>
      items={state.namespaces}
      value={state.namespace}
      onValueChange={(ns: string | null) => {
        if (ns && ns !== state.namespace) kube.selectNamespace(ns)
      }}
    >
      <ComboboxInputGroup className="k-picker">
        <span className="k-picker-label">ns</span>
        <ComboboxInput placeholder="Namespace" aria-label="Namespace" spellCheck={false} />
        <ComboboxTrigger />
      </ComboboxInputGroup>
      <ComboboxPopup className="k-picker-popup">
        <ComboboxEmpty>No matches</ComboboxEmpty>
        <ComboboxList>
          {(ns: string) => (
            <ComboboxItem key={ns} value={ns}>
              <span className={`k-opt-label${ns === state.namespace ? ' k-strong' : ''}`}>{ns}</span>
            </ComboboxItem>
          )}
        </ComboboxList>
      </ComboboxPopup>
    </Combobox>
  )
}

/** The EKS context's AWS profile: its credential status and how to renew it. */
function AwsRow({ state, kube }: { state: State; kube: Kube }) {
  const aws = state.aws
  if (!aws) return null
  const st = profileStatus(aws, state.now)
  const bad = st.tone === 'bad' || st.tone === 'warn'
  let action: ReactElement | null = null
  if (state.awsRefreshing.has(aws.name)) {
    action = (
      <span className="k-muted k-inline">
        <Spinner className="k-spin" aria-hidden /> refreshing…
      </span>
    )
  } else if (bad && aws.refreshable && !state.awsNeedsInput.has(aws.name)) {
    action = (
      <Button variant="link" size="sm" className="k-link" title="Refresh the AWS credentials (no terminal)" onClick={() => void kube.awsRefresh(aws)}>
        Refresh
      </Button>
    )
  } else if (bad && aws.login && aws.login.length) {
    action = (
      <Button variant="link" size="sm" className="k-link" onClick={() => kube.awsLogin(aws)}>
        Log In
      </Button>
    )
  }
  return (
    <div className="k-aws" title={st.title}>
      <span className="k-muted">AWS {aws.name}</span>
      <span className={`k-status ${st.tone}`}>{st.text}</span>
      {action}
    </div>
  )
}

const VIEWS = [
  { value: 'apps', label: 'Apps' },
  { value: 'resources', label: 'Resources' },
]

export function Pickers({ state, kube }: { state: State; kube: Kube }) {
  const ctx = state.contexts.find((c) => c.name === state.context)
  const partial = state.overview?.partial || []
  return (
    <div className="k-pickers">
      <div className="k-line">
        <ContextPicker state={state} kube={kube} />
      </div>
      <div className="k-line">
        <div className="k-grow">
          <NamespacePicker state={state} kube={kube} />
        </div>
        {ctx && !ctx.current && (
          <Tip tip="Make this the kubeconfig's current context (all terminals and tools)">
            <IconButton variant="ghost" size="sm" aria-label="Make this the kubeconfig's current context" onClick={() => void kube.useAsCurrent()}>
              <Star />
            </IconButton>
          </Tip>
        )}
      </div>
      <AwsRow state={state} kube={kube} />
      <div className="k-line">
        <Segment
          size="xs"
          shape="rounded"
          aria-label="View"
          options={VIEWS}
          value={state.view}
          onValueChange={(v) => {
            const view: View = v === 'resources' ? 'resources' : 'apps'
            kube.patch({ view })
            kube.saveState({ view })
          }}
        />
        <Input
          size="sm"
          wrapperClassName="k-filter"
          placeholder="Filter"
          aria-label="Filter"
          spellCheck={false}
          leading={<Search />}
          value={state.query}
          onChange={(e) => kube.patch({ query: e.target.value })}
          onKeyDown={(e) => {
            if (e.key === 'Escape' && state.query) {
              e.preventDefault()
              kube.patch({ query: '' })
            }
          }}
        />
      </div>
      {partial.length > 0 && <div className="k-muted k-partial">Not shown (no access): {partial.join(', ')}</div>}
    </div>
  )
}
