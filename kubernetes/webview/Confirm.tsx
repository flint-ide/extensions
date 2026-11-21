// The panel's confirm dialog: confirm(message, label, danger) resolves true when the user confirms.
import {
  AlertDialog,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  Button,
} from '@flint-ide/ui'
import { createContext, type ReactNode, useCallback, useContext, useRef, useState } from 'react'

type Ask = { readonly message: string; readonly label: string; readonly danger: boolean; readonly resolve: (ok: boolean) => void }
type Confirm = (message: string, label: string, danger?: boolean) => Promise<boolean>

const ConfirmContext = createContext<Confirm>(() => Promise.resolve(false))

export const useConfirm = (): Confirm => useContext(ConfirmContext)

export function ConfirmProvider({ children }: { children: ReactNode }) {
  const [ask, setAsk] = useState<Ask | null>(null)
  const [open, setOpen] = useState(false)
  const action = useRef<HTMLButtonElement>(null)

  const confirm = useCallback<Confirm>(
    (message, label, danger = false) =>
      new Promise<boolean>((resolve) => {
        setAsk({ message, label, danger, resolve })
        setOpen(true)
      }),
    [],
  )

  // Resolves once: the action button and the dialog's own close (Cancel, Escape) both end up here.
  const pending = useRef<((ok: boolean) => void) | null>(null)
  pending.current = open ? (ask?.resolve ?? null) : null
  const answer = (ok: boolean) => {
    const resolve = pending.current
    pending.current = null
    resolve?.(ok)
    setOpen(false)
  }

  return (
    <ConfirmContext.Provider value={confirm}>
      {children}
      <AlertDialog open={open} onOpenChange={(next) => (next ? setOpen(true) : answer(false))} onOpenChangeComplete={(o) => (o ? undefined : setAsk(null))}>
        <AlertDialogContent initialFocus={action} className="k-confirm">
          <AlertDialogHeader>
            <AlertDialogTitle className="k-confirm-text">{ask?.message}</AlertDialogTitle>
          </AlertDialogHeader>
          <AlertDialogFooter layout="inline">
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <Button ref={action} size="sm" variant={ask?.danger ? 'destructive' : 'primary'} onClick={() => answer(true)}>
              {ask?.label}
            </Button>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </ConfirmContext.Provider>
  )
}
