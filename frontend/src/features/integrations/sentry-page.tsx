import { useTranslation } from 'react-i18next';
import { useCurrentProject } from '@/state/projects';
import { NotConnectedCard, SelectProjectPrompt } from './integration-gate';
import { ConnectedSentryDashboard } from './sentry/connected-dashboard';
import { type ConnectedSentryConfig, parseProjects } from './sentry/types';
import { useSentryConfig } from './sentry/use-sentry-config';

export function SentryPage() {
  const { t } = useTranslation();
  const { project } = useCurrentProject();
  const { config } = useSentryConfig(project);

  const projects = parseProjects(config?.projects);
  const connectedConfig: ConnectedSentryConfig | null = config?.token && config.org && projects.length > 0 ? { token: config.token, org: config.org, projects, url: config.url } : null;

  if (!project) {
    return <SelectProjectPrompt>{t('integrations.sentry.selectProject')}</SelectProjectPrompt>;
  }

  if (!connectedConfig) {
    return (
      <NotConnectedCard
        projectId={project.id}
        integrationId="sentry"
        title={t('integrations.sentry.title')}
        subtitle={t('integrations.sentry.notConnected', { project: project.name })}
        connectTitle={t('integrations.sentry.connectTitle')}
        connectDesc={t('integrations.sentry.connectDesc')}
        cta={t('integrations.sentry.configureCta')}
      />
    );
  }

  return <ConnectedSentryDashboard project={project} config={connectedConfig} />;
}
