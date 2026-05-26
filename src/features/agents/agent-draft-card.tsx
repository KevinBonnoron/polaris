import { useLiveQuery } from '@tanstack/react-db';
import { Trash2 } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { agentsCollection } from '@/collections/agents.collection';
import { customProvidersCollection } from '@/collections/custom-providers.collection';
import { Button } from '@/components/ui/button';
import { formatTokens } from '@/lib/format';
import { formatRelative } from '@/lib/time';
import { cn } from '@/lib/utils';
import type { Agent } from '@/types';
import { resolveProviderIcon } from '@/components/brand-icons';
import { findAgentKind, OPENCODE_DESCRIPTOR } from './agent-kinds';

interface Props {
  agent: Agent;
  selected: boolean;
  onSelect: () => void;
}

export function AgentDraftCard({ agent, selected, onSelect }: Props) {
  const { t } = useTranslation();
  const { data: providers = [] } = useLiveQuery((q) => q.from({ p: customProvidersCollection }));
  const provider = agent.providerId ? providers.find((p) => p.id === agent.providerId) : undefined;
  const kindCfg = findAgentKind(agent.kind) ?? (agent.kind === 'opencode' ? OPENCODE_DESCRIPTOR : undefined);
  const KindIcon = resolveProviderIcon(provider?.icon) ?? kindCfg?.icon;

  const remove = async () => {
    try {
      await agentsCollection.delete(agent.id);
    } catch {
      // proto: best effort
    }
  };

  return (
    <div className="group relative">
      <button type="button" onClick={onSelect} className={cn('flex w-full flex-col gap-1 rounded-md border px-3 py-2.5 text-left transition-colors', selected ? 'border-border/60 bg-accent' : 'border-border/30 hover:border-border/50 hover:bg-accent/50')}>
        <div className="flex items-center gap-2">
          <span className="size-2 shrink-0 rounded-full bg-muted-foreground" aria-hidden />
          {KindIcon && <KindIcon className="size-3.5 shrink-0 text-muted-foreground" />}
          <span className="shrink-0 text-[11px] text-muted-foreground">{t('agents.status.draft', { defaultValue: 'Brouillon' })}</span>
        </div>
        <p className="truncate text-xs text-muted-foreground">{t('agents.draft.hint', { defaultValue: 'Envoyez un message pour démarrer' })}</p>
        <div className="flex items-center justify-between text-[10px] text-muted-foreground">
          <span>{t('agents.card.started', { when: formatRelative(agent.startedAt) })}</span>
          <span className="tabular-nums">{t('agents.card.tokens', { count: formatTokens(0) })}</span>
        </div>
      </button>
      <div className="absolute right-2 top-2 hidden group-hover:block">
        <Button variant="ghost" size="icon" onClick={remove} className="size-6 text-muted-foreground hover:text-destructive" title={t('agents.draft.discard', { defaultValue: 'Supprimer le brouillon' })}>
          <Trash2 className="size-3" />
        </Button>
      </div>
    </div>
  );
}
