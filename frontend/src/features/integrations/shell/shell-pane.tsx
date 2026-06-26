import { Terminal, useTerminal } from '@wterm/react';
import { Plus, RotateCcw, Square, Trash2, X } from 'lucide-react';
import { useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { cn } from '@/lib/utils';
import { ClipboardGetText } from '@/wailsjs/runtime/runtime';
import { useCSharpRun } from '../csharp/csharp-run-context';
import { useGodotRun } from '../godot/godot-run-context';
import { useNodejsRun } from '../nodejs/nodejs-run-context';
import { usePythonRun } from '../python/python-run-context';
import { useTaskfileRun } from '../taskfile/taskfile-run-context';
import { patchScrollToBottom } from '../terminal-scroll-fix';
import { TerminalScrollbar } from '../terminal-scrollbar';
import { TerminalView } from '../terminal-view';
import { useTerminalAutoscroll } from '../use-terminal-autoscroll';
import type { ShellSession } from './shell-context';
import { useShellRun } from './shell-context';

export function ShellPane() {
  const { sessions, activeSessionId, setActiveSessionId, activeKind, setActiveKind, startSession, closeSession, paneOpen, setPaneOpen, paneHeight: height, setPaneHeight: setHeight } = useShellRun();
  const { run: nodejsRun, isRunning: nodejsRunning, stop: nodejsStop, restart: nodejsRestart, clear: nodejsClear } = useNodejsRun();
  const { run: pythonRun, isRunning: pythonRunning, stop: pythonStop, restart: pythonRestart, clear: pythonClear } = usePythonRun();
  const { run: csharpRun, isRunning: csharpRunning, stop: csharpStop, restart: csharpRestart, clear: csharpClear } = useCSharpRun();
  const { run: taskfileRun, isRunning: taskfileRunning, stop: taskfileStop, restart: taskfileRestart, clear: taskfileClear } = useTaskfileRun();
  const { run: godotRun, isRunning: godotRunning, stop: godotStop, restart: godotRestart, clear: godotClear } = useGodotRun();
  const { t } = useTranslation();

  const prevNodejsRunId = useRef<string | undefined>(nodejsRun?.runId);
  useEffect(() => {
    if (nodejsRun && prevNodejsRunId.current !== nodejsRun.runId) {
      prevNodejsRunId.current = nodejsRun.runId;
      setActiveKind('nodejs');
      setPaneOpen(true);
    }
    if (!nodejsRun) {
      prevNodejsRunId.current = undefined;
    }
  }, [nodejsRun, setActiveKind, setPaneOpen]);

  const prevPythonRunId = useRef<string | undefined>(pythonRun?.runId);
  useEffect(() => {
    if (pythonRun && prevPythonRunId.current !== pythonRun.runId) {
      prevPythonRunId.current = pythonRun.runId;
      setActiveKind('python');
      setPaneOpen(true);
    }
    if (!pythonRun) {
      prevPythonRunId.current = undefined;
    }
  }, [pythonRun, setActiveKind, setPaneOpen]);

  const prevCsharpRunId = useRef<string | undefined>(csharpRun?.runId);
  useEffect(() => {
    if (csharpRun && prevCsharpRunId.current !== csharpRun.runId) {
      prevCsharpRunId.current = csharpRun.runId;
      setActiveKind('csharp');
      setPaneOpen(true);
    }
    if (!csharpRun) {
      prevCsharpRunId.current = undefined;
    }
  }, [csharpRun, setActiveKind, setPaneOpen]);

  const prevTaskfileRunId = useRef<string | undefined>(taskfileRun?.runId);
  useEffect(() => {
    if (taskfileRun && prevTaskfileRunId.current !== taskfileRun.runId) {
      prevTaskfileRunId.current = taskfileRun.runId;
      setActiveKind('taskfile');
      setPaneOpen(true);
    }
    if (!taskfileRun) {
      prevTaskfileRunId.current = undefined;
    }
  }, [taskfileRun, setActiveKind, setPaneOpen]);

  const prevGodotRunId = useRef<string | undefined>(godotRun?.runId);
  useEffect(() => {
    if (godotRun && prevGodotRunId.current !== godotRun.runId) {
      prevGodotRunId.current = godotRun.runId;
      setActiveKind('godot');
      setPaneOpen(true);
    }
    if (!godotRun) {
      prevGodotRunId.current = undefined;
    }
  }, [godotRun, setActiveKind, setPaneOpen]);

  useEffect(() => {
    if (activeKind === 'nodejs' && !nodejsRun) {
      setActiveKind(pythonRun ? 'python' : csharpRun ? 'csharp' : taskfileRun ? 'taskfile' : godotRun ? 'godot' : 'shell');
    }
    if (activeKind === 'python' && !pythonRun) {
      setActiveKind(nodejsRun ? 'nodejs' : csharpRun ? 'csharp' : taskfileRun ? 'taskfile' : godotRun ? 'godot' : 'shell');
    }
    if (activeKind === 'csharp' && !csharpRun) {
      setActiveKind(nodejsRun ? 'nodejs' : pythonRun ? 'python' : taskfileRun ? 'taskfile' : godotRun ? 'godot' : 'shell');
    }
    if (activeKind === 'taskfile' && !taskfileRun) {
      setActiveKind(nodejsRun ? 'nodejs' : pythonRun ? 'python' : csharpRun ? 'csharp' : godotRun ? 'godot' : 'shell');
    }
    if (activeKind === 'godot' && !godotRun) {
      setActiveKind(nodejsRun ? 'nodejs' : pythonRun ? 'python' : csharpRun ? 'csharp' : taskfileRun ? 'taskfile' : 'shell');
    }
    if (activeKind === 'shell' && !activeSessionId && sessions.length > 0) {
      setActiveSessionId(sessions[sessions.length - 1].sessionId);
    }
  }, [activeKind, activeSessionId, sessions, nodejsRun, pythonRun, csharpRun, taskfileRun, godotRun, setActiveKind, setActiveSessionId]);

  const onDragStart = (e: React.PointerEvent) => {
    e.preventDefault();
    const startY = e.clientY;
    const startH = height;
    const onMove = (ev: PointerEvent) => setHeight(Math.max(120, Math.min(800, startH - (ev.clientY - startY))));
    const onUp = () => {
      window.removeEventListener('pointermove', onMove);
      window.removeEventListener('pointerup', onUp);
    };
    window.addEventListener('pointermove', onMove);
    window.addEventListener('pointerup', onUp);
  };

  const hasAnything = sessions.length > 0 || !!nodejsRun || !!pythonRun || !!csharpRun || !!taskfileRun || !!godotRun;
  const visible = paneOpen && hasAnything;

  const activeShellSession = activeKind === 'shell' ? (sessions.find((s) => s.sessionId === activeSessionId) ?? sessions[0] ?? null) : null;

  if (!visible) {
    return null;
  }

  return (
    <div className="flex shrink-0 flex-col border-t border-border" style={{ height }}>
      <div className="h-1 w-full shrink-0 cursor-ns-resize bg-transparent hover:bg-primary/20 active:bg-primary/30" onPointerDown={onDragStart} />

      <div className="flex flex-1 overflow-hidden">
        {/* Terminal content */}
        <div className="flex flex-1 flex-col overflow-hidden">
          {activeKind === 'nodejs' && nodejsRun && (
            <>
              <div className="flex h-8 shrink-0 items-center gap-2 border-b border-border/50 px-3">
                {nodejsRunning ? (
                  <Badge variant="secondary" className="h-4 gap-1 bg-emerald-500/10 px-1.5 text-[10px] text-emerald-400">
                    <span className="size-1.5 animate-pulse rounded-full bg-emerald-500" />
                    {t('integrations.nodejs.running')}
                  </Badge>
                ) : nodejsRun.exited ? (
                  <Badge variant={nodejsRun.exited.code === 0 ? 'secondary' : 'destructive'} className={cn('h-4 px-1.5 text-[10px]', nodejsRun.exited.code === 0 && 'bg-emerald-500/10 text-emerald-400')}>
                    {nodejsRun.exited.code === 0 ? t('integrations.nodejs.exitOk') : t('integrations.nodejs.exitCode', { code: nodejsRun.exited.code })}
                  </Badge>
                ) : null}
                <div className="flex-1" />
                <Button size="icon" variant="ghost" className="size-6" onClick={() => void nodejsRestart()}>
                  <RotateCcw className="size-3" />
                </Button>
                {nodejsRunning && (
                  <Button size="icon" variant="ghost" className="size-6" onClick={() => void nodejsStop()}>
                    <Square className="size-3 text-destructive" />
                  </Button>
                )}
              </div>
              <div className="flex-1 overflow-hidden">
                <TerminalView runId={nodejsRun.runId} lines={nodejsRun.lines} />
              </div>
              {nodejsRun.exited?.error && <div className="shrink-0 px-3 py-1.5 font-mono text-xs text-red-400">{nodejsRun.exited.error}</div>}
            </>
          )}

          {activeKind === 'python' && pythonRun && (
            <>
              <div className="flex h-8 shrink-0 items-center gap-2 border-b border-border/50 px-3">
                {pythonRunning ? (
                  <Badge variant="secondary" className="h-4 gap-1 bg-emerald-500/10 px-1.5 text-[10px] text-emerald-400">
                    <span className="size-1.5 animate-pulse rounded-full bg-emerald-500" />
                    {t('integrations.python.running')}
                  </Badge>
                ) : pythonRun.exited ? (
                  <Badge variant={pythonRun.exited.code === 0 ? 'secondary' : 'destructive'} className={cn('h-4 px-1.5 text-[10px]', pythonRun.exited.code === 0 && 'bg-emerald-500/10 text-emerald-400')}>
                    {pythonRun.exited.code === 0 ? t('integrations.python.exitOk') : t('integrations.python.exitCode', { code: pythonRun.exited.code })}
                  </Badge>
                ) : null}
                <div className="flex-1" />
                <Button size="icon" variant="ghost" className="size-6" onClick={() => void pythonRestart()}>
                  <RotateCcw className="size-3" />
                </Button>
                {pythonRunning && (
                  <Button size="icon" variant="ghost" className="size-6" onClick={() => void pythonStop()}>
                    <Square className="size-3 text-destructive" />
                  </Button>
                )}
              </div>
              <div className="flex-1 overflow-hidden">
                <TerminalView runId={pythonRun.runId} lines={pythonRun.lines} />
              </div>
              {pythonRun.exited?.error && <div className="shrink-0 px-3 py-1.5 font-mono text-xs text-red-400">{pythonRun.exited.error}</div>}
            </>
          )}

          {activeKind === 'csharp' && csharpRun && (
            <>
              <div className="flex h-8 shrink-0 items-center gap-2 border-b border-border/50 px-3">
                {csharpRunning ? (
                  <Badge variant="secondary" className="h-4 gap-1 bg-emerald-500/10 px-1.5 text-[10px] text-emerald-400">
                    <span className="size-1.5 animate-pulse rounded-full bg-emerald-500" />
                    {t('integrations.csharp.running')}
                  </Badge>
                ) : csharpRun.exited ? (
                  <Badge variant={csharpRun.exited.code === 0 ? 'secondary' : 'destructive'} className={cn('h-4 px-1.5 text-[10px]', csharpRun.exited.code === 0 && 'bg-emerald-500/10 text-emerald-400')}>
                    {csharpRun.exited.code === 0 ? t('integrations.csharp.exitOk') : t('integrations.csharp.exitCode', { code: csharpRun.exited.code })}
                  </Badge>
                ) : null}
                <div className="flex-1" />
                <Button size="icon" variant="ghost" className="size-6" onClick={() => void csharpRestart()}>
                  <RotateCcw className="size-3" />
                </Button>
                {csharpRunning && (
                  <Button size="icon" variant="ghost" className="size-6" onClick={() => void csharpStop()}>
                    <Square className="size-3 text-destructive" />
                  </Button>
                )}
              </div>
              <div className="flex-1 overflow-hidden">
                <TerminalView runId={csharpRun.runId} lines={csharpRun.lines} />
              </div>
              {csharpRun.exited?.error && <div className="shrink-0 px-3 py-1.5 font-mono text-xs text-red-400">{csharpRun.exited.error}</div>}
            </>
          )}

          {activeKind === 'taskfile' && taskfileRun && (
            <>
              <div className="flex h-8 shrink-0 items-center gap-2 border-b border-border/50 px-3">
                {taskfileRunning ? (
                  <Badge variant="secondary" className="h-4 gap-1 bg-emerald-500/10 px-1.5 text-[10px] text-emerald-400">
                    <span className="size-1.5 animate-pulse rounded-full bg-emerald-500" />
                    {t('integrations.taskfile.running')}
                  </Badge>
                ) : taskfileRun.exited ? (
                  <Badge variant={taskfileRun.exited.code === 0 ? 'secondary' : 'destructive'} className={cn('h-4 px-1.5 text-[10px]', taskfileRun.exited.code === 0 && 'bg-emerald-500/10 text-emerald-400')}>
                    {taskfileRun.exited.code === 0 ? t('integrations.taskfile.exitOk') : t('integrations.taskfile.exitCode', { code: taskfileRun.exited.code })}
                  </Badge>
                ) : null}
                <div className="flex-1" />
                <Button size="icon" variant="ghost" className="size-6" onClick={() => void taskfileRestart()}>
                  <RotateCcw className="size-3" />
                </Button>
                {taskfileRunning && (
                  <Button size="icon" variant="ghost" className="size-6" onClick={() => void taskfileStop()}>
                    <Square className="size-3 text-destructive" />
                  </Button>
                )}
              </div>
              <div className="flex-1 overflow-hidden">
                <TerminalView runId={taskfileRun.runId} lines={taskfileRun.lines} />
              </div>
              {taskfileRun.exited?.error && <div className="shrink-0 px-3 py-1.5 font-mono text-xs text-red-400">{taskfileRun.exited.error}</div>}
            </>
          )}

          {activeKind === 'godot' && godotRun && (
            <>
              <div className="flex h-8 shrink-0 items-center gap-2 border-b border-border/50 px-3">
                {godotRunning ? (
                  <Badge variant="secondary" className="h-4 gap-1 bg-emerald-500/10 px-1.5 text-[10px] text-emerald-400">
                    <span className="size-1.5 animate-pulse rounded-full bg-emerald-500" />
                    {t('integrations.godot.running')}
                  </Badge>
                ) : godotRun.exited ? (
                  <Badge variant={godotRun.exited.code === 0 ? 'secondary' : 'destructive'} className={cn('h-4 px-1.5 text-[10px]', godotRun.exited.code === 0 && 'bg-emerald-500/10 text-emerald-400')}>
                    {godotRun.exited.code === 0 ? t('integrations.godot.exitOk') : t('integrations.godot.exitCode', { code: godotRun.exited.code })}
                  </Badge>
                ) : null}
                <div className="flex-1" />
                <Button size="icon" variant="ghost" className="size-6" onClick={() => void godotRestart()}>
                  <RotateCcw className="size-3" />
                </Button>
                {godotRunning && (
                  <Button size="icon" variant="ghost" className="size-6" onClick={() => void godotStop()}>
                    <Square className="size-3 text-destructive" />
                  </Button>
                )}
              </div>
              <div className="flex-1 overflow-hidden">
                <TerminalView runId={godotRun.runId} lines={godotRun.lines} />
              </div>
              {godotRun.exited?.error && <div className="shrink-0 px-3 py-1.5 font-mono text-xs text-red-400">{godotRun.exited.error}</div>}
            </>
          )}

          {activeKind === 'shell' && <div className="flex-1 overflow-hidden">{activeShellSession ? <ShellTerminal key={activeShellSession.sessionId} session={activeShellSession} /> : <div className="flex h-full items-center justify-center text-xs text-muted-foreground">{t('integrations.shell.newSession')}</div>}</div>}
        </div>

        {/* Session list */}
        <div className="flex w-40 shrink-0 flex-col border-l border-border/50 bg-muted/20">
          <div className="flex h-8 shrink-0 items-center justify-between border-b border-border/50 px-2">
            <span className="text-xs font-medium text-muted-foreground">{t('integrations.shell.title')}</span>
            <Button size="icon" variant="ghost" className="size-5" onClick={() => setPaneOpen(false)}>
              <X className="size-3" />
            </Button>
          </div>

          <div className="flex-1 overflow-y-auto py-1">
            {sessions.map((s) => {
              const label = s.workDir.split('/').filter(Boolean).pop() ?? 'shell';
              const isActive = activeKind === 'shell' && s.sessionId === (activeShellSession?.sessionId ?? null);
              return (
                <SessionItem
                  key={s.sessionId}
                  label={label}
                  active={isActive}
                  exited={s.exited}
                  onSelect={() => {
                    setActiveKind('shell');
                    setActiveSessionId(s.sessionId);
                  }}
                  onDelete={() => closeSession(s.sessionId)}
                />
              );
            })}
            {nodejsRun && (
              <SessionItem
                label={nodejsRun.scriptName}
                active={activeKind === 'nodejs'}
                running={nodejsRunning}
                exited={nodejsRun.exited}
                onSelect={() => setActiveKind('nodejs')}
                onDelete={() => {
                  nodejsClear();
                  setActiveKind('shell');
                }}
              />
            )}
            {pythonRun && (
              <SessionItem
                label={pythonRun.scriptName}
                active={activeKind === 'python'}
                running={pythonRunning}
                exited={pythonRun.exited}
                onSelect={() => setActiveKind('python')}
                onDelete={() => {
                  pythonClear();
                  setActiveKind('shell');
                }}
              />
            )}
            {csharpRun && (
              <SessionItem
                label={csharpRun.scriptName}
                active={activeKind === 'csharp'}
                running={csharpRunning}
                exited={csharpRun.exited}
                onSelect={() => setActiveKind('csharp')}
                onDelete={() => {
                  csharpClear();
                  setActiveKind('shell');
                }}
              />
            )}
            {taskfileRun && (
              <SessionItem
                label={taskfileRun.scriptName}
                active={activeKind === 'taskfile'}
                running={taskfileRunning}
                exited={taskfileRun.exited}
                onSelect={() => setActiveKind('taskfile')}
                onDelete={() => {
                  taskfileClear();
                  setActiveKind('shell');
                }}
              />
            )}
            {godotRun && (
              <SessionItem
                label={godotRun.scriptName}
                active={activeKind === 'godot'}
                running={godotRunning}
                exited={godotRun.exited}
                onSelect={() => setActiveKind('godot')}
                onDelete={() => {
                  godotClear();
                  setActiveKind('shell');
                }}
              />
            )}
          </div>

          <div className="shrink-0 border-t border-border/50 p-1">
            <Button size="sm" variant="ghost" className="h-7 w-full justify-start gap-1.5 text-xs text-muted-foreground hover:text-foreground" onClick={() => void startSession()}>
              <Plus className="size-3" />
              {t('integrations.shell.newSession')}
            </Button>
          </div>
        </div>
      </div>
    </div>
  );
}

function SessionItem({ label, active, running, exited, onSelect, onDelete }: { label: string; active: boolean; running?: boolean; exited?: { code: number } | null | undefined; onSelect: () => void; onDelete: () => void }) {
  const [hovered, setHovered] = useState(false);
  return (
    <div
      role="button"
      tabIndex={0}
      onClick={onSelect}
      onKeyDown={(e) => {
        if (e.key === 'Enter') {
          onSelect();
        }
      }}
      onMouseEnter={() => setHovered(true)}
      onMouseLeave={() => setHovered(false)}
      className={cn('flex cursor-pointer items-center gap-1.5 rounded-sm px-2 py-1 mx-1 text-xs', active ? 'bg-background text-foreground' : 'text-muted-foreground hover:bg-muted/50 hover:text-foreground')}
    >
      {running && <span className="size-1.5 shrink-0 animate-pulse rounded-full bg-emerald-500" />}
      {!running && exited?.code !== undefined && exited.code !== 0 && <span className="size-1.5 shrink-0 rounded-full bg-destructive" />}
      <span className="flex-1 truncate font-mono">{label}</span>
      {hovered && (
        <button
          type="button"
          className="shrink-0 rounded p-0.5 hover:bg-destructive/20 hover:text-destructive"
          onClick={(e) => {
            e.stopPropagation();
            onDelete();
          }}
        >
          <Trash2 className="size-3" />
        </button>
      )}
    </div>
  );
}

// Strip SOH/STX (\001/\002) and literal \[ \] emitted by bash/readline as PS1 markers.
const cleanChunk = (s: string) => s.replace(/[\x01\x02]|\\[[\]]/g, '');

function ShellTerminal({ session }: { session: ShellSession }) {
  const { sendInput, resize } = useShellRun();
  const { ref, write } = useTerminal();
  const outerRef = useRef<HTMLDivElement>(null);
  const [ready, setReady] = useState(false);
  const [scroller, setScroller] = useState<HTMLElement | null>(null);
  const countRef = useRef(0);
  const chunksRef = useRef(session.chunks);
  chunksRef.current = session.chunks;

  useTerminalAutoscroll(scroller);

  // biome-ignore lint/correctness/useExhaustiveDependencies: runs once on ready
  useEffect(() => {
    if (!ready) {
      return;
    }
    for (const chunk of chunksRef.current) {
      write(cleanChunk(chunk));
    }
    countRef.current = chunksRef.current.length;
  }, [ready]);

  const chunkCount = session.chunks.length;
  // biome-ignore lint/correctness/useExhaustiveDependencies: chunkCount is the trigger
  useEffect(() => {
    if (!ready) {
      return;
    }
    const slice = session.chunks.slice(countRef.current);
    for (const chunk of slice) {
      write(cleanChunk(chunk));
    }
    countRef.current = session.chunks.length;
  }, [chunkCount, ready]);

  // biome-ignore lint/correctness/useExhaustiveDependencies: stable refs
  useEffect(() => {
    const el = outerRef.current;
    if (!el) {
      return;
    }
    const ro = new ResizeObserver(() => {
      const inst = ref.current?.instance as { cols?: number; rows?: number } | null;
      if (inst?.cols && inst?.rows) {
        resize(session.sessionId, inst.cols, inst.rows);
      }
    });
    ro.observe(el);
    return () => ro.disconnect();
  }, []);

  // Intercept Tab in capture phase so WebKitGTK can't steal it for focus navigation.
  // Paste is read through Wails since WebKitGTK's DOM clipboardData is unreliable.
  // biome-ignore lint/correctness/useExhaustiveDependencies: stable refs
  useEffect(() => {
    const el = outerRef.current;
    if (!el || !ready) {
      return;
    }
    const onKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Tab') {
        e.preventDefault();
        e.stopPropagation();
        sendInput(session.sessionId, '\t');
        return;
      }
      if ((e.ctrlKey || e.metaKey) && !e.altKey && (e.key === 'v' || e.key === 'V')) {
        e.preventDefault();
        e.stopPropagation();
        void ClipboardGetText()
          .then((text) => {
            if (!text) {
              return;
            }
            const bridge = (ref.current?.instance as { bridge?: { bracketedPaste?: () => boolean } } | null)?.bridge;
            if (bridge?.bracketedPaste?.()) {
              sendInput(session.sessionId, `\x1b[200~${text.replace(/\x1b/g, '')}\x1b[201~`);
            } else {
              sendInput(session.sessionId, text);
            }
          })
          .catch(() => {});
      }
    };
    el.addEventListener('keydown', onKeyDown, { capture: true });
    return () => el.removeEventListener('keydown', onKeyDown, { capture: true });
  }, [ready]);

  return (
    <div ref={outerRef} className="relative h-full w-full" onClick={() => ref.current?.focus()}>
      <Terminal
        ref={ref}
        className="h-full w-full"
        autoResize
        onResize={() => {
          const inst = ref.current?.instance as { cols?: number; rows?: number } | null;
          if (inst?.cols && inst?.rows) {
            resize(session.sessionId, inst.cols, inst.rows);
          }
        }}
        onReady={() => {
          setReady(true);
          setScroller(patchScrollToBottom(ref.current?.instance));
          ref.current?.focus();
        }}
        onData={(data) => sendInput(session.sessionId, data)}
      />
      <TerminalScrollbar scroller={scroller} />
    </div>
  );
}
