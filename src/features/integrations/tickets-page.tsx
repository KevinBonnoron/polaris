import { Plug } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { useCurrentProject } from '@/state/projects';
import { ConfigureIntegrationModal } from './configure-integration-modal';
import { ConnectedTicketsBoard } from './tickets/connected-board';
import type { ConnectedTicketsConfig } from './tickets/types';
import { useTicketsConfig } from './tickets/use-tickets-config';

export function TicketsPage() {
  const { t } = useTranslation();
  const { project } = useCurrentProject();
  const { config } = useTicketsConfig(project);

  const connectedConfig: ConnectedTicketsConfig | null = config?.baseUrl && config.email && config.token && config.projectKey ? { baseUrl: config.baseUrl, email: config.email, token: config.token, projectKey: config.projectKey, boardId: config.boardId } : null;

  if (!project) {
    return (
      <div className="flex h-full items-center justify-center p-4">
        <p className="text-sm text-muted-foreground">{t('integrations.tickets.selectProject')}</p>
      </div>
    );
  }

  if (!connectedConfig) {
    return (
      <div className="flex h-full flex-col gap-6 p-4">
        <header className="flex flex-col gap-1">
          <h1 className="text-2xl font-semibold tracking-tight">{t('integrations.tickets.title')}</h1>
          <p className="text-sm text-muted-foreground">{t('integrations.tickets.notConnected', { project: project.name })}</p>
        </header>
        <Card className="border-dashed">
          <CardHeader className="items-center text-center">
            <div className="mb-3 flex size-12 items-center justify-center rounded-full bg-muted">
              <Plug className="size-5 text-muted-foreground" />
            </div>
            <CardTitle className="text-base">{t('integrations.tickets.connectTitle')}</CardTitle>
            <CardDescription>{t('integrations.tickets.connectDesc')}</CardDescription>
          </CardHeader>
          <CardContent className="flex justify-center">
            <ConfigureIntegrationModal projectId={project.id} integrationId="tickets">
              <Button>{t('integrations.tickets.configureCta')}</Button>
            </ConfigureIntegrationModal>
          </CardContent>
        </Card>
      </div>
    );
  }

  return <ConnectedTicketsBoard project={project} config={connectedConfig} />;
}
