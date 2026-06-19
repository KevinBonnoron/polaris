import { Plug } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { useCurrentProject } from '@/state/projects';
import { ConfigureIntegrationModal } from './configure-integration-modal';
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
    return (
      <div className="flex h-full items-center justify-center p-4">
        <p className="text-sm text-muted-foreground">{t('integrations.sentry.selectProject')}</p>
      </div>
    );
  }

  if (!connectedConfig) {
    return (
      <div className="flex h-full flex-col gap-6 p-4">
        <header className="flex flex-col gap-1">
          <h1 className="text-2xl font-semibold tracking-tight">{t('integrations.sentry.title')}</h1>
          <p className="text-sm text-muted-foreground">{t('integrations.sentry.notConnected', { project: project.name })}</p>
        </header>
        <Card className="border-dashed">
          <CardHeader className="items-center text-center">
            <div className="mb-3 flex size-12 items-center justify-center rounded-full bg-muted">
              <Plug className="size-5 text-muted-foreground" />
            </div>
            <CardTitle className="text-base">{t('integrations.sentry.connectTitle')}</CardTitle>
            <CardDescription>{t('integrations.sentry.connectDesc')}</CardDescription>
          </CardHeader>
          <CardContent className="flex justify-center">
            <ConfigureIntegrationModal projectId={project.id} integrationId="sentry">
              <Button>{t('integrations.sentry.configureCta')}</Button>
            </ConfigureIntegrationModal>
          </CardContent>
        </Card>
      </div>
    );
  }

  return <ConnectedSentryDashboard project={project} config={connectedConfig} />;
}
