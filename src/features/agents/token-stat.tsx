import { useTranslation } from 'react-i18next';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import { formatTokens } from '@/lib/format';
import type { TokenParts } from './use-live-tokens';

interface Props {
  tokens: number;
  parts: TokenParts;
  className?: string;
}

// TokenStat renders the running token count with a hover breakdown so users can
// see that a large total is mostly cache re-reads of the fixed context, not new
// work. The trigger keeps the same label other stats use.
export function TokenStat({ tokens, parts, className }: Props) {
  const { t } = useTranslation();
  const rows: Array<[string, number]> = [
    [t('agents.detail.tokenBreakdown.input'), parts.input],
    [t('agents.detail.tokenBreakdown.output'), parts.output],
    [t('agents.detail.tokenBreakdown.cacheCreate'), parts.cacheCreation],
    [t('agents.detail.tokenBreakdown.cacheRead'), parts.cacheRead],
  ];

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span className={className}>{t('agents.card.tokens', { count: formatTokens(tokens) })}</span>
      </TooltipTrigger>
      <TooltipContent className="flex flex-col gap-1">
        <span className="font-medium">{t('agents.detail.tokenBreakdown.title')}</span>
        {rows.map(([label, value]) => (
          <span key={label} className="flex justify-between gap-4 tabular-nums">
            <span className="opacity-70">{label}</span>
            <span>{value.toLocaleString()}</span>
          </span>
        ))}
        <span className="mt-1 max-w-52 opacity-60">{t('agents.detail.tokenBreakdown.hint')}</span>
      </TooltipContent>
    </Tooltip>
  );
}
