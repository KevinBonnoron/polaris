import { Plug } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { useCurrentProject } from '@/state/projects';
import { ConfigureIntegrationModal } from './configure-integration-modal';
import { ConnectedJiraBoard } from './jira/connected-board';
import type { ConnectedJiraConfig } from './jira/types';
import { useJiraConfig } from './jira/use-jira-config';

export function JiraPage() {
  const { t } = useTranslation();
  const { project } = useCurrentProject();
  const { config } = useJiraConfig(project);

  const connectedConfig: ConnectedJiraConfig | null = config?.baseUrl && config.email && config.token && config.projectKey ? { baseUrl: config.baseUrl, email: config.email, token: config.token, projectKey: config.projectKey, boardId: config.boardId } : null;

  if (!project) {
    return (
      <div className="flex h-full items-center justify-center p-4">
        <p className="text-sm text-muted-foreground">{t('integrations.jira.selectProject')}</p>
      </div>
    );
  }

  if (!connectedConfig) {
    return (
      <div className="flex h-full flex-col gap-6 p-4">
        <header className="flex flex-col gap-1">
          <h1 className="text-2xl font-semibold tracking-tight">{t('integrations.jira.title')}</h1>
          <p className="text-sm text-muted-foreground">{t('integrations.jira.notConnected', { project: project.name })}</p>
        </header>
        <Card className="border-dashed">
          <CardHeader className="items-center text-center">
            <div className="mb-3 flex size-12 items-center justify-center rounded-full bg-muted">
              <Plug className="size-5 text-muted-foreground" />
            </div>
            <CardTitle className="text-base">{t('integrations.jira.connectTitle')}</CardTitle>
            <CardDescription>{t('integrations.jira.connectDesc')}</CardDescription>
          </CardHeader>
          <CardContent className="flex justify-center">
            <ConfigureIntegrationModal projectId={project.id} integrationId="jira">
              <Button>{t('integrations.jira.configureCta')}</Button>
            </ConfigureIntegrationModal>
          </CardContent>
        </Card>
      </div>
    );
  }

  return <ConnectedJiraBoard project={project} config={connectedConfig} />;
}
