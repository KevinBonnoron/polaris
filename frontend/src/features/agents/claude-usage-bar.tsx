import { RefreshCw } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { Button } from '@/components/ui/button';
import { Progress } from '@/components/ui/progress';
import { Skeleton } from '@/components/ui/skeleton';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import { cn } from '@/lib/utils';
import { FetchClaudeUsage } from '@/wailsjs/go/main/App';
import { createUsageHook } from './create-usage-hook';

interface ModelUsage {
  percentUsed: number;
  resetAt?: string;
}

interface UsageData {
  sessionPercentUsed: number;
  sessionResetAt?: string;
  weeklyPercentUsed: number;
  weeklyResetAt?: string;
  weeklyByModel?: Record<string, ModelUsage | undefined>;
}

function barColor(pct: number): string {
  if (pct >= 90) {
    return 'bg-red-500';
  }
  if (pct >= 70) {
    return 'bg-amber-500';
  }
  return 'bg-emerald-500';
}

function formatResetAt(iso: string | undefined, locale: string): string | null {
  if (!iso) {
    return null;
  }
  const target = new Date(iso);
  const now = Date.now();
  const diffMs = target.getTime() - now;

  if (diffMs > 0 && diffMs < 24 * 60 * 60 * 1000) {
    const rtf = new Intl.RelativeTimeFormat(locale, { numeric: 'auto' });
    const diffMin = Math.round(diffMs / 60_000);
    if (diffMin < 60) {
      return rtf.format(diffMin, 'minute');
    }
    return rtf.format(Math.round(diffMin / 60), 'hour');
  }

  return new Intl.DateTimeFormat(locale, { dateStyle: 'medium', timeStyle: 'short' }).format(target);
}

function UsageRow({ label, percentUsed, resetAt, locale }: { label: string; percentUsed: number; resetAt?: string; locale: string }) {
  const { t } = useTranslation();
  const reset = formatResetAt(resetAt, locale);
  return (
    <div className="space-y-1.5">
      <div className="flex items-baseline justify-between text-[10px]">
        <span className="text-muted-foreground">{label}</span>
        <span className="tabular-nums">
          {100 - percentUsed}% {t('agents.usage.left')}
          {reset && <span className="ml-1 text-muted-foreground">· {reset}</span>}
        </span>
      </div>
      <Progress value={percentUsed} className="h-1.5" indicatorClassName={barColor(percentUsed)} />
    </div>
  );
}

export const useClaudeUsage = createUsageHook<UsageData>(FetchClaudeUsage);

export function ClaudeUsageBar({ model }: { model?: string }) {
  const { t, i18n } = useTranslation();
  const locale = i18n.language;
  const { usage, loading, error, refresh } = useClaudeUsage();

  if (error && !usage) {
    return null;
  }

  if (!usage) {
    if (loading) {
      return (
        <div className="space-y-2 pt-2">
          <div className="flex items-center justify-between">
            <Skeleton className="h-2.5 w-14" />
            <Skeleton className="size-5 rounded" />
          </div>
          <div className="space-y-1.5">
            <div className="flex items-baseline justify-between">
              <Skeleton className="h-2.5 w-10" />
              <Skeleton className="h-2.5 w-20" />
            </div>
            <Skeleton className="h-1.5 w-full" />
          </div>
          <div className="space-y-1.5">
            <div className="flex items-baseline justify-between">
              <Skeleton className="h-2.5 w-10" />
              <Skeleton className="h-2.5 w-20" />
            </div>
            <Skeleton className="h-1.5 w-full" />
          </div>
        </div>
      );
    }

    return null;
  }

  const modelUsage = model ? usage.weeklyByModel?.[model] : undefined;

  return (
    <div className="space-y-2 pt-2">
      <div className="flex items-center justify-between">
        <span className="text-[10px] font-medium text-muted-foreground">{t('agents.usage.title')}</span>
        <Tooltip>
          <TooltipTrigger asChild>
            <Button variant="ghost" size="icon" className="size-5" onClick={() => void refresh(true)} disabled={loading}>
              <RefreshCw className={cn('size-3', loading && 'animate-spin')} />
            </Button>
          </TooltipTrigger>
          <TooltipContent>{t('common.refresh')}</TooltipContent>
        </Tooltip>
      </div>

      <UsageRow label={t('agents.usage.session')} percentUsed={usage.sessionPercentUsed} resetAt={usage.sessionResetAt} locale={locale} />
      {modelUsage && model && <UsageRow label={t('agents.usage.weeklyModel', { model: t(`agents.modelFamily.${model}`) })} percentUsed={modelUsage.percentUsed} resetAt={modelUsage.resetAt} locale={locale} />}
      <UsageRow label={t('agents.usage.weekly')} percentUsed={usage.weeklyPercentUsed} resetAt={usage.weeklyResetAt} locale={locale} />
    </div>
  );
}
