import { Plug, Send, Settings } from 'lucide-react';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import { PageHeader } from '@/components/atoms/page-header';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { ConfigureIntegrationModal } from '@/features/integrations/configure-integration-modal';
import { findIntegration } from '@/features/integrations/integration-catalog';
import { toastError } from '@/lib/toast-error';
import { useCurrentProject } from '@/state/projects';
import { TestMessagingProvider } from '@/wailsjs/go/main/App';

interface Props {
  integrationId: 'slack' | 'discord' | 'telegram';
}

export function MessagingPage({ integrationId }: Props) {
  const { t } = useTranslation();
  const { project } = useCurrentProject();
  const integration = findIntegration(integrationId);
  const config = project?.integrations?.[integrationId] as Record<string, string> | undefined;

  const isConnected = integrationId === 'telegram' ? Boolean(config?.token && config?.channel) : Boolean(config?.webhook);

  const [testing, setTesting] = useState(false);

  if (!project || !integration) {
    return null;
  }

  const Icon = integration.icon;
  const providerName = integration.name;

  const sendTest = async () => {
    setTesting(true);
    try {
      await TestMessagingProvider(integrationId, project.id);
      toast.success(t('integrations.messaging.sent'));
    } catch (err) {
      toastError({ title: t('integrations.messaging.sendFailed'), err });
    } finally {
      setTesting(false);
    }
  };

  if (!isConnected) {
    return (
      <div className="flex h-full flex-col gap-6 p-4">
        <PageHeader icon={<Icon className="size-5 text-muted-foreground" />} title={providerName} subtitle={t('integrations.messaging.connectDesc', { provider: providerName })} />
        <Card className="border-dashed">
          <CardHeader className="items-center text-center">
            <div className="mb-3 flex size-12 items-center justify-center rounded-full bg-muted">
              <Plug className="size-5 text-muted-foreground" />
            </div>
            <CardTitle className="text-base">{t('integrations.messaging.configureCta', { provider: providerName })}</CardTitle>
            <CardDescription>{t('integrations.messaging.connectDesc', { provider: providerName })}</CardDescription>
          </CardHeader>
          <CardContent className="flex justify-center">
            <ConfigureIntegrationModal projectId={project.id} integrationId={integrationId}>
              <Button>{t('integrations.messaging.configureCta', { provider: providerName })}</Button>
            </ConfigureIntegrationModal>
          </CardContent>
        </Card>
      </div>
    );
  }

  const subtitle = integrationId === 'telegram' ? `Chat ID: ${config?.channel}` : config?.webhook?.replace(/\/[^/]+$/, '/...');

  return (
    <div className="flex h-full flex-col gap-6 p-4">
      <PageHeader
        icon={<Icon className="size-5 text-muted-foreground" />}
        title={providerName}
        subtitle={subtitle}
        actions={
          <ConfigureIntegrationModal projectId={project.id} integrationId={integrationId}>
            <Button variant="outline" size="sm">
              <Settings className="size-3.5" />
              {t('common.configure')}
            </Button>
          </ConfigureIntegrationModal>
        }
      />

      <Card>
        <CardHeader>
          <CardTitle className="text-sm font-medium">{t('integrations.messaging.testTitle')}</CardTitle>
          <CardDescription>{t('integrations.messaging.testDesc', { provider: providerName })}</CardDescription>
        </CardHeader>
        <CardContent>
          <Button onClick={sendTest} disabled={testing} size="sm">
            <Send className="size-3.5" />
            {testing ? t('integrations.messaging.sending') : t('integrations.messaging.send')}
          </Button>
        </CardContent>
      </Card>
    </div>
  );
}
