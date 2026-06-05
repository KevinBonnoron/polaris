import { type ReactNode, createContext, useCallback, useContext, useEffect, useRef, useState } from 'react';
import { ResizeShell, StartShell, StopShell, WriteToShell } from '@/wailsjs/go/main/App';
import { EventsOn } from '@/wailsjs/runtime/runtime';
import { toastError } from '@/lib/toast-error';
import { useCurrentProject } from '@/state/projects';
import { useTranslation } from 'react-i18next';

export interface ShellSession {
  sessionId: string;
  workDir: string;
  chunks: string[];
  exited?: { code: number };
}

type TerminalKind = 'shell' | 'nodejs' | 'python';

interface ShellContextValue {
  sessions: ShellSession[];
  activeSessionId: string | null;
  setActiveSessionId: (id: string) => void;
  activeKind: TerminalKind;
  setActiveKind: (kind: TerminalKind) => void;
  startSession: () => Promise<void>;
  closeSession: (id: string) => void;
  sendInput: (id: string, data: string) => void;
  resize: (id: string, cols: number, rows: number) => void;
  paneOpen: boolean;
  setPaneOpen: (open: boolean) => void;
  paneHeight: number;
  setPaneHeight: (h: number) => void;
}

const ShellContext = createContext<ShellContextValue | null>(null);

export function ShellRunProvider({ children }: { children: ReactNode }) {
  const { t } = useTranslation();
  const { project } = useCurrentProject();
  const projectRef = useRef(project);
  projectRef.current = project;

  const [sessions, setSessions] = useState<ShellSession[]>([]);
  const [activeSessionId, setActiveSessionIdRaw] = useState<string | null>(null);
  const [activeKind, setActiveKind] = useState<TerminalKind>('shell');
  const [paneOpen, setPaneOpen] = useState(false);
  const [paneHeight, setPaneHeight] = useState(280);
  const orphanChunksRef = useRef<Map<string, string[]>>(new Map());

  useEffect(() => {
    const cancelData = EventsOn('shell:data', (payload: { sessionId: string; data: string }) => {
      setSessions((prev) => {
        const idx = prev.findIndex((s) => s.sessionId === payload.sessionId);
        if (idx !== -1) {
          const updated = [...prev];
          updated[idx] = { ...prev[idx], chunks: [...prev[idx].chunks, payload.data] };
          return updated;
        }
        const buf = orphanChunksRef.current.get(payload.sessionId) ?? [];
        orphanChunksRef.current.set(payload.sessionId, [...buf, payload.data]);
        return prev;
      });
    });

    const cancelExit = EventsOn('shell:exit', (payload: { sessionId: string; code: number }) => {
      setSessions((prev) => {
        const idx = prev.findIndex((s) => s.sessionId === payload.sessionId);
        if (idx !== -1) {
          const updated = [...prev];
          updated[idx] = { ...prev[idx], exited: { code: payload.code } };
          return updated;
        }
        return prev;
      });
    });

    return () => {
      cancelData();
      cancelExit();
    };
  }, []);

  const setActiveSessionId = useCallback((id: string) => {
    setActiveSessionIdRaw(id);
  }, []);

  const startSession = useCallback(async () => {
    const workDir = projectRef.current?.path ?? '';
    try {
      const sessionId = await StartShell(workDir);
      const earlyChunks = orphanChunksRef.current.get(sessionId) ?? [];
      orphanChunksRef.current.delete(sessionId);
      setSessions((prev) => [...prev, { sessionId, workDir, chunks: earlyChunks }]);
      setActiveSessionIdRaw(sessionId);
      setActiveKind('shell');
      setPaneOpen(true);
    } catch (err) {
      toastError({ title: t('integrations.shell.couldNotStart'), err });
    }
  }, [t]);

  const closeSession = useCallback((id: string) => {
    void StopShell(id).catch(() => {});
    setSessions((prev) => {
      const remaining = prev.filter((s) => s.sessionId !== id);
      setActiveSessionIdRaw((current) => {
        if (current !== id) {
          return current;
        }
        return remaining.length > 0 ? remaining[remaining.length - 1].sessionId : null;
      });
      return remaining;
    });
  }, []);

  const sendInput = useCallback((id: string, data: string) => {
    void WriteToShell(id, data).catch(() => {});
  }, []);

  const resize = useCallback((id: string, cols: number, rows: number) => {
    void ResizeShell(id, cols, rows).catch(() => {});
  }, []);

  return <ShellContext.Provider value={{ sessions, activeSessionId, setActiveSessionId, activeKind, setActiveKind, startSession, closeSession, sendInput, resize, paneOpen, setPaneOpen, paneHeight, setPaneHeight }}>{children}</ShellContext.Provider>;
}

export function useShellRun() {
  const ctx = useContext(ShellContext);
  if (!ctx) {
    throw new Error('useShellRun must be used inside ShellRunProvider');
  }
  return ctx;
}
