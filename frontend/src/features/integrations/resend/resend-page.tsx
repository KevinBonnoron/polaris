import { CheckCircle2, Mail, Plug, Send, Settings, XCircle } from 'lucide-react';
import { useCallback, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import { PageHeader } from '@/components/atoms/page-header';
import { RefreshAction } from '@/components/atoms/refresh-action';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { ConfigureIntegrationModal } from '@/features/integrations/configure-integration-modal';
import { toastError } from '@/lib/toast-error';
import { useCurrentProject } from '@/state/projects';
import { ListResendDomains, ListResendEmails, SendResendEmail } from '@/wailsjs/go/main/App';
import { resend } from '@/wailsjs/go/models';

interface ResendConfig {
  apiKey?: string;
  fromEmail?: string;
}

interface DashboardData {
  domainVerified: boolean;
  emails: resend.Email[];
}

export function ResendPage() {
  const { t } = useTranslation();
  const { project } = useCurrentProject();
  const config = project?.integrations?.resend as ResendConfig | undefined;
  const isConnected = Boolean(config?.apiKey && config?.fromEmail);

  const [loading, setLoading] = useState(false);
  const [data, setData] = useState<DashboardData | null>(null);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(async (apiKey: string, fromEmail: string) => {
    setLoading(true);
    setError(null);
    try {
      const cfg = resend.Config.createFrom({ apiKey, fromEmail });
      const domain = fromEmail.split('@')[1] ?? '';
      const [domains, emails] = await Promise.all([ListResendDomains(cfg), ListResendEmails(cfg, 50)]);
      setData({
        domainVerified: (domains ?? []).some((d) => d.name === domain && d.status === 'verified'),
        emails: emails ?? [],
      });
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    if (config?.apiKey && config?.fromEmail) {
      load(config.apiKey, config.fromEmail);
    }
  }, [config?.apiKey, config?.fromEmail, load]);

  if (!project) {
    return (
      <div className="flex h-full items-center justify-center p-4">
        <p className="text-sm text-muted-foreground">{t('integrations.resend.selectProject')}</p>
      </div>
    );
  }

  if (!isConnected) {
    return (
      <div className="flex h-full flex-col gap-6 p-4">
        <PageHeader icon={<Mail className="size-5 text-muted-foreground" />} title={t('integrations.resend.title')} subtitle={t('integrations.resend.notConnected', { project: project.name })} />
        <Card className="border-dashed">
          <CardHeader className="items-center text-center">
            <div className="mb-3 flex size-12 items-center justify-center rounded-full bg-muted">
              <Plug className="size-5 text-muted-foreground" />
            </div>
            <CardTitle className="text-base">{t('integrations.resend.connectTitle')}</CardTitle>
            <CardDescription>{t('integrations.resend.connectDesc')}</CardDescription>
          </CardHeader>
          <CardContent className="flex justify-center">
            <ConfigureIntegrationModal projectId={project.id} integrationId="resend">
              <Button>{t('integrations.resend.configureCta')}</Button>
            </ConfigureIntegrationModal>
          </CardContent>
        </Card>
      </div>
    );
  }

  const domainBadge =
    data == null ? null : data.domainVerified ? (
      <Badge variant="default" className="gap-1 text-xs">
        <CheckCircle2 className="size-3" />
        {t('integrations.resend.verified')}
      </Badge>
    ) : (
      <Badge variant="destructive" className="gap-1 text-xs">
        <XCircle className="size-3" />
        {t('integrations.resend.unverified')}
      </Badge>
    );

  return (
    <div className="flex h-full flex-col gap-6 overflow-auto p-4">
      <PageHeader
        icon={<Mail className="size-5 text-muted-foreground" />}
        title={t('integrations.resend.title')}
        subtitle={config?.fromEmail}
        badges={domainBadge}
        actions={
          <div className="flex items-center gap-2">
            <RefreshAction onRefresh={() => load(config?.apiKey ?? '', config?.fromEmail ?? '')} loading={loading} />
            <ConfigureIntegrationModal projectId={project.id} integrationId="resend">
              <Button variant="outline" size="sm">
                <Settings className="size-3.5" />
                {t('common.configure')}
              </Button>
            </ConfigureIntegrationModal>
          </div>
        }
      />

      {error && <p className="text-sm text-destructive">{error}</p>}

      {data && <StatsRow emails={data.emails} />}
      {data && <RecentEmailsCard emails={data.emails.slice(0, 10)} />}

      <SendTestCard apiKey={config?.apiKey ?? ''} fromEmail={config?.fromEmail ?? ''} />
    </div>
  );
}

function eventVariant(event: string): 'default' | 'secondary' | 'destructive' | 'outline' {
  if (event === 'delivered' || event === 'opened' || event === 'clicked') {
    return 'default';
  }
  if (event === 'bounced' || event === 'complained') {
    return 'destructive';
  }
  return 'secondary';
}

function StatsRow({ emails }: { emails: resend.Email[] }) {
  const { t } = useTranslation();
  const total = emails.length;
  const delivered = emails.filter((e) => ['delivered', 'opened', 'clicked'].includes(e.lastEvent)).length;
  const bounced = emails.filter((e) => ['bounced', 'complained'].includes(e.lastEvent)).length;

  return (
    <div className="grid grid-cols-3 gap-4">
      {[
        { label: t('integrations.resend.statTotal'), value: total },
        { label: t('integrations.resend.statDelivered'), value: delivered },
        { label: t('integrations.resend.statBounced'), value: bounced },
      ].map(({ label, value }) => (
        <Card key={label}>
          <CardContent className="px-6 py-4">
            <p className="text-2xl font-semibold">{value}</p>
            <p className="text-xs text-muted-foreground">{label}</p>
          </CardContent>
        </Card>
      ))}
    </div>
  );
}

function RecentEmailsCard({ emails }: { emails: resend.Email[] }) {
  const { t } = useTranslation();

  if (emails.length === 0) {
    return null;
  }

  return (
    <Card>
      <div className="flex items-center gap-2 border-b px-6 py-4">
        <span className="text-sm font-semibold">{t('integrations.resend.recentTitle')}</span>
      </div>
      <div className="divide-y">
        {emails.map((e) => (
          <div key={e.id} className="flex items-center gap-3 px-6 py-2.5">
            <Badge variant={eventVariant(e.lastEvent)} className="w-20 shrink-0 justify-center text-xs capitalize">
              {e.lastEvent || '—'}
            </Badge>
            <span className="min-w-0 flex-1 truncate text-sm">{e.subject || '—'}</span>
            <span className="shrink-0 text-xs text-muted-foreground">{e.to?.[0] ?? '—'}</span>
            <span className="shrink-0 text-xs text-muted-foreground">{formatDate(e.createdAt)}</span>
          </div>
        ))}
      </div>
    </Card>
  );
}

function formatDate(iso: string): string {
  if (!iso) {
    return '—';
  }
  try {
    return new Intl.DateTimeFormat(undefined, { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' }).format(new Date(iso));
  } catch {
    return iso;
  }
}

function SendTestCard({ apiKey, fromEmail }: { apiKey: string; fromEmail: string }) {
  const { t } = useTranslation();
  const [to, setTo] = useState('');
  const [sending, setSending] = useState(false);

  const send = async () => {
    if (!to) {
      return;
    }
    setSending(true);
    try {
      const cfg = resend.Config.createFrom({ apiKey, fromEmail });
      const input = resend.SendInput.createFrom({ to, subject: t('integrations.resend.defaultSubject'), text: t('integrations.resend.defaultBody') });
      await SendResendEmail(cfg, input);
      toast.success(t('integrations.resend.sent'));
      setTo('');
    } catch (err) {
      toastError({ title: t('integrations.resend.sendFailed'), err });
    } finally {
      setSending(false);
    }
  };

  return (
    <Card>
      <div className="flex items-center gap-2 border-b px-6 py-4">
        <span className="text-sm font-semibold">{t('integrations.resend.testTitle')}</span>
        <span className="text-xs text-muted-foreground">{t('integrations.resend.testDesc')}</span>
      </div>
      <div className="px-6 py-3">
        <div className="flex gap-2">
          <Input value={to} onChange={(e) => setTo(e.target.value)} placeholder="recipient@example.com" type="email" className="flex-1" />
          <Button onClick={send} disabled={sending || !to}>
            <Send className="size-4" />
            {sending ? t('integrations.resend.sending') : t('integrations.resend.send')}
          </Button>
        </div>
      </div>
    </Card>
  );
}
