import { KanbanSquare, Plus, Settings } from 'lucide-react';
import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { OpenExternalAction } from '@/components/atoms/open-external-action';
import { PageHeader } from '@/components/atoms/page-header';
import { RefreshAction } from '@/components/atoms/refresh-action';
import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { Separator } from '@/components/ui/separator';
import { ConfigureIntegrationModal } from '@/features/integrations/configure-integration-modal';
import { toastError } from '@/lib/toast-error';
import type { Project } from '@/types';
import { ASSIGNEE_ALL, ASSIGNEE_ME, BoardFilters, type Filters, matchesFilters } from './board-filters';
import { BoardSkeleton } from './board-skeleton';
import { ColumnVisibilityMenu } from './column-visibility';
import { CreateJiraIssueModal } from './create-issue-modal';
import { JiraIssueDetailModal } from './issue-detail-modal';
import { SprintBoard } from './sprint-board';
import { type BoardColumn, type ConnectedJiraConfig, groupIssues } from './types';
import { useJiraConfig } from './use-jira-config';
import { type BoardOption, useJiraBoards, useJiraSprint } from './use-jira-sprint';

export function ConnectedJiraBoard({ project, config }: { project: Project; config: ConnectedJiraConfig }) {
  const { t } = useTranslation();
  const [openIssueKey, setOpenIssueKey] = useState<string | null>(null);
  const { issues, meta, loading, error, pendingKeys, reload, transition } = useJiraSprint(config);
  const { boards } = useJiraBoards(config);
  const { config: persistedConfig, update: updateConfig } = useJiraConfig(project);

  const [filters, setFilters] = useState<Filters>({ query: '', assignee: config.email ? ASSIGNEE_ME : ASSIGNEE_ALL });

  const selectedBoardId = config.boardId ?? meta?.boardId;

  const hiddenColumnsByBoard = persistedConfig?.hiddenColumnsByBoard ?? {};
  const hiddenColumns = useMemo(() => {
    const k = selectedBoardId != null ? String(selectedBoardId) : null;
    return new Set(k ? (hiddenColumnsByBoard[k] ?? []) : []);
  }, [hiddenColumnsByBoard, selectedBoardId]);

  const selectBoard = (boardId: number | null) => {
    updateConfig((draft) => {
      if (boardId == null) {
        delete draft.boardId;
      } else {
        draft.boardId = boardId;
      }
    });
  };

  const assignees = useMemo(() => {
    const me = config.email?.toLowerCase();
    const set = new Set<string>();
    for (const issue of issues) {
      if (!issue.assignee) {
        continue;
      }
      if (me && issue.assigneeEmail?.toLowerCase() === me) {
        continue;
      }
      set.add(issue.assignee);
    }
    return Array.from(set).sort();
  }, [issues, config.email]);

  const { visibleColumns, columnTargetStatusIds, allColumnNames } = useMemo(() => {
    const allNames = (meta?.columns ?? []).map((c) => c.name);
    const filtered = issues.filter((i) => matchesFilters(i, filters, config.email));
    const grouped: BoardColumn[] = groupIssues(filtered, meta?.columns);
    const targetIds: Record<string, string[]> = {};
    for (const col of meta?.columns ?? []) {
      targetIds[col.name] = col.statusIds ?? [];
    }
    return {
      visibleColumns: grouped.filter((c) => !hiddenColumns.has(c.name)),
      columnTargetStatusIds: targetIds,
      allColumnNames: allNames,
    };
  }, [issues, meta, filters, hiddenColumns, config.email]);

  const baseUrl = config.baseUrl.replace(/\/$/, '');
  const boardUrl = meta?.boardId ? `${baseUrl}/secure/RapidBoard.jspa?rapidView=${meta.boardId}` : null;

  const toggleColumn = (name: string) => {
    if (selectedBoardId == null) {
      return;
    }
    const k = String(selectedBoardId);
    updateConfig((draft) => {
      const map = { ...(draft.hiddenColumnsByBoard ?? {}) };
      const cur = new Set(map[k] ?? []);
      if (cur.has(name)) {
        cur.delete(name);
      } else {
        cur.add(name);
      }
      if (cur.size === 0) {
        delete map[k];
      } else {
        map[k] = Array.from(cur);
      }
      draft.hiddenColumnsByBoard = map;
    });
  };

  const handleDropIssue = (issueKey: string, targetStatusIds: string[]) => {
    transition(issueKey, targetStatusIds).catch((err) => {
      toastError({ title: t('integrations.jira.transitionFailed'), err });
    });
  };

  const hasData = meta !== null;

  return (
    <div className="flex h-full flex-col gap-4 p-4">
      <PageHeader
        icon={<KanbanSquare className="size-5 text-muted-foreground" />}
        title={meta?.name ?? t('integrations.jira.sprintBoard')}
        subtitle={`${project.name} · ${config.projectKey}`}
        actions={
          <>
            <CreateJiraIssueModal config={config} onCreated={reload}>
              <Button size="sm">
                <Plus className="size-3.5" />
                {t('integrations.jira.createIssue')}
              </Button>
            </CreateJiraIssueModal>
            <Separator orientation="vertical" className="h-6" />
            {boardUrl && <OpenExternalAction url={boardUrl} label={t('integrations.jira.openBoard')} />}
            <RefreshAction onRefresh={reload} loading={loading} />
            <ConfigureIntegrationModal projectId={project.id} integrationId="jira">
              <Button variant="outline" size="sm">
                <Settings className="size-3.5" />
                {t('common.configure')}
              </Button>
            </ConfigureIntegrationModal>
          </>
        }
      />

      {hasData && (
        <BoardFilters
          filters={filters}
          assignees={assignees}
          currentUserEmail={config.email}
          onChange={setFilters}
          boardSelector={
            boards.length > 0 ? (
              <Select value={selectedBoardId ? String(selectedBoardId) : ''} onValueChange={(v) => selectBoard(v ? Number(v) : null)}>
                <SelectTrigger size="sm">
                  <SelectValue placeholder={t('integrations.jira.selectBoard')} />
                </SelectTrigger>
                <SelectContent>
                  {boards.map((b: BoardOption) => (
                    <SelectItem key={b.id} value={String(b.id)}>
                      {b.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            ) : null
          }
          columnsMenu={allColumnNames.length > 0 ? <ColumnVisibilityMenu columns={allColumnNames} hidden={hiddenColumns} onToggle={toggleColumn} /> : null}
        />
      )}

      {error && (
        <Card className="border-destructive/60">
          <CardContent className="py-3 text-sm text-destructive">{error}</CardContent>
        </Card>
      )}

      <div className="min-h-0 flex-1 overflow-hidden">
        {!hasData ? <BoardSkeleton /> : issues.length === 0 ? <p className="text-sm text-muted-foreground">{t('integrations.jira.noIssues')}</p> : <SprintBoard columns={visibleColumns} columnTargetStatusIds={columnTargetStatusIds} pendingKeys={pendingKeys} onDropIssue={handleDropIssue} onOpenIssue={setOpenIssueKey} />}
      </div>
      {openIssueKey && <JiraIssueDetailModal config={config} issueKey={openIssueKey} open={true} onOpenChange={(o) => !o && setOpenIssueKey(null)} />}
    </div>
  );
}
