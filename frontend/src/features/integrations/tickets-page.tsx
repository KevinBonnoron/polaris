import { useTranslation } from 'react-i18next';
import { useCurrentProject } from '@/state/projects';
import { NotConnectedCard, SelectProjectPrompt } from './integration-gate';
import { ConnectedTicketsBoard } from './tickets/connected-board';
import type { ConnectedTicketsConfig } from './tickets/types';
import { useTicketsConfig } from './tickets/use-tickets-config';

export function TicketsPage() {
  const { t } = useTranslation();
  const { project } = useCurrentProject();
  const { config } = useTicketsConfig(project);

  const connectedConfig: ConnectedTicketsConfig | null = config?.baseUrl && config.email && config.token && config.projectKey ? { baseUrl: config.baseUrl, email: config.email, token: config.token, projectKey: config.projectKey, boardId: config.boardId } : null;

  if (!project) {
    return <SelectProjectPrompt>{t('integrations.tickets.selectProject')}</SelectProjectPrompt>;
  }

  if (!connectedConfig) {
    return (
      <NotConnectedCard
        projectId={project.id}
        integrationId="tickets"
        title={t('integrations.tickets.title')}
        subtitle={t('integrations.tickets.notConnected', { project: project.name })}
        connectTitle={t('integrations.tickets.connectTitle')}
        connectDesc={t('integrations.tickets.connectDesc')}
        cta={t('integrations.tickets.configureCta')}
      />
    );
  }

  return <ConnectedTicketsBoard project={project} config={connectedConfig} />;
}
