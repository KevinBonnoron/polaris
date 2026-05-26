import { RotateCcw, Square, X } from 'lucide-react';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { cn } from '@/lib/utils';
import { TerminalView } from '../terminal-view';
import { usePythonRun } from './python-run-context';

export function PythonTerminal() {
  const { t } = useTranslation();
  const { run, isRunning, terminalOpen, setTerminalOpen, stop, restart, clear } = usePythonRun();
  const [height, setHeight] = useState(240);

  const onDragStart = (e: React.PointerEvent) => {
    e.preventDefault();
    const startY = e.clientY;
    const startH = height;
    const onMove = (ev: PointerEvent) => {
      setHeight(Math.max(100, Math.min(600, startH - (ev.clientY - startY))));
    };
    const onUp = () => {
      window.removeEventListener('pointermove', onMove);
      window.removeEventListener('pointerup', onUp);
    };
    window.addEventListener('pointermove', onMove);
    window.addEventListener('pointerup', onUp);
  };

  if (!run || !terminalOpen) {
    return null;
  }

  return (
    <div className="flex shrink-0 flex-col border-t border-border" style={{ height }}>
      <div className="h-1 w-full shrink-0 cursor-ns-resize bg-transparent hover:bg-primary/20 active:bg-primary/30" onPointerDown={onDragStart} />
      <div className="flex h-9 shrink-0 items-center gap-2 border-b border-border/50 bg-muted/30 px-3">
        <span className="text-xs font-medium text-muted-foreground">{t('integrations.python.terminal')}</span>
        <span className="truncate text-xs font-mono">{run.scriptName}</span>

        {isRunning && (
          <Badge variant="secondary" className="h-4 gap-1 bg-emerald-500/10 px-1.5 text-[10px] text-emerald-400">
            <span className="size-1.5 animate-pulse rounded-full bg-emerald-500" />
            {t('integrations.python.running')}
          </Badge>
        )}
        {run.exited && (
          <Badge variant={run.exited.code === 0 ? 'secondary' : 'destructive'} className={cn('h-4 px-1.5 text-[10px]', run.exited.code === 0 && 'bg-emerald-500/10 text-emerald-400')}>
            {run.exited.code === 0 ? t('integrations.python.exitOk') : t('integrations.python.exitCode', { code: run.exited.code })}
          </Badge>
        )}

        <div className="flex-1" />

        <Button size="icon" variant="ghost" onClick={() => void restart()} className="size-6">
          <RotateCcw className="size-3" />
        </Button>
        {isRunning && (
          <Button size="icon" variant="ghost" onClick={() => void stop()} className="size-6">
            <Square className="size-3 text-destructive" />
          </Button>
        )}
        <Button size="icon" variant="ghost" onClick={() => (isRunning ? setTerminalOpen(false) : clear())} className="size-6">
          <X className="size-3" />
        </Button>
      </div>

      <TerminalView runId={run.runId} lines={run.lines} />

      {run.exited?.error && <div className="shrink-0 px-3 py-1.5 font-mono text-xs text-red-400">{run.exited.error}</div>}
    </div>
  );
}
