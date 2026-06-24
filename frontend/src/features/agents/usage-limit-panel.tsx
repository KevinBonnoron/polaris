import { RefreshCw } from 'lucide-react';
import type { ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@/components/ui/button';
import { Progress } from '@/components/ui/progress';
import { Skeleton } from '@/components/ui/skeleton';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import { cn } from '@/lib/utils';
import { formatUsageResetAt, type UsageLimitRow, usageBarColor } from './usage-limit-utils';

interface Props {
  title: ReactNode;
  rows: UsageLimitRow[];
  loading: boolean;
  onRefresh: () => void;
  footer?: ReactNode;
  skeletonRows?: number;
}

export function UsageLimitPanel({ title, rows, loading, onRefresh, footer, skeletonRows = 1 }: Props) {
  const { t, i18n } = useTranslation();
  const locale = i18n.language;
  const displayRows = rows.map((row) => ({ ...row, reset: formatUsageResetAt(row.resetAt, locale) }));

  if (rows.length === 0 && !loading) {
    return null;
  }

  if (rows.length === 0) {
    return (
      <div className="space-y-2 pt-2">
        <div className="flex items-center justify-between">
          <Skeleton className="h-2.5 w-14" />
          <Skeleton className="size-5 rounded" />
        </div>
        {Array.from({ length: skeletonRows }).map((_, idx) => (
          <div key={idx} className="space-y-1.5">
            <div className="flex items-baseline justify-between">
              <Skeleton className="h-2.5 w-10" />
              <Skeleton className="h-2.5 w-20" />
            </div>
            <Skeleton className="h-1.5 w-full" />
          </div>
        ))}
      </div>
    );
  }

  return (
    <div className="space-y-2 pt-2">
      <div className="flex items-center justify-between">
        <span className="text-[10px] font-medium text-muted-foreground">{title}</span>
        <Tooltip>
          <TooltipTrigger asChild>
            <Button variant="ghost" size="icon" className="size-5" onClick={onRefresh} disabled={loading} aria-label={t('common.refresh')} title={t('common.refresh')}>
              <RefreshCw className={cn('size-3', loading && 'animate-spin')} />
            </Button>
          </TooltipTrigger>
          <TooltipContent>{t('common.refresh')}</TooltipContent>
        </Tooltip>
      </div>
      {displayRows.map((row) => (
        <div key={row.label} className="space-y-1.5">
          <div className="flex items-baseline justify-between text-[10px]">
            <span className="text-muted-foreground">{row.label}</span>
            <span className="tabular-nums">
              {row.valueLabel ?? (
                <>
                  {100 - row.percentUsed}% {t('agents.usage.left')}
                  {row.reset && <span className="ml-1 text-muted-foreground">· {row.reset}</span>}
                </>
              )}
            </span>
          </div>
          <Progress value={row.percentUsed} className="h-1.5" indicatorClassName={usageBarColor(row.percentUsed)} />
        </div>
      ))}
      {footer}
    </div>
  );
}
