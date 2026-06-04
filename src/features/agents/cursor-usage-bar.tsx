import { RefreshCw } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { Button } from '@/components/ui/button';
import { Progress } from '@/components/ui/progress';
import { Skeleton } from '@/components/ui/skeleton';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import { cn } from '@/lib/utils';
import { FetchCursorUsage } from '@/wailsjs/go/main/App';
import { createUsageHook } from './create-usage-hook';

interface CursorUsageData {
  numRequests: number;
  numRequestsTotal: number;
  numTokens: number;
  numCents: number;
  startOfMonth?: string;
  membershipType?: string;
  lastUpdated: string;
  error?: string;
}

function barColor(pct: number): string {
  if (pct >= 90) { return 'bg-red-500'; }
  if (pct >= 70) { return 'bg-amber-500'; }
  return 'bg-emerald-500';
}

function UsageRow({ label, value, total, locale }: { label: string; value: number; total: number; locale: string }) {
  const { t } = useTranslation();
  const pct = total > 0 ? Math.min(100, Math.round((value / total) * 100)) : 0;
  const remaining = total - value;
  return (
    <div className="space-y-1.5">
      <div className="flex items-baseline justify-between text-[10px]">
        <span className="text-muted-foreground">{label}</span>
        <span className="tabular-nums">
          {remaining.toLocaleString(locale)} {t('agents.cursorUsage.left')}
          <span className="ml-1 text-muted-foreground">
            · {value.toLocaleString(locale)} / {total.toLocaleString(locale)}
          </span>
        </span>
      </div>
      <Progress value={pct} className="h-1.5" indicatorClassName={barColor(pct)} />
    </div>
  );
}

const useCursorUsage = createUsageHook<CursorUsageData>(FetchCursorUsage);

export function CursorUsageBar() {
  const { t, i18n } = useTranslation();
  const locale = i18n.language;
  const { usage, loading, error, refresh } = useCursorUsage();

  if (error && !usage) {
    return null;
  }

  if (!usage) {
    if (loading) {
      return (
        <div className="space-y-2 pt-2">
          <div className="flex items-center justify-between">
            <Skeleton className="h-2.5 w-16" />
            <Skeleton className="size-5 rounded" />
          </div>
          <div className="space-y-1.5">
            <div className="flex items-baseline justify-between">
              <Skeleton className="h-2.5 w-10" />
              <Skeleton className="h-2.5 w-24" />
            </div>
            <Skeleton className="h-1.5 w-full" />
          </div>
        </div>
      );
    }
    return null;
  }

  const hasRequestBar = usage.numRequestsTotal > 0;
  const hasCost = usage.numCents > 0;
  const plan = usage.membershipType;

  return (
    <div className="space-y-2 pt-2">
      <div className="flex items-center justify-between">
        <span className="text-[10px] font-medium text-muted-foreground">
          {t('agents.cursorUsage.title')}
          {plan && <span className="ml-1 capitalize opacity-60">{plan}</span>}
        </span>
        <Tooltip>
          <TooltipTrigger asChild>
            <Button variant="ghost" size="icon" className="size-5" onClick={() => void refresh(true)} disabled={loading}>
              <RefreshCw className={cn('size-3', loading && 'animate-spin')} />
            </Button>
          </TooltipTrigger>
          <TooltipContent>{t('common.refresh')}</TooltipContent>
        </Tooltip>
      </div>

      {hasRequestBar && <UsageRow label={t('agents.cursorUsage.requests')} value={usage.numRequests} total={usage.numRequestsTotal} locale={locale} />}

      {!hasRequestBar && <div className="text-[10px] text-muted-foreground">{t('agents.cursorUsage.requestsUnlimited', { count: usage.numRequests })}</div>}

      {hasCost && (
        <div className="flex items-baseline justify-between text-[10px]">
          <span className="text-muted-foreground">{t('agents.cursorUsage.cost')}</span>
          <span className="tabular-nums">${(usage.numCents / 100).toFixed(2)}</span>
        </div>
      )}
    </div>
  );
}
