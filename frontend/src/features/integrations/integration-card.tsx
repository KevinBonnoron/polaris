import { Check } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { cn } from '@/lib/utils';
import type { Integration } from './integration-catalog';

interface Props {
  integration: Integration;
  connected: boolean;
  onToggle: (id: string) => void;
}

export function IntegrationCard({ integration, connected, onToggle }: Props) {
  const { t } = useTranslation();
  const Icon = integration.icon;
  return (
    <button type="button" onClick={() => onToggle(integration.id)} className={cn('flex items-center gap-3 rounded-lg border p-3 text-left transition-colors', connected ? 'border-primary/40 bg-accent/40' : 'border-border hover:bg-accent/40')}>
      <div className={cn('flex size-10 shrink-0 items-center justify-center rounded-md', integration.tint)}>
        <Icon className="size-5" />
      </div>
      <div className="min-w-0 flex-1">
        <div className="truncate font-medium">{integration.name}</div>
        <div className="text-xs text-muted-foreground">{connected ? t('integrations.card.connected') : t('integrations.card.notConnected')}</div>
      </div>
      {connected && <Check className="size-4 text-primary" aria-hidden />}
    </button>
  );
}
