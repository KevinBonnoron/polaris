import { useLiveQuery } from '@tanstack/react-db';
import { Bot } from 'lucide-react';
import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { PageHeader } from '@/components/atoms/page-header';
import { agentsCollection } from '@/db';
import { PageLoader } from '@/features/shell/page-loader';
import { useCurrentProject } from '@/state/projects';
import type { AgentKind } from '@/types';
import { AgentCard } from './agent-card';
import { AgentDetailModal } from './agent-detail-modal';
import { AgentsEmpty } from './agents-empty';
import { NewAgentButton } from './new-agent-button';

export function AgentsPage() {
  const { t } = useTranslation();
  const { project, projectId } = useCurrentProject();
  const { data: list = [], isReady } = useLiveQuery((q) => q.from({ a: agentsCollection }));
  const [pendingKind, setPendingKind] = useState<AgentKind | null>(null);
  const agents = useMemo(() => {
    const filtered = projectId ? list.filter((a) => a.projectId === projectId) : list;
    const rank: Record<string, number> = { waiting: 0, error: 1, working: 2, completed: 3, idle: 4 };
    return [...filtered].sort((a, b) => {
      const ra = rank[a.status] ?? 5;
      const rb = rank[b.status] ?? 5;
      if (ra !== rb) {
        return ra - rb;
      }
      return b.startedAt - a.startedAt;
    });
  }, [list, projectId]);

  const subtitle = `${project?.name ?? t('agents.page.noProject')}${isReady && agents.length > 0 ? ` · ${t('sidebar.agentCount', { count: agents.length })}` : ''}`;

  return (
    <div className="flex h-full flex-col gap-6 p-6">
      <PageHeader icon={<Bot className="size-5 text-muted-foreground" />} title={t('agents.page.title')} subtitle={subtitle} actions={<NewAgentButton onSelect={setPendingKind} />} />

      {!isReady ? (
        <PageLoader />
      ) : agents.length === 0 ? (
        <AgentsEmpty onSelect={setPendingKind} />
      ) : (
        <div className="grid grid-cols-1 gap-3 md:grid-cols-2 xl:grid-cols-3">
          {agents.map((agent) => (
            <AgentCard key={agent.id} agent={agent} />
          ))}
        </div>
      )}

      {pendingKind && <AgentDetailModal pending={{ kindId: pendingKind }} open={true} onOpenChange={(o) => !o && setPendingKind(null)} />}
    </div>
  );
}
