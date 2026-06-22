import { createContext, type ReactNode, useCallback, useContext, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@/components/ui/button';
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog';

export type ConfirmOptions = {
  title: string;
  description?: ReactNode;
  confirmLabel?: string;
  cancelLabel?: string;
  destructive?: boolean;
};

type ConfirmFn = (options: ConfirmOptions) => Promise<boolean>;

const ConfirmContext = createContext<ConfirmFn | null>(null);

export function ConfirmProvider({ children }: { children: ReactNode }) {
  const { t } = useTranslation();
  const [options, setOptions] = useState<ConfirmOptions | null>(null);
  const resolveRef = useRef<((value: boolean) => void) | null>(null);
  const queueRef = useRef<Array<{ options: ConfirmOptions; resolve: (value: boolean) => void }>>([]);
  const handoffPendingRef = useRef(false);

  const confirm = useCallback<ConfirmFn>((opts) => {
    return new Promise<boolean>((resolve) => {
      if (resolveRef.current || handoffPendingRef.current) {
        queueRef.current.push({ options: opts, resolve });
        return;
      }
      resolveRef.current = resolve;
      setOptions(opts);
    });
  }, []);

  const settle = useCallback((value: boolean) => {
    const currentResolve = resolveRef.current;
    if (!currentResolve) {
      return;
    }
    resolveRef.current = null;
    currentResolve(value);

    const next = queueRef.current.shift();
    setOptions(null);
    if (next) {
      // Defer so a second click in the same frame hits the null ref above instead of resolving the queued request.
      handoffPendingRef.current = true;
      queueMicrotask(() => {
        handoffPendingRef.current = false;
        resolveRef.current = next.resolve;
        setOptions(next.options);
      });
    }
  }, []);

  return (
    <ConfirmContext.Provider value={confirm}>
      {children}
      <Dialog
        open={options !== null}
        onOpenChange={(open) => {
          if (!open) {
            settle(false);
          }
        }}
      >
        {options && (
          <DialogContent showCloseButton={false} className="sm:max-w-md">
            <DialogHeader>
              <DialogTitle>{options.title}</DialogTitle>
              {options.description && <DialogDescription>{options.description}</DialogDescription>}
            </DialogHeader>
            <DialogFooter>
              <Button variant="outline" onClick={() => settle(false)}>
                {options.cancelLabel ?? t('common.cancel')}
              </Button>
              <Button variant={options.destructive ? 'destructive' : 'default'} onClick={() => settle(true)} autoFocus>
                {options.confirmLabel ?? t('common.confirm')}
              </Button>
            </DialogFooter>
          </DialogContent>
        )}
      </Dialog>
    </ConfirmContext.Provider>
  );
}

export function useConfirm(): ConfirmFn {
  const ctx = useContext(ConfirmContext);
  if (!ctx) {
    throw new Error('useConfirm must be used within a ConfirmProvider');
  }
  return ctx;
}
