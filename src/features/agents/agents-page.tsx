import { useLiveQuery } from '@tanstack/react-db';
import { Bot, MousePointerClick } from 'lucide-react';
import { useEffect, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { agentsCollection } from '@/collections/agents.collection';
import { notificationsCollection } from '@/collections/notifications.collection';
import { PageHeader } from '@/components/atoms/page-header';
import { PageLoader } from '@/features/shell/page-loader';
import { selectAgent, useSelectedAgentId } from '@/state/agent-selection';
import { useCurrentProject } from '@/state/projects';
import type { Notification } from '@/types';
import { AgentConversation } from './agent-conversation';
import { AgentDraftCard } from './agent-draft-card';
import { AgentListItem } from './agent-list-item';
import { AgentsEmpty } from './agents-empty';
import { NewAgentButton } from './new-agent-button';

export function AgentsPage() {
  const { t } = useTranslation();
  const { project, projectId } = useCurrentProject();
  const { data: list = [], isReady } = useLiveQuery((q) => q.from({ a: agentsCollection }));
  const selectedId = useSelectedAgentId();

  const agents = useMemo(() => {
    const filtered = projectId ? list.filter((a) => a.projectId === projectId) : list;
    const rank: Record<string, number> = { draft: 0, waiting: 1, error: 2, working: 3, completed: 4, idle: 5 };
    return [...filtered].sort((a, b) => {
      const ra = rank[a.status] ?? 6;
      const rb = rank[b.status] ?? 6;
      if (ra !== rb) {
        return ra - rb;
      }
      return b.startedAt - a.startedAt;
    });
  }, [list, projectId]);

  const selected = agents.find((a) => a.id === selectedId) ?? null;
  const subtitle = `${project?.name ?? t('agents.page.noProject')}${isReady && agents.length > 0 ? ` · ${t('sidebar.agentCount', { count: agents.length })}` : ''}`;

  const { data: notifications = [] } = useLiveQuery((q) => q.from({ n: notificationsCollection }));
  useEffect(() => {
    if (!selectedId) return;
    const unread = notifications.filter(
      (n: Notification) => !n.read && n.type === 'agent' && n.payload.agentId === selectedId,
    );
    for (const n of unread) {
      notificationsCollection.update(n.id, (draft) => {
        draft.read = true;
      });
    }
  }, [selectedId, notifications]);

  return (
    <div className="flex h-full flex-col gap-6 p-4">
      <PageHeader icon={<Bot className="size-5 text-muted-foreground" />} title={t('agents.page.title')} subtitle={subtitle} actions={<NewAgentButton />} />

      {!isReady ? (
        <PageLoader />
      ) : agents.length === 0 ? (
        <AgentsEmpty />
      ) : (
        <div className="flex min-h-0 flex-1 gap-4">
          <div className="flex w-80 shrink-0 flex-col gap-1 overflow-y-auto pr-1">
            {agents.map((agent) => (agent.status === 'draft' ? <AgentDraftCard key={agent.id} agent={agent} selected={agent.id === selectedId} onSelect={() => selectAgent(agent.id)} /> : <AgentListItem key={agent.id} agent={agent} selected={agent.id === selectedId} onSelect={() => selectAgent(agent.id)} />))}
            <NewAgentButton variant="ghost" size="sm" className="mt-1 h-auto w-full justify-center gap-2 rounded-md border border-dashed border-border px-3 py-4 font-normal text-muted-foreground hover:bg-accent/50 hover:text-foreground" />
          </div>

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
      <p className="text-sm">{t('agents.page.selectSession', { defaultValue: 'Sélectionne une session à gauche' })}</p>
    </div>
  );
}
