import { useTranslation } from 'react-i18next';
import { FetchCodexUsage } from '@/wailsjs/go/main/App';
import { createUsageHook } from './create-usage-hook';
import { UsageLimitPanel } from './usage-limit-panel';
import { formatUsageWindow } from './usage-limit-utils';
import { type TokenParts } from './use-live-tokens';

interface CodexUsageData {
  percentUsed: number;
  resetAt?: string;
  windowMinutes?: number;
  weeklyPercentUsed: number | null;
  weeklyResetAt: string | null;
  weeklyWindowMinutes: number | null;
  planType?: string;
  totalTokens: TokenParts;
  lifetimeTokens?: number;
}

export const useCodexUsage = createUsageHook<CodexUsageData>(FetchCodexUsage);

export function CodexUsageBar() {
  const { t } = useTranslation();
  const { usage, loading, error, refresh } = useCodexUsage();

  if (error && !usage) {
    return null;
  }

  const sessionWindow = formatUsageWindow(usage?.windowMinutes, '?');

  const rows = usage
    ? [
        { label: t('agents.usage.window', { provider: 'Codex', window: sessionWindow }), percentUsed: usage.percentUsed, resetAt: usage.resetAt },
        ...(usage.weeklyPercentUsed !== null ? [{ label: t('agents.usage.weekly'), percentUsed: usage.weeklyPercentUsed, resetAt: usage.weeklyResetAt ?? undefined }] : []),
      ]
    : [];

  const footer = usage?.planType ? <div className="text-[10px] text-muted-foreground">{t('agents.usage.plan', { plan: usage.planType })}</div> : null;

  return <UsageLimitPanel title={t('agents.usage.title', { provider: 'Codex' })} rows={rows} loading={loading} onRefresh={() => void refresh(true)} footer={footer} skeletonRows={2} />;
}
