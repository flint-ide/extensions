// The Kubernetes panel: a webview page built with Flint's UI kit
// (go run ./cmd/flint-webview from a Flint checkout; see ../README.md ▸ Files).
import { syncFlintTheme } from '@flint-ide/ui'
import { createRoot } from 'react-dom/client'
import { useActions } from './actions'
import { AppsView } from './AppsView'
import { ConfirmProvider } from './Confirm'
import { Notices } from './Notices'
import { Header, Pickers } from './Pickers'
import { ResourcesView } from './ResourcesView'
import { RowMenuProvider } from './RowMenu'
import { useKube, withKey } from './store'
import './panel.css'

const TREE_ID = 'k-tree'

function Panel() {
  const { state, kube } = useKube()
  const actions = useActions(kube)
  const ov = state.overview
  const toggleSection = (key: string) => kube.update((s) => ({ ...s, closed: withKey(s.closed, key, !s.closed.has(key)) }))
  const toggleApp = (key: string, open: boolean) => kube.update((s) => ({ ...s, chosen: { ...s.chosen, [key]: open } }))
  const toggleMore = (key: string) => kube.update((s) => ({ ...s, moreProblems: withKey(s.moreProblems, key, !s.moreProblems.has(key)) }))

  return (
    <div className="k-panel">
      <Header state={state} kube={kube} onTerminal={actions.contextTerminal} />
      <Pickers state={state} kube={kube} />
      <div className="k-tree" id={TREE_ID}>
        <Notices state={state} kube={kube} />
        {ov && state.view === 'apps' && <AppsView ov={ov} state={state} actions={actions} onApp={toggleApp} onMore={toggleMore} />}
        {ov && state.view === 'resources' && <ResourcesView ov={ov} state={state} actions={actions} onToggle={toggleSection} />}
      </div>
    </div>
  )
}

// The page's own menus replace the browser's, except in text fields.
document.addEventListener('contextmenu', (e) => {
  if (!e.defaultPrevented && !(e.target instanceof HTMLInputElement)) e.preventDefault()
})

syncFlintTheme()
const root = document.getElementById('root')
if (root) {
  createRoot(root).render(
    <ConfirmProvider>
      <RowMenuProvider scroller={TREE_ID}>
        <Panel />
      </RowMenuProvider>
    </ConfirmProvider>,
  )
}
