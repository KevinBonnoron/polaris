import { Terminal, useTerminal } from '@wterm/react';
import { Plus, X } from 'lucide-react';
import { useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@/components/ui/button';
import { ScrollArea } from '@/components/ui/scroll-area';
import { cn } from '@/lib/utils';
import type { ShellSession } from './shell-context';
import { useShellRun } from './shell-context';

export function ShellPane() {
  const { sessions, activeSessionId, setActiveSessionId, startSession, closeSession, paneOpen, setPaneOpen } = useShellRun();
  const { t } = useTranslation();
  const [height, setHeight] = useState(280);

  const onDragStart = (e: React.PointerEvent) => {
    e.preventDefault();
    const startY = e.clientY;
    const startH = height;
    const onMove = (ev: PointerEvent) => {
      setHeight(Math.max(120, Math.min(800, startH - (ev.clientY - startY))));
    };
    const onUp = () => {
      window.removeEventListener('pointermove', onMove);
      window.removeEventListener('pointerup', onUp);
    };
    window.addEventListener('pointermove', onMove);
    window.addEventListener('pointerup', onUp);
  };

  const activeSession = sessions.find((s) => s.sessionId === activeSessionId) ?? sessions[0] ?? null;

  if (!paneOpen || sessions.length === 0) return null;

  return (
    <div className="flex shrink-0 flex-col border-t border-border" style={{ height }}>
      <div className="h-1 w-full shrink-0 cursor-ns-resize bg-transparent hover:bg-primary/20 active:bg-primary/30" onPointerDown={onDragStart} />

      {/* Tab bar */}
      <div className="flex h-9 shrink-0 items-center gap-1 border-b border-border/50 bg-muted/30 px-2">
        <span className="mr-1 text-xs font-medium text-muted-foreground">{t('shell.title')}</span>
        <div className="flex flex-1 items-center gap-0.5 overflow-x-auto">
          {sessions.map((s) => (
            <ShellTab
              key={s.sessionId}
              session={s}
              active={s.sessionId === (activeSession?.sessionId ?? null)}
              onSelect={() => setActiveSessionId(s.sessionId)}
              onClose={() => closeSession(s.sessionId)}
            />
          ))}
        </div>
        <Button size="icon" variant="ghost" className="size-6 shrink-0" onClick={() => void startSession()}>
          <Plus className="size-3" />
        </Button>
        <Button size="icon" variant="ghost" className="size-6 shrink-0" onClick={() => setPaneOpen(false)}>
          <X className="size-3" />
        </Button>
      </div>

      {/* Active terminal */}
      <div className="flex-1 overflow-hidden">
        {activeSession && (
          <ShellTerminal key={activeSession.sessionId} session={activeSession} />
        )}
      </div>
    </div>
  );
}

function ShellTab({ session, active, onSelect, onClose }: { session: ShellSession; active: boolean; onSelect: () => void; onClose: () => void }) {
  const label = session.workDir.split('/').filter(Boolean).pop() ?? session.sessionId.slice(0, 6);
  return (
    <button
      type="button"
      onClick={onSelect}
      className={cn(
        'flex h-6 items-center gap-1 rounded px-2 text-xs transition-colors',
        active ? 'bg-background text-foreground shadow-sm' : 'text-muted-foreground hover:bg-muted/50 hover:text-foreground',
        session.exited && 'opacity-60',
      )}
    >
      <span className="max-w-[120px] truncate font-mono">{label}</span>
      {session.exited && <span className="text-muted-foreground">·{session.exited.code}</span>}
      <span
        role="button"
        tabIndex={0}
        onClick={(e) => { e.stopPropagation(); onClose(); }}
        onKeyDown={(e) => { if (e.key === 'Enter') { e.stopPropagation(); onClose(); } }}
        className="ml-0.5 rounded p-0.5 hover:bg-destructive/20 hover:text-destructive"
      >
        <X className="size-2.5" />
      </span>
    </button>
  );
}

function ShellTerminal({ session }: { session: ShellSession }) {
  const { sendInput, resize } = useShellRun();
  const { ref, write } = useTerminal();
  const outerRef = useRef<HTMLDivElement>(null);
  const [ready, setReady] = useState(false);
  const countRef = useRef(0);
  const chunksRef = useRef(session.chunks);
  chunksRef.current = session.chunks;

  const scrollToBottom = () => {
    const viewport = outerRef.current?.querySelector('[data-slot="scroll-area-viewport"]') as HTMLElement | null;
    if (viewport) viewport.scrollTop = viewport.scrollHeight;
  };

  // biome-ignore lint/correctness/useExhaustiveDependencies: runs once on ready
  useEffect(() => {
    if (!ready) return;
    const initial = chunksRef.current;
    for (const chunk of initial) write(chunk);
    countRef.current = initial.length;
    requestAnimationFrame(scrollToBottom);
  }, [ready]);

  const chunkCount = session.chunks.length;
  // biome-ignore lint/correctness/useExhaustiveDependencies: chunkCount is the trigger
  useEffect(() => {
    if (!ready) return;
    const slice = session.chunks.slice(countRef.current);
    for (const chunk of slice) write(chunk);
    countRef.current = session.chunks.length;
    requestAnimationFrame(scrollToBottom);
  }, [chunkCount, ready]);

  // biome-ignore lint/correctness/useExhaustiveDependencies: stable refs
  useEffect(() => {
    const el = outerRef.current;
    if (!el) return;
    const ro = new ResizeObserver(() => {
      const wt = ref.current?.instance as { _measureCharSize?: () => { charWidth: number; rowHeight: number } | null; _rowHeight?: number } | null;
      if (!wt) return;
      const charSize = wt._measureCharSize?.() ?? null;
      const charW = charSize?.charWidth ?? 8;
      const rowH = charSize?.rowHeight ?? wt._rowHeight ?? 17;
      const { width, height } = el.getBoundingClientRect();
      const cols = Math.max(1, Math.floor(width / charW));
      const rows = Math.max(1, Math.floor(height / rowH));
      ref.current?.resize(cols, rows);
      resize(session.sessionId, cols, rows);
    });
    ro.observe(el);
    return () => ro.disconnect();
  }, []);

  return (
    <div ref={outerRef} className="h-full w-full">
      <ScrollArea className="h-full">
        <Terminal
          ref={ref}
          className="w-full"
          autoResize={false}
          rows={1}
          onReady={(wt) => {
            const wtAny = wt as unknown as { _measureCharSize?: () => { charWidth: number; rowHeight: number } | null; _rowHeight?: number };
            const charSize = wtAny._measureCharSize?.() ?? null;
            const charW = charSize?.charWidth ?? 8;
            const rowH = charSize?.rowHeight ?? wtAny._rowHeight ?? 17;
            const { width, height } = outerRef.current?.getBoundingClientRect() ?? { width: 0, height: 0 };
            const cols = Math.max(1, Math.floor(width / charW));
            const rows = Math.max(1, Math.floor(height / rowH));
            wt.resize(cols, rows);
            resize(session.sessionId, cols, rows);
            setReady(true);
          }}
          onData={(data) => sendInput(session.sessionId, data)}
        />
      </ScrollArea>
    </div>
  );
}
