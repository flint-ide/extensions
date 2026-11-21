// Row actions: one DropdownMenu shared by every row, opened at the pointer by a
// right-click or under a row's "more" button. WunderUI has no context menu, so the
// menu anchors to an invisible button moved to the point where it should open.
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuShortcut, DropdownMenuTrigger, IconButton } from '@flint-ide/ui'
import { MoreHorizontal } from '@flint-ide/ui/icons'
import { createContext, type MouseEvent, type ReactNode, useCallback, useContext, useEffect, useState } from 'react'
import type { MenuEntry } from './types'

type Open = (entries: readonly MenuEntry[], x: number, y: number) => void
type MenuState = { readonly entries: readonly MenuEntry[]; readonly x: number; readonly y: number; readonly open: boolean }

const MenuContext = createContext<Open>(() => undefined)

const visible = (entries: readonly (MenuEntry | false | null | undefined)[]): readonly MenuEntry[] =>
  entries.filter((e): e is MenuEntry => !!e)

/** onContextMenu handler that opens the entries at the pointer. */
export function useContextMenu(): (entries: readonly (MenuEntry | false | null | undefined)[]) => (e: MouseEvent) => void {
  const open = useContext(MenuContext)
  return useCallback(
    (entries) => (e) => {
      e.preventDefault()
      e.stopPropagation()
      open(visible(entries), e.clientX, e.clientY)
    },
    [open],
  )
}

/** The "more" button at the end of a row: the same menu, under the button. */
export function MoreButton({ entries }: { entries: readonly (MenuEntry | false | null | undefined)[] }) {
  const open = useContext(MenuContext)
  return (
    <IconButton
      className="k-more"
      variant="ghost"
      size="sm"
      aria-label="Actions"
      onClick={(e) => {
        e.stopPropagation()
        const r = e.currentTarget.getBoundingClientRect()
        open(visible(entries), r.right - 4, r.bottom)
      }}
      onDoubleClick={(e) => e.stopPropagation()}
    >
      <MoreHorizontal />
    </IconButton>
  )
}

export function RowMenuProvider({ children, scroller }: { children: ReactNode; scroller: string }) {
  const [menu, setMenu] = useState<MenuState>({ entries: [], x: 0, y: 0, open: false })
  const open = useCallback<Open>((entries, x, y) => {
    if (entries.length > 0) setMenu({ entries, x, y, open: true })
  }, [])
  const close = useCallback(() => setMenu((m) => (m.open ? { ...m, open: false } : m)), [])

  // Like a native context menu: it closes when the tree scrolls or the window loses focus.
  useEffect(() => {
    const el = document.getElementById(scroller)
    el?.addEventListener('scroll', close)
    window.addEventListener('blur', close)
    window.addEventListener('resize', close)
    return () => {
      el?.removeEventListener('scroll', close)
      window.removeEventListener('blur', close)
      window.removeEventListener('resize', close)
    }
  }, [close, scroller])

  return (
    <MenuContext.Provider value={open}>
      {children}
      <DropdownMenu open={menu.open} onOpenChange={(o) => (o ? undefined : close())}>
        <DropdownMenuTrigger
          render={<button type="button" tabIndex={-1} aria-hidden className="k-menu-anchor" style={{ left: menu.x, top: menu.y }} />}
        />
        <DropdownMenuContent className="k-menu" sideOffset={2}>
          {menu.entries.map((entry, i) =>
            entry === 'sep' ? (
              <DropdownMenuSeparator key={`sep${i}`} />
            ) : (
              <DropdownMenuItem key={`${i}:${entry.label}`} variant={entry.danger ? 'destructive' : 'default'} onClick={entry.onSelect}>
                {entry.label}
                {entry.hint && <DropdownMenuShortcut>{entry.hint}</DropdownMenuShortcut>}
              </DropdownMenuItem>
            ),
          )}
        </DropdownMenuContent>
      </DropdownMenu>
    </MenuContext.Provider>
  )
}
