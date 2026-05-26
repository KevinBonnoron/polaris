import { Plug } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { useCurrentProject } from '@/state/projects';
import { ConfigureIntegrationModal } from '../configure-integration-modal';
import { ConnectedDokployDashboard } from './connected-dashboard';
import { type ConnectedDokployConfig, parseProjects } from './types';
import { useDokployConfig } from './use-dokploy-config';

export function DokployPage() {
  const { t } = useTranslation();
  const { project } = useCurrentProject();
  const { config } = useDokployConfig(project);

  const connectedConfig: ConnectedDokployConfig | null = config?.baseUrl && config.apiKey ? { baseUrl: config.baseUrl, apiKey: config.apiKey, projects: parseProjects(config.projects) } : null;

  if (!project) {
    return (
      <div className="flex h-full items-center justify-center p-4">
        <p className="text-sm text-muted-foreground">{t('integrations.dokploy.selectProject')}</p>
      </div>
    );
  }

  if (!connectedConfig) {
    return (
      <div className="flex h-full flex-col gap-6 p-4">
        <header className="flex flex-col gap-1">
          <h1 className="text-2xl font-semibold tracking-tight">{t('integrations.dokploy.title')}</h1>
          <p className="text-sm text-muted-foreground">{t('integrations.dokploy.notConnected', { project: project.name })}</p>
        </header>
        <Card className="border-dashed">
          <CardHeader className="items-center text-center">
            <div className="mb-3 flex size-12 items-center justify-center rounded-full bg-muted">
              <Plug className="size-5 text-muted-foreground" />
            </div>
            <CardTitle className="text-base">{t('integrations.dokploy.connectTitle')}</CardTitle>
            <CardDescription>{t('integrations.dokploy.connectDesc')}</CardDescription>
          </CardHeader>
          <CardContent className="flex justify-center">
            <ConfigureIntegrationModal projectId={project.id} integrationId="dokploy">
              <Button>{t('integrations.dokploy.configureCta')}</Button>
            </ConfigureIntegrationModal>
          </CardContent>
        </Card>
      </div>
    );
  }

  return <ConnectedDokployDashboard project={project} config={connectedConfig} />;
}
