import { useTranslation } from 'react-i18next';
import { useCurrentProject } from '@/state/projects';
import { NotConnectedCard, SelectProjectPrompt } from '../integration-gate';
import { ConnectedDokployDashboard } from './connected-dashboard';
import { type ConnectedDokployConfig, parseProjects } from './types';
import { useDokployConfig } from './use-dokploy-config';

export function DokployPage() {
  const { t } = useTranslation();
  const { project } = useCurrentProject();
  const { config } = useDokployConfig(project);

  const connectedConfig: ConnectedDokployConfig | null = config?.baseUrl && config.apiKey ? { baseUrl: config.baseUrl, apiKey: config.apiKey, projects: parseProjects(config.projects) } : null;

  if (!project) {
    return <SelectProjectPrompt>{t('integrations.dokploy.selectProject')}</SelectProjectPrompt>;
  }

  if (!connectedConfig) {
    return (
      <NotConnectedCard
        projectId={project.id}
        integrationId="dokploy"
        title={t('integrations.dokploy.title')}
        subtitle={t('integrations.dokploy.notConnected', { project: project.name })}
        connectTitle={t('integrations.dokploy.connectTitle')}
        connectDesc={t('integrations.dokploy.connectDesc')}
        cta={t('integrations.dokploy.configureCta')}
      />
    );
  }

  return <ConnectedDokployDashboard project={project} config={connectedConfig} />;
}
