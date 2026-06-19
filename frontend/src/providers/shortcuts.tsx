import { createContext, type ReactNode, useCallback, useContext, useEffect, useState } from 'react';
import { DEFAULT_SHORTCUTS, detectMac, fromBackend, type ShortcutDef, type ShortcutId, toBackend } from '@/lib/shortcuts';
import { GetShortcutsSettings, UpdateShortcutsSettings } from '@/wailsjs/go/main/App';

interface ShortcutsState {
  shortcuts: Record<ShortcutId, ShortcutDef>;
  isMac: boolean;
  modifierSymbol: string;
  isRecording: boolean;
  setRecording: (value: boolean) => void;
  updateShortcut: (id: ShortcutId, patch: Pick<ShortcutDef, 'key' | 'meta'>) => void;
  resetShortcut: (id: ShortcutId) => void;
}

const ShortcutsContext = createContext<ShortcutsState | null>(null);

function persist(shortcuts: Record<ShortcutId, ShortcutDef>) {
  UpdateShortcutsSettings(toBackend(shortcuts)).catch(() => {});
}

export function ShortcutsProvider({ children }: { children: ReactNode }) {
  const isMac = detectMac();
  const [shortcuts, setShortcuts] = useState<Record<ShortcutId, ShortcutDef>>({ ...DEFAULT_SHORTCUTS });
  const [isRecording, setRecording] = useState(false);

  useEffect(() => {
    GetShortcutsSettings()
      .then((s) => setShortcuts(fromBackend(s)))
      .catch(() => {});
  }, []);

  const updateShortcut = useCallback((id: ShortcutId, patch: Pick<ShortcutDef, 'key' | 'meta'>) => {
    setShortcuts((prev) => {
      const next = { ...prev, [id]: { id, ...patch } };
      persist(next);
      return next;
    });
  }, []);

  const resetShortcut = useCallback((id: ShortcutId) => {
    setShortcuts((prev) => {
      const next = { ...prev, [id]: DEFAULT_SHORTCUTS[id] };
      persist(next);
      return next;
    });
  }, []);

  return (
    <ShortcutsContext.Provider
      value={{
        shortcuts,
        isMac,
        modifierSymbol: isMac ? '⌘' : 'Ctrl',
        isRecording,
        setRecording,
        updateShortcut,
        resetShortcut,
      }}
    >
      {children}
    </ShortcutsContext.Provider>
  );
}

export function useShortcuts(): ShortcutsState {
  const ctx = useContext(ShortcutsContext);
  if (!ctx) {
    throw new Error('useShortcuts must be used inside <ShortcutsProvider>');
  }

  return ctx;
}
