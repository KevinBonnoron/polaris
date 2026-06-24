import { useTranslation } from 'react-i18next';
import { FetchClaudeUsage } from '@/wailsjs/go/main/App';
import { createUsageHook } from './create-usage-hook';
import { UsageLimitPanel } from './usage-limit-panel';

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

export const useClaudeUsage = createUsageHook<UsageData>(FetchClaudeUsage);

interface Props {
  model?: string;
}

export function ClaudeUsageBar({ model }: Props) {
  const { t } = useTranslation();
  const { usage, loading, error, refresh } = useClaudeUsage();

  if (error && !usage) {
    return null;
  }

  const modelUsage = model ? usage?.weeklyByModel?.[model] : undefined;
  const rows = usage
    ? [
        { label: t('agents.usage.window', { provider: 'Claude', window: '5h' }), percentUsed: usage.sessionPercentUsed, resetAt: usage.sessionResetAt },
        ...(modelUsage && model ? [{ label: t('agents.usage.weeklyModel', { model: t(`agents.modelFamily.${model}`) }), percentUsed: modelUsage.percentUsed, resetAt: modelUsage.resetAt }] : []),
        { label: t('agents.usage.weekly'), percentUsed: usage.weeklyPercentUsed, resetAt: usage.weeklyResetAt },
      ]
    : [];

  return <UsageLimitPanel title={t('agents.usage.title', { provider: 'Claude' })} rows={rows} loading={loading} onRefresh={() => void refresh(true)} skeletonRows={2} />;
}
