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

interface ProjectState {
  sessions: ShellSession[];
  activeId: string | null;
}

interface ShellContextValue {
  sessions: ShellSession[];
  activeSessionId: string | null;
  setActiveSessionId: (id: string) => void;
  startSession: () => Promise<void>;
  closeSession: (id: string) => void;
  sendInput: (id: string, data: string) => void;
  resize: (id: string, cols: number, rows: number) => void;
  paneOpen: boolean;
  setPaneOpen: (open: boolean) => void;
}

const ShellContext = createContext<ShellContextValue | null>(null);

function getState(map: Map<string, ProjectState>, pid: string): ProjectState {
  return map.get(pid) ?? { sessions: [], activeId: null };
}

export function ShellRunProvider({ children }: { children: ReactNode }) {
  const { t } = useTranslation();
  const { project } = useCurrentProject();
  const projectIdRef = useRef<string | undefined>(undefined);
  projectIdRef.current = project?.id;

  const [allState, setAllState] = useState<Map<string, ProjectState>>(new Map());
  const [paneOpen, setPaneOpen] = useState(false);
  // Buffer chunks that arrive before the session is registered in allState
  const orphanChunksRef = useRef<Map<string, string[]>>(new Map());

  useEffect(() => {
    const cancelData = EventsOn('shell:data', (payload: { sessionId: string; data: string }) => {
      setAllState((prev) => {
        for (const [pid, { sessions, activeId }] of prev) {
          const idx = sessions.findIndex((s) => s.sessionId === payload.sessionId);
          if (idx !== -1) {
            const next = new Map(prev);
            const updated = [...sessions];
            updated[idx] = { ...sessions[idx], chunks: [...sessions[idx].chunks, payload.data] };
            next.set(pid, { sessions: updated, activeId });
            return next;
          }
        }
        // Session not registered yet — buffer the chunk
        const buf = orphanChunksRef.current.get(payload.sessionId) ?? [];
        orphanChunksRef.current.set(payload.sessionId, [...buf, payload.data]);
        return prev;
      });
    });

    const cancelExit = EventsOn('shell:exit', (payload: { sessionId: string; code: number }) => {
      setAllState((prev) => {
        for (const [pid, { sessions, activeId }] of prev) {
          const idx = sessions.findIndex((s) => s.sessionId === payload.sessionId);
          if (idx !== -1) {
            const next = new Map(prev);
            const updated = [...sessions];
            updated[idx] = { ...sessions[idx], exited: { code: payload.code } };
            next.set(pid, { sessions: updated, activeId });
            return next;
          }
        }
        return prev;
      });
    });

    return () => {
      cancelData();
      cancelExit();
    };
  }, []);

  const pid = project?.id ?? '';
  const { sessions, activeId: activeSessionId } = getState(allState, pid);

  const setActiveSessionId = useCallback((id: string) => {
    const p = projectIdRef.current;
    if (!p) return;
    setAllState((prev) => {
      const next = new Map(prev);
      next.set(p, { ...getState(prev, p), activeId: id });
      return next;
    });
  }, []);

  const startSession = useCallback(async () => {
    const p = projectIdRef.current;
    const workDir = project?.path ?? '';
    if (!p) return;
    try {
      const sessionId = await StartShell(workDir);
      const earlyChunks = orphanChunksRef.current.get(sessionId) ?? [];
      orphanChunksRef.current.delete(sessionId);
      setAllState((prev) => {
        const next = new Map(prev);
        const { sessions: existing } = getState(prev, p);
        next.set(p, {
          sessions: [...existing, { sessionId, workDir, chunks: earlyChunks }],
          activeId: sessionId,
        });
        return next;
      });
      setPaneOpen(true);
    } catch (err) {
      toastError({ title: t('shell.couldNotStart'), err });
    }
  }, [project?.path, t]);

  const closeSession = useCallback((id: string) => {
    const p = projectIdRef.current;
    if (!p) return;
    void StopShell(id).catch(() => {});
    setAllState((prev) => {
      const next = new Map(prev);
      const { sessions: existing, activeId: currentActive } = getState(prev, p);
      const remaining = existing.filter((s) => s.sessionId !== id);
      const newActive =
        currentActive === id
          ? (remaining.length > 0 ? remaining[remaining.length - 1].sessionId : null)
          : currentActive;
      next.set(p, { sessions: remaining, activeId: newActive });
      return next;
    });
  }, []);

  const sendInput = useCallback((id: string, data: string) => {
    void WriteToShell(id, data).catch(() => {});
  }, []);

  const resize = useCallback((id: string, cols: number, rows: number) => {
    void ResizeShell(id, cols, rows).catch(() => {});
  }, []);

  return (
    <ShellContext.Provider
      value={{ sessions, activeSessionId, setActiveSessionId, startSession, closeSession, sendInput, resize, paneOpen, setPaneOpen }}
    >
      {children}
    </ShellContext.Provider>
  );
}

export function useShellRun() {
  const ctx = useContext(ShellContext);
  if (!ctx) throw new Error('useShellRun must be used inside ShellRunProvider');
  return ctx;
}
