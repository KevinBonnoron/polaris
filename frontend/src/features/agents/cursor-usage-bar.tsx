import { useTranslation } from 'react-i18next';
import { FetchCursorUsage } from '@/wailsjs/go/main/App';
import { createUsageHook } from './create-usage-hook';
import { UsageLimitPanel } from './usage-limit-panel';

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

const useCursorUsage = createUsageHook<CursorUsageData>(FetchCursorUsage);

export function CursorUsageBar() {
  const { t, i18n } = useTranslation();
  const locale = i18n.language;
  const { usage, loading, error, refresh } = useCursorUsage();

  if (error && !usage) {
    return null;
  }

  const hasRequestBar = (usage?.numRequestsTotal ?? 0) > 0;
  const rows =
    usage && hasRequestBar
      ? [
          {
            label: t('agents.cursorUsage.requests'),
            percentUsed: Math.min(100, Math.round((usage.numRequests / usage.numRequestsTotal) * 100)),
            valueLabel: (
              <>
                {(usage.numRequestsTotal - usage.numRequests).toLocaleString(locale)} {t('agents.usage.left')}
                <span className="ml-1 text-muted-foreground">
                  · {usage.numRequests.toLocaleString(locale)} / {usage.numRequestsTotal.toLocaleString(locale)}
                </span>
              </>
            ),
          },
        ]
      : [];
  const footer = usage ? (
    <>
      {!hasRequestBar && <div className="text-[10px] text-muted-foreground">{t('agents.cursorUsage.requestsUnlimited', { count: usage.numRequests })}</div>}
      {usage.numCents > 0 && (
        <div className="flex items-baseline justify-between text-[10px]">
          <span className="text-muted-foreground">{t('agents.cursorUsage.cost')}</span>
          <span className="tabular-nums">${(usage.numCents / 100).toFixed(2)}</span>
        </div>
      )}
    </>
  ) : null;
  const title = (
    <>
      {t('agents.usage.title', { provider: 'Cursor' })}
      {usage?.membershipType && <span className="ml-1 capitalize opacity-60">{usage.membershipType}</span>}
    </>
  );

  return <UsageLimitPanel title={title} rows={rows} loading={loading} onRefresh={() => void refresh(true)} footer={footer} />;
}
