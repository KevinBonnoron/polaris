import { RefreshCw } from 'lucide-react';
import { useCallback, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@/components/ui/button';
import { Progress } from '@/components/ui/progress';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import { cn } from '@/lib/utils';
import { FetchClaudeUsage } from '@/wailsjs/go/main/App';

interface UsageData {
  sessionPercentUsed: number;
  sessionResetAt?: string;
  weeklyPercentUsed: number;
  weeklyResetAt?: string;
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

function useClaudeUsage() {
  const [usage, setUsage] = useState<UsageData | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const refresh = useCallback(async (force = false) => {
    setLoading(true);
    setError(null);
    try {
      const data = await FetchClaudeUsage(force);
      if (data.error) {
        setError(data.error);
      } else {
        setUsage(data);
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void refresh(false);
  }, [refresh]);

  return { usage, loading, error, refresh };
}

export function ClaudeUsageBar() {
  const { t, i18n } = useTranslation();
  const locale = i18n.language;
  const { usage, loading, error, refresh } = useClaudeUsage();

  if (error && !usage) {
    return null;
  }

  if (!usage) {
    if (loading) {
      return (
        <div className="flex items-center gap-2 pt-1 text-[10px] text-muted-foreground">
          <RefreshCw className="size-3 animate-spin" />
          {t('agents.usage.loading')}
        </div>
      );
    }

    return null;
  }

  const sessionLeft = 100 - usage.sessionPercentUsed;
  const weeklyLeft = 100 - usage.weeklyPercentUsed;
  const sessionReset = formatResetAt(usage.sessionResetAt, locale);
  const weeklyReset = formatResetAt(usage.weeklyResetAt, locale);

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

      <div className="space-y-1.5">
        <div className="flex items-baseline justify-between text-[10px]">
          <span className="text-muted-foreground">{t('agents.usage.session')}</span>
          <span className="tabular-nums">
            {sessionLeft}% {t('agents.usage.left')}
            {sessionReset && <span className="ml-1 text-muted-foreground">· {sessionReset}</span>}
          </span>
        </div>
        <Progress value={usage.sessionPercentUsed} className="h-1.5" indicatorClassName={barColor(usage.sessionPercentUsed)} />
      </div>

      <div className="space-y-1.5">
        <div className="flex items-baseline justify-between text-[10px]">
          <span className="text-muted-foreground">{t('agents.usage.weekly')}</span>
          <span className="tabular-nums">
            {weeklyLeft}% {t('agents.usage.left')}
            {weeklyReset && <span className="ml-1 text-muted-foreground">· {weeklyReset}</span>}
          </span>
        </div>
        <Progress value={usage.weeklyPercentUsed} className="h-1.5" indicatorClassName={barColor(usage.weeklyPercentUsed)} />
      </div>
    </div>
  );
}
