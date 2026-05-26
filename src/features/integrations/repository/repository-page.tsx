import { GitBranch, GitPullRequest, History, MessageSquare, Settings, Workflow } from 'lucide-react';
import { useCallback, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { OpenExternalAction } from '@/components/atoms/open-external-action';
import { PageHeader } from '@/components/atoms/page-header';
import { RefreshAction } from '@/components/atoms/refresh-action';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { ConfigureIntegrationModal } from '@/features/integrations/configure-integration-modal';
import { getIntegrations } from '@/features/integrations/project-integrations';
import { useCurrentProject } from '@/state/projects';
import { ChangesTab } from './changes-tab';
import { IssuesTab } from './issues-tab';
import { PullRequestsTab } from './pull-requests-tab';
import { RunsTab } from './runs-tab';
import { PROVIDER_LABEL, type RepoConfig } from './types';
import type { ReloadApi } from './use-register-reload';
import { buildRemoteUrl } from './utils';

export function RepositoryPage() {
  const { t } = useTranslation();
  const { project } = useCurrentProject();

  const config = (project ? (getIntegrations(project).repository as RepoConfig | undefined) : undefined) ?? null;

  const [activeTab, setActiveTab] = useState('changes');
  const [reloadApi, setReloadApi] = useState<ReloadApi | null>(null);
  const onRegister = useCallback((api: ReloadApi | null) => setReloadApi(api), []);
  const onTabChange = useCallback((value: string) => {
    setReloadApi(null);
    setActiveTab(value);
  }, []);

  if (!project) {
    return (
      <div className="flex h-full items-center justify-center p-6">
        <p className="text-sm text-muted-foreground">{t('integrations.repository.selectProject')}</p>
      </div>
    );
  }

  const owner = config?.owner ?? '';
  const repo = config?.repo ?? '';
  const canFetch = config?.provider === 'github' && Boolean(owner && repo);
  const providerLabel = config ? (PROVIDER_LABEL[config.provider ?? ''] ?? t('integrations.repository.labelRepository')) : null;
  const slug = owner && repo ? `${owner}/${repo}` : project.name;
  const remoteUrl = config ? buildRemoteUrl(config) : null;

  return (
    <div className="flex h-full flex-col gap-6 overflow-hidden p-6">
      <PageHeader
        icon={<GitBranch className="size-5 text-muted-foreground" />}
        title={slug}
        badges={providerLabel ? <Badge variant="secondary">{providerLabel}</Badge> : undefined}
        subtitle={project.name}
        actions={
          <>
            {remoteUrl && <OpenExternalAction url={remoteUrl} />}
            {canFetch && activeTab !== 'changes' && <RefreshAction onRefresh={reloadApi?.reload ?? (() => {})} loading={reloadApi?.loading} disabled={!reloadApi} />}
            <ConfigureIntegrationModal projectId={project.id} integrationId="repository">
              <Button variant="outline" size="sm">
                <Settings className="size-3.5" />
                {t('common.configure')}
              </Button>
            </ConfigureIntegrationModal>
          </>
        }
      />

      <Tabs value={activeTab} onValueChange={onTabChange} className="flex min-h-0 flex-1 flex-col gap-4">
        <TabsList>
          <TabsTrigger value="changes" className="gap-2">
            <History className="size-4" />
            {t('integrations.repository.changes')}
          </TabsTrigger>
          {canFetch && (
            <>
              <TabsTrigger value="prs" className="gap-2">
                <GitPullRequest className="size-4" />
                {config?.provider === 'gitlab' ? t('integrations.repository.mergeRequests') : t('integrations.repository.pullRequests')}
              </TabsTrigger>
              <TabsTrigger value="issues" className="gap-2">
                <MessageSquare className="size-4" />
                {t('integrations.repository.issues')}
              </TabsTrigger>
              <TabsTrigger value="ci" className="gap-2">
                <Workflow className="size-4" />
                {config?.provider === 'gitlab' ? t('integrations.repository.pipelines') : t('integrations.repository.actions')}
              </TabsTrigger>
            </>
          )}
        </TabsList>

        <TabsContent value="changes" className="m-0 h-0 flex-1">
          <ChangesTab projectId={project.id} />
        </TabsContent>
        {canFetch && (
          <>
            <TabsContent value="prs" className="m-0 h-0 flex-1">
              <PullRequestsTab owner={owner} repo={repo} onRegister={onRegister} />
            </TabsContent>
            <TabsContent value="issues" className="m-0 h-0 flex-1">
              <IssuesTab owner={owner} repo={repo} onRegister={onRegister} />
            </TabsContent>
            <TabsContent value="ci" className="m-0 h-0 flex-1">
              <RunsTab owner={owner} repo={repo} onRegister={onRegister} />
            </TabsContent>
          </>
        )}
      </Tabs>
    </div>
  );
}
