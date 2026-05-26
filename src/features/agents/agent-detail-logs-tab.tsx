import { type RefObject, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { ScrollArea } from '@/components/ui/scroll-area';
import { buildLogBlocks, LogBlocksGrid } from './log-format';
import { ThinkingIndicator } from './thinking-indicator';

interface Props {
  log: string;
  isWorking: boolean;
  logRef: RefObject<HTMLDivElement | null>;
  onLogScroll: () => void;
}

export function AgentDetailLogsTab({ log, isWorking, logRef, onLogScroll }: Props) {
  const { t } = useTranslation();

  const blocks = useMemo(() => {
    if (!log) {
      return null;
    }
    const raw = log.split('\n');
    while (raw.length > 0 && raw[raw.length - 1] === '') {
      raw.pop();
    }
    const collapsed: string[] = [];
    let prevBlank = false;
    for (const line of raw) {
      const blank = line.trim() === '';
      if (blank && prevBlank) {
        continue;
      }
      collapsed.push(line);
      prevBlank = blank;
    }
    return buildLogBlocks(collapsed);
  }, [log]);

  return (
    <ScrollArea className="min-h-0 flex-1 rounded-md border border-border bg-background/60" viewportProps={{ ref: logRef, onScroll: onLogScroll, className: 'px-4 py-3' }}>
      {blocks ? <LogBlocksGrid blocks={blocks} /> : <span className="font-mono text-xs text-muted-foreground">{isWorking ? t('agents.detail.waitingOutput') : t('agents.detail.noOutput')}</span>}
      {isWorking && <ThinkingIndicator />}
    </ScrollArea>
  );
}
