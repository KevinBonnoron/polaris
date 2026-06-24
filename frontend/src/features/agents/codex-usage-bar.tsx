import { useTranslation } from 'react-i18next';
import { formatTokens } from '@/lib/format';
import { FetchCodexUsage } from '@/wailsjs/go/main/App';
import { createUsageHook } from './create-usage-hook';
import { UsageLimitPanel } from './usage-limit-panel';
import { formatUsageWindow } from './usage-limit-utils';
import { tokenTotal, type TokenParts } from './use-live-tokens';

interface CodexUsageData {
  percentUsed: number;
  resetAt?: string;
  windowMinutes?: number;
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

  const totalTokens = usage ? tokenTotal(usage.totalTokens) || usage.lifetimeTokens || 0 : 0;
  const window = formatUsageWindow(usage?.windowMinutes, '?');
  const rows = usage ? [{ label: t('agents.usage.window', { provider: 'Codex', window }), percentUsed: usage.percentUsed, resetAt: usage.resetAt }] : [];
  const footer = usage ? (
    <div className="flex items-center justify-between text-[10px]">
      <span className="text-muted-foreground">{usage.planType ? t('agents.usage.plan', { plan: usage.planType }) : t('agents.usage.localTokens')}</span>
      <span className="tabular-nums">{formatTokens(totalTokens)}</span>
    </div>
  ) : null;

  return <UsageLimitPanel title={t('agents.usage.title', { provider: 'Codex' })} rows={rows} loading={loading} onRefresh={() => void refresh(true)} footer={footer} />;
}
