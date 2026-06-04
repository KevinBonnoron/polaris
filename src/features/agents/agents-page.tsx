import { useLiveQuery } from '@tanstack/react-db';
import { Bot, MousePointerClick } from 'lucide-react';
import { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { agentsCollection } from '@/collections/agents.collection';
import { customProvidersCollection } from '@/collections/custom-providers.collection';
import { notificationsCollection } from '@/collections/notifications.collection';
import { resolveProviderIcon } from '@/components/brand-icons';
import { PageHeader } from '@/components/atoms/page-header';
import { cn } from '@/lib/utils';
import { ScrollArea } from '@/components/ui/scroll-area';
import { PageLoader } from '@/features/shell/page-loader';
import { selectAgent, useSelectedAgentId } from '@/state/agent-selection';
import { useCurrentProject } from '@/state/projects';
import type { Notification } from '@/types';
import { AgentConversation } from './agent-conversation';
import { AgentDraftCard } from './agent-draft-card';
import { AgentListItem } from './agent-list-item';
import { AgentsEmpty } from './agents-empty';
import { NewAgentButton } from './new-agent-button';

const STATUS_ORDER = ['draft', 'waiting', 'error', 'working', 'completed', 'idle', 'archived'] as const;
type AgentStatus = (typeof STATUS_ORDER)[number];

export function AgentsPage() {
  const { t } = useTranslation();
  const { project, projectId } = useCurrentProject();
  const { data: list = [], isReady } = useLiveQuery((q) => q.from({ a: agentsCollection }));
  const { data: providers = [] } = useLiveQuery((q) => q.from({ p: customProvidersCollection }));
  const selectedId = useSelectedAgentId();
  const [statusFilter, setStatusFilter] = useState<AgentStatus | null>(null);

  const agents = useMemo(() => {
    const filtered = projectId ? list.filter((a) => a.projectId === projectId) : list;
    const rank: Record<string, number> = { draft: 0, waiting: 1, error: 2, working: 3, completed: 4, idle: 5, archived: 6 };
    return [...filtered].sort((a, b) => {
      const ra = rank[a.status] ?? 6;
      const rb = rank[b.status] ?? 6;
      if (ra !== rb) { return ra - rb; }
      return (b.updatedAt ?? b.startedAt) - (a.updatedAt ?? a.startedAt);
    });
  }, [list, projectId]);

  const selected = agents.find((a) => a.id === selectedId) ?? null;
  const subtitle = `${project?.name ?? t('agents.page.noProject')}${isReady && agents.length > 0 ? ` · ${t('sidebar.agentCount', { count: agents.length })}` : ''}`;

  const { data: notifications = [] } = useLiveQuery((q) => q.from({ n: notificationsCollection }));
  useEffect(() => {
    if (!selectedId) { return; }
    const unread = notifications.filter((n: Notification) => !n.read && n.type === 'agent' && n.payload.agentId === selectedId);
    for (const n of unread) {
      notificationsCollection.update(n.id, (draft) => {
        draft.read = true;
      });
    }
  }, [selectedId, notifications]);

  const statusCounts = useMemo(() => {
    const counts: Partial<Record<AgentStatus, number>> = {};
    for (const a of agents) {
      const s = a.status as AgentStatus;
      counts[s] = (counts[s] ?? 0) + 1;
    }
    return counts;
  }, [agents]);

  const activeStatuses = useMemo(() => STATUS_ORDER.filter((s) => (statusCounts[s] ?? 0) > 0), [statusCounts]);

  const effectiveFilter = statusFilter && activeStatuses.includes(statusFilter) ? statusFilter : null;

  const groups = useMemo(() => {
    const toShow = effectiveFilter ? agents.filter((a) => a.status === effectiveFilter) : agents;
    return STATUS_ORDER.map((status) => ({ status, items: toShow.filter((a) => a.status === status) })).filter((g) => g.items.length > 0);
  }, [agents, effectiveFilter]);

  return (
    <div className="flex h-full flex-col gap-6 p-4">
      <PageHeader icon={<Bot className="size-5 text-muted-foreground" />} title={t('agents.page.title')} subtitle={subtitle} actions={<NewAgentButton />} />

      {!isReady ? (
        <PageLoader />
      ) : agents.length === 0 ? (
        <AgentsEmpty />
      ) : (
        <div className="flex min-h-0 flex-1 gap-4">
          <ScrollArea className="w-80 shrink-0">
            <div className="flex w-80 flex-col gap-1 pr-2">
              {activeStatuses.length > 1 && (
                <div className="mb-1 flex flex-wrap gap-1 px-1">
                  <button onClick={() => setStatusFilter(null)} className={cn('rounded px-2 py-0.5 text-xs transition-colors', effectiveFilter === null ? 'bg-foreground/10 font-medium text-foreground' : 'text-muted-foreground hover:text-foreground')}>
                    {t('agents.page.filterAll')}
                  </button>
                  {activeStatuses.map((status) => (
                    <button key={status} onClick={() => setStatusFilter(effectiveFilter === status ? null : status)} className={cn('rounded px-2 py-0.5 text-xs transition-colors', effectiveFilter === status ? 'bg-foreground/10 font-medium text-foreground' : 'text-muted-foreground hover:text-foreground')}>
                      {t(`agents.status.${status}`)}
                      <span className="ml-1 tabular-nums opacity-60">{statusCounts[status]}</span>
                    </button>
                  ))}
                </div>
              )}

              {groups.map((group, i) => (
                <div key={group.status}>
                  {groups.length > 1 && <div className={cn('px-1 pb-1 text-xs text-muted-foreground/60', i > 0 && 'mt-3 border-t border-border/40 pt-2')}>{t(`agents.status.${group.status}`)}</div>}
                  <div className="flex flex-col gap-0.5">
                    {group.items.map((agent) => {
                      const provider = agent.providerId ? providers.find((p) => p.id === agent.providerId) : undefined;
                      const providerIcon = resolveProviderIcon(provider?.icon);
                      return agent.status === 'draft' ? (
                        <AgentDraftCard key={agent.id} agent={agent} selected={agent.id === selectedId} onSelect={() => selectAgent(agent.id)} />
                      ) : (
                        <AgentListItem key={agent.id} agent={agent} selected={agent.id === selectedId} onSelect={() => selectAgent(agent.id)} providerIcon={providerIcon} />
                      );
                    })}
                  </div>
                </div>
              ))}

              <NewAgentButton variant="ghost" size="sm" className="mt-1 h-auto w-full justify-center gap-2 rounded-md border border-dashed border-border px-3 py-4 font-normal text-muted-foreground hover:bg-accent/50 hover:text-foreground" />
            </div>
          </ScrollArea>

          <div className="min-w-0 flex-1 rounded-lg border border-border bg-card/40 p-4">{selected ? <AgentConversation key={selected.id} agentId={selected.id} /> : <NoSelection />}</div>
        </div>
      )}
    </div>
  );
}

function NoSelection() {
  const { t } = useTranslation();
  return (
    <div className="flex h-full w-full flex-col items-center justify-center gap-3 text-center text-muted-foreground">
      <div className="flex size-12 items-center justify-center rounded-full bg-muted">
        <MousePointerClick className="size-5" />
      </div>
      <p className="text-sm">{t('agents.page.selectSession')}</p>
    </div>
  );
}
