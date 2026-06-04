import { Clock } from 'lucide-react';
import { type RefObject, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import type { polaris } from '@/wailsjs/go/models';
import { Button } from '@/components/ui/button';
import { ScrollArea } from '@/components/ui/scroll-area';
import { Skeleton } from '@/components/ui/skeleton';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import { cn } from '@/lib/utils';
import { buildLogBlocks, LogBlocksGrid } from './log-format';
import { ThinkingIndicator } from './thinking-indicator';

const LOG_SKELETON_WIDTHS = ['w-3/4', 'w-1/2', 'w-5/6', 'w-2/3', 'w-4/5', 'w-1/3', 'w-3/5'];

interface Props {
  log: polaris.StreamEvent[];
  isWorking: boolean;
  isLoading: boolean;
  logRef: RefObject<HTMLDivElement | null>;
  onLogScroll: () => void;
}

export function AgentDetailLogsTab({ log, isWorking, isLoading, logRef, onLogScroll }: Props) {
  const { t } = useTranslation();
  const [showTimestamps, setShowTimestamps] = useState(false);

  const blocks = useMemo(() => {
    if (!log?.length) {
      return null;
    }
    return buildLogBlocks(log);
  }, [log]);

  return (
    <div className="relative min-h-0 flex-1">
      <ScrollArea className="h-full rounded-md border border-border bg-background/60" viewportProps={{ ref: logRef, onScroll: onLogScroll, className: 'px-4 py-3' }}>
        {isLoading && !blocks ? (
          <div className="flex flex-col gap-2">
            {LOG_SKELETON_WIDTHS.map((w, i) => (
              <Skeleton key={i} className={cn('h-3 rounded', w)} />
            ))}
          </div>
        ) : blocks ? (
          <LogBlocksGrid blocks={blocks} showTimestamps={showTimestamps} />
        ) : (
          <span className="font-mono text-xs text-muted-foreground">{isWorking ? t('agents.detail.waitingOutput') : t('agents.detail.noOutput')}</span>
        )}
        {isWorking && <ThinkingIndicator />}
      </ScrollArea>
      <Tooltip>
        <TooltipTrigger asChild>
          <Button type="button" variant="ghost" size="icon" className={cn('absolute right-2 top-2 size-6 opacity-40 hover:opacity-100', showTimestamps && 'opacity-80')} onClick={() => setShowTimestamps((v) => !v)}>
            <Clock className="size-3.5" />
          </Button>
        </TooltipTrigger>
        <TooltipContent side="left">{showTimestamps ? t('agents.detail.hideTimestamps') : t('agents.detail.showTimestamps')}</TooltipContent>
      </Tooltip>
    </div>
  );
}
