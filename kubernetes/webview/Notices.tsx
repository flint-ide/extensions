// What the tree shows instead of (or above) rows: errors, loading, empty states.
import { Alert, AlertDescription, Button, EmptyState, Skeleton } from '@flint-ide/ui'
import { Boxes, Unplug } from '@flint-ide/ui/icons'
import type { ReactNode } from 'react'
import { hasFlint } from './bridge'
import { isAuthError } from './kube'
import type { Kube, State } from './store'

/** A quiet line in the tree ("Nothing matches."). */
export const Notice = ({ children }: { children: ReactNode }) => <div className="k-notice">{children}</div>

function ErrorAlert({ text, action }: { text: string; action?: ReactNode }) {
  return (
    <Alert variant="negative" className="k-alert">
      <AlertDescription className="k-alert-text">{text}</AlertDescription>
      {action}
    </Alert>
  )
}

/** The overview's error; under an expired cluster login, a button that renews it. */
function OverviewError({ state, kube }: { state: State; kube: Kube }) {
  const { aws, error } = state
  if (!error) return null
  const fixable = !!aws && isAuthError(error) && (aws.refreshable || (aws.login?.length ?? 0) > 0)
  const refreshing = !!aws && state.awsRefreshing.has(aws.name)
  const fix =
    fixable && aws ? (
      <Button
        variant="link"
        size="sm"
        className="k-alert-action"
        pending={refreshing}
        onClick={() => (aws.refreshable && !state.awsNeedsInput.has(aws.name) ? void kube.awsRefresh(aws) : kube.awsLogin(aws))}
      >
        {refreshing ? `Refreshing AWS credentials (${aws.name})…` : `Refresh AWS credentials (${aws.name})`}
      </Button>
    ) : undefined
  return <ErrorAlert text={error} action={fix} />
}

function Loading() {
  return (
    <div className="k-loading" aria-label="Loading" role="status">
      {[72, 56, 64, 48, 60].map((w, i) => (
        <Skeleton key={i} variant="text" style={{ width: `${w}%` }} />
      ))}
    </div>
  )
}

export function Notices({ state, kube }: { state: State; kube: Kube }) {
  return (
    <>
      {!hasFlint && <EmptyState icon={<Unplug />} title="Open this panel in Flint." variant="default" className="k-empty" />}
      {state.contextsError && <ErrorAlert text={state.contextsError} />}
      <OverviewError state={state} kube={kube} />
      {!state.overview && !state.error && state.context && state.loading && <Loading />}
      {!state.contextsError && hasFlint && state.contexts.length === 0 && !state.context && !state.loading && (
        <EmptyState icon={<Boxes />} title="No contexts in the kubeconfig." variant="default" className="k-empty" />
      )}
    </>
  )
}
