import { Square, X } from 'lucide-react';
import { useEffect, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { cn } from '@/lib/utils';
import { useNodejsRun } from './nodejs-run-context';

export function NodejsTerminal() {
  const { t } = useTranslation();
  const { run, isRunning, terminalOpen, setTerminalOpen, stop, clear } = useNodejsRun();
  const outputRef = useRef<HTMLDivElement>(null);

  const lineCount = run?.lines.length ?? 0;
  // biome-ignore lint/correctness/useExhaustiveDependencies: lineCount is the scroll trigger
  useEffect(() => {
    const el = outputRef.current;
    if (el) {
      el.scrollTop = el.scrollHeight;
    }
  }, [lineCount]);

  if (!run || !terminalOpen) {
    return null;
  }

  return (
    <div className="flex shrink-0 flex-col border-t border-border" style={{ height: 240 }}>
      <div className="flex h-9 shrink-0 items-center gap-2 border-b border-border/50 bg-muted/30 px-3">
        <span className="text-xs font-medium text-muted-foreground">{t('integrations.nodejs.terminal')}</span>
        <span className="truncate text-xs font-mono">{run.scriptName}</span>

        {isRunning && (
          <Badge variant="secondary" className="h-4 gap-1 bg-emerald-500/10 px-1.5 text-[10px] text-emerald-400">
            <span className="size-1.5 animate-pulse rounded-full bg-emerald-500" />
            {t('integrations.nodejs.running')}
          </Badge>
        )}
        {run.exited && (
          <Badge variant={run.exited.code === 0 ? 'secondary' : 'destructive'} className={cn('h-4 px-1.5 text-[10px]', run.exited.code === 0 && 'bg-emerald-500/10 text-emerald-400')}>
            {run.exited.code === 0 ? t('integrations.nodejs.exitOk') : t('integrations.nodejs.exitCode', { code: run.exited.code })}
          </Badge>
        )}

        <div className="flex-1" />

        {isRunning && (
          <Button size="icon" variant="ghost" onClick={() => stop()} className="size-6">
            <Square className="size-3 text-destructive" />
          </Button>
        )}
        <Button size="icon" variant="ghost" onClick={() => (isRunning ? setTerminalOpen(false) : clear())} className="size-6">
          <X className="size-3" />
        </Button>
      </div>

      <div ref={outputRef} className="flex-1 overflow-auto bg-muted/20 px-3 py-2 font-mono text-xs">
        {run.lines.map((line) => (
          <div key={line.seq} className={cn('whitespace-pre-wrap', line.stream === 'stderr' && 'text-red-400', line.stream === 'system' && 'text-muted-foreground')}>
            {line.line}
          </div>
        ))}
        {run.exited?.error && <div className="mt-2 text-red-400">{run.exited.error}</div>}
      </div>
    </div>
  );
}
