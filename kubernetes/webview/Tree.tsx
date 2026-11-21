// Tree rows: a dense 22 px row with a twisty, a kind tag or a health dot, the
// name, a hint, a status pill and the row's actions (right-click or the "more"
// button that shows on hover).
import { BadgeCounter, Spinner } from '@flint-ide/ui'
import { ChevronDown, ChevronRight } from '@flint-ide/ui/icons'
import type { ReactNode } from 'react'
import { MoreButton, useContextMenu } from './RowMenu'
import type { MenuEntry, Tone } from './types'
import './tree.css'

type RowProps = {
  readonly depth: 0 | 1 | 2
  readonly title?: string
  readonly className?: string
  readonly menu?: readonly (MenuEntry | false)[]
  readonly onClick?: () => void
  readonly onDoubleClick?: () => void
  readonly expanded?: boolean
  readonly children: ReactNode
}

export function Row({ depth, title, className, menu, onClick, onDoubleClick, expanded, children }: RowProps) {
  const contextMenu = useContextMenu()
  const interactive = !!onClick
  return (
    <div
      className={`k-row k-depth${depth}${className ? ` ${className}` : ''}`}
      title={title}
      role={interactive ? 'button' : undefined}
      tabIndex={interactive ? 0 : undefined}
      aria-expanded={expanded}
      onClick={onClick}
      onDoubleClick={onDoubleClick}
      onKeyDown={
        interactive
          ? (e) => {
              if (e.key === 'Enter' || e.key === ' ') {
                e.preventDefault()
                onClick?.()
              }
            }
          : undefined
      }
      onContextMenu={menu ? contextMenu(menu) : undefined}
    >
      {children}
      {menu && <MoreButton entries={menu} />}
    </div>
  )
}

export const Twisty = ({ open }: { open: boolean }) => (
  <span className="k-twisty" aria-hidden>
    {open ? <ChevronDown /> : <ChevronRight />}
  </span>
)

export const Kind = ({ children }: { children: string }) => <span className="k-kind">{children}</span>

export const Dot = ({ tone }: { tone: Tone }) => <span className={`k-dot ${tone}`} aria-label={tone} />

export const Name = ({ children, strong }: { children: ReactNode; strong?: boolean }) => (
  <span className={`k-name${strong ? ' k-strong' : ''}`}>{children}</span>
)

export const Hint = ({ children }: { children: ReactNode }) => <span className="k-hint">{children}</span>

/** A status pill; busy shows a spinner while an action on the row runs. */
export const Status = ({ tone, busy, children }: { tone: Tone; busy?: boolean; children: ReactNode }) => (
  <span className={`k-status ${tone}`}>{busy ? <Spinner className="k-spin" aria-label="Working" /> : children}</span>
)

export const Problem = ({ tone, pod, children }: { tone: 'bad' | 'warn'; pod?: boolean; children: ReactNode }) => (
  <div className={`k-problem ${tone}${pod ? ' k-pod-problem' : ''}`}>{children}</div>
)

/** A collapsible section of the Resources view with its item count. */
export function Section({ title, count, open, onToggle, children }: { title: string; count: number; open: boolean; onToggle: () => void; children: ReactNode }) {
  if (count === 0) return null
  return (
    <div>
      <Row depth={0} className="k-section" onClick={onToggle} expanded={open}>
        <Twisty open={open} />
        <Name strong>{title}</Name>
        <BadgeCounter count={count} tone="neutral" className="k-count" />
      </Row>
      {open && children}
    </div>
  )
}
