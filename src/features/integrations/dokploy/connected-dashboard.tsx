import { ChevronDown, ChevronRight, Loader2, MousePointerClick, Rocket, Settings } from 'lucide-react';
import { useCallback, useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { OpenExternalAction } from '@/components/atoms/open-external-action';
import { PageHeader } from '@/components/atoms/page-header';
import { RefreshAction } from '@/components/atoms/refresh-action';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import { ScrollArea } from '@/components/ui/scroll-area';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { Skeleton } from '@/components/ui/skeleton';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { ConfigureIntegrationModal } from '@/features/integrations/configure-integration-modal';
import { formatAgo } from '@/lib/format-ago';
import { cn } from '@/lib/utils';
import type { Project } from '@/types';
import { GetDokployDeploymentLogs, GetDokployServiceLogs } from '@/wailsjs/go/main/App';
import { dokploy as dokployModel } from '@/wailsjs/go/models';
import { ServiceActions } from './service-actions';
import { isDeployable, STATUS_VARIANT, serviceIcon } from './status';
import type { ConnectedDokployConfig } from './types';
import type { DokployDeployment, DokployService } from './use-dokploy-dashboard';
import { useDokployDashboard } from './use-dokploy-dashboard';

function epochSeconds(iso: string): number {
  const ms = Date.parse(iso);
  return Number.isNaN(ms) ? 0 : Math.round(ms / 1000);
}

function formatDuration(startedAt: string, finishedAt: string): string | null {
  const start = Date.parse(startedAt);
  const end = Date.parse(finishedAt);
  if (Number.isNaN(start) || Number.isNaN(end) || end < start) {
    return null;
  }
  const secs = Math.round((end - start) / 1000);
  if (secs < 60) {
    return `${secs}s`;
  }
  const m = Math.floor(secs / 60);
  const s = secs % 60;
  return s === 0 ? `${m}m` : `${m}m ${s}s`;
}

type ServiceView = { service: DokployService; deployments: DokployDeployment[] };

function buildServiceViews(services: DokployService[], deployments: DokployDeployment[]): ServiceView[] {
  const byService = new Map<string, DokployDeployment[]>();
  for (const d of deployments) {
    const key = d.applicationId || d.composeId;
    if (!key) {
      continue;
    }
    if (!byService.has(key)) {
      byService.set(key, []);
    }
    (byService.get(key) as DokployDeployment[]).push(d);
  }

  return services
    .map((service) => {
      const list = (byService.get(service.id) ?? []).slice().sort((a, b) => epochSeconds(b.createdAt) - epochSeconds(a.createdAt));
      return { service, deployments: list };
    })
    .sort((a, b) => a.service.name.localeCompare(b.service.name));
}

const STATUS_DOT: Record<string, string> = {
  running: 'bg-green-500',
  done: 'bg-green-500',
  error: 'bg-red-500',
  idle: 'bg-muted-foreground/40',
};

export function ConnectedDokployDashboard({ project, config }: { project: Project; config: ConnectedDokployConfig }) {
  const { t, i18n } = useTranslation();
  const { services, deployments, loading, error, reload } = useDokployDashboard(config);
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [logRefreshKey, setLogRefreshKey] = useState(0);
  const [env, setEnv] = useState<string>('all');

  const environments = useMemo(() => {
    const set = new Set<string>();
    for (const s of services) {
      if (s.environment) {
        set.add(s.environment);
      }
    }
    return Array.from(set).sort();
  }, [services]);

  const filtered = useMemo(() => (env === 'all' ? services : services.filter((s) => s.environment === env)), [services, env]);
  const views = useMemo(() => buildServiceViews(filtered, deployments), [filtered, deployments]);
  const selected = views.find((v) => v.service.id === selectedId) ?? null;

  const handleRefresh = useCallback(() => {
    reload();
    setLogRefreshKey((k) => k + 1);
  }, [reload]);

  return (
    <div className="flex h-full flex-col gap-4 p-6">
      <PageHeader
        icon={<Rocket className="size-5 text-muted-foreground" />}
        title={t('integrations.dokploy.title')}
        subtitle={`${project.name} · ${config.baseUrl.replace(/^https?:\/\//, '')}`}
        badges={<Badge variant="secondary">{t('integrations.dokploy.serviceCount', { count: services.length })}</Badge>}
        actions={
          <>
            {environments.length > 1 && (
              <Select value={env} onValueChange={setEnv}>
                <SelectTrigger size="sm">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="all">{t('integrations.dokploy.allEnvironments')}</SelectItem>
                  {environments.map((e) => (
                    <SelectItem key={e} value={e}>
                      {e}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            )}
            <OpenExternalAction url={config.baseUrl} label={t('integrations.dokploy.openInDokploy')} />
            <RefreshAction onRefresh={handleRefresh} loading={loading} />
            <ConfigureIntegrationModal projectId={project.id} integrationId="dokploy">
              <Button variant="outline" size="sm">
                <Settings className="size-3.5" />
                {t('common.configure')}
              </Button>
            </ConfigureIntegrationModal>
          </>
        }
      />

      {error && (
        <Card className="border-destructive/60">
          <CardContent className="py-3 text-sm text-destructive">{error}</CardContent>
        </Card>
      )}

      {loading && services.length === 0 ? (
        <div className="flex flex-col gap-2">
          {Array.from({ length: 4 }).map((_, i) => (
            // biome-ignore lint/suspicious/noArrayIndexKey: static skeleton placeholders
            <Skeleton key={i} className="h-14 w-full" />
          ))}
        </div>
      ) : views.length === 0 ? (
        <p className="text-sm text-muted-foreground">{t('integrations.dokploy.noServices')}</p>
      ) : (
        <div className="flex min-h-0 flex-1 gap-4">
          <div className="flex w-64 shrink-0 flex-col gap-1 overflow-y-auto pr-1">
            {views.map((sv) => (
              <ServiceCard key={sv.service.id} view={sv} selected={sv.service.id === selectedId} onSelect={() => setSelectedId(sv.service.id)} locale={i18n.language} />
            ))}
          </div>

          <div className="min-h-0 min-w-0 flex-1 overflow-hidden rounded-lg border border-border bg-card/40 p-4">{selected ? <ServiceDetail key={selected.service.id} view={selected} config={config} locale={i18n.language} logRefreshKey={logRefreshKey} onAction={handleRefresh} /> : <NoSelection />}</div>
        </div>
      )}
    </div>
  );
}

function ServiceCard({ view, selected, onSelect, locale }: { view: ServiceView; selected: boolean; onSelect: () => void; locale: string }) {
  const { t } = useTranslation();
  const { service, deployments } = view;
  const Icon = serviceIcon(service.type);
  const latest = deployments[0];

  return (
    <button type="button" onClick={onSelect} className={cn('flex w-full flex-col gap-1 rounded-md px-3 py-2.5 text-left transition-colors', selected ? 'bg-accent' : 'hover:bg-accent/50')}>
      <div className="flex items-center gap-2">
        {service.status && <span className={cn('size-2 shrink-0 rounded-full', STATUS_DOT[service.status] ?? 'bg-muted-foreground/40')} />}
        <Icon className="size-3.5 shrink-0 text-muted-foreground" />
        <span className="min-w-0 flex-1 truncate text-sm font-medium">{service.name}</span>
      </div>
      <div className="flex items-center gap-2 pl-[calc(0.5rem+12px)] text-[11px] text-muted-foreground">
        <span className="capitalize">{service.type}</span>
        {latest && (
          <>
            <span>·</span>
            <span>{formatAgo(epochSeconds(latest.createdAt), locale)}</span>
          </>
        )}
        {!latest && isDeployable(service.type) && <span>· {t('integrations.dokploy.neverDeployed')}</span>}
      </div>
    </button>
  );
}

function NoSelection() {
  const { t } = useTranslation();
  return (
    <div className="flex h-full w-full flex-col items-center justify-center gap-3 text-center text-muted-foreground">
      <div className="flex size-12 items-center justify-center rounded-full bg-muted">
        <MousePointerClick className="size-5" />
      </div>
      <p className="text-sm">{t('integrations.dokploy.selectService')}</p>
    </div>
  );
}

function ServiceDetail({ view, config, locale, logRefreshKey, onAction }: { view: ServiceView; config: ConnectedDokployConfig; locale: string; logRefreshKey: number; onAction: () => void }) {
  const { t } = useTranslation();
  const { service, deployments } = view;
  const deployable = isDeployable(service.type);
  const isApp = service.type === 'application';

  return (
    <div className="flex h-full min-w-0 flex-col gap-4 overflow-hidden">
      <div className="flex items-center gap-2">
        <span className="text-sm font-semibold">{service.name}</span>
        <Badge variant="outline" className="text-[10px] uppercase tracking-wide">
          {service.type}
        </Badge>
        {service.status && (
          <Badge variant={STATUS_VARIANT[service.status] ?? 'outline'} className="capitalize">
            {service.status}
          </Badge>
        )}
        {deployable && (
          <span className="ml-auto">
            <ServiceActions config={config} service={service} onDone={onAction} />
          </span>
        )}
      </div>

      {isApp ? (
        <Tabs defaultValue="deployments" className="flex min-h-0 flex-1 flex-col">
          <TabsList>
            <TabsTrigger value="deployments">{t('integrations.dokploy.tabs.deployments')}</TabsTrigger>
            <TabsTrigger value="logs">{t('integrations.dokploy.tabs.logs')}</TabsTrigger>
          </TabsList>
          <TabsContent value="deployments" forceMount className="min-h-0 min-w-0 flex-1 data-[state=inactive]:hidden">
            <DeploymentsPanel deployments={deployments} deployable={deployable} config={config} locale={locale} />
          </TabsContent>
          <TabsContent value="logs" forceMount className="min-h-0 min-w-0 flex-1 data-[state=inactive]:hidden">
            <ServiceLogsPanel config={config} service={service} refreshKey={logRefreshKey} />
          </TabsContent>
        </Tabs>
      ) : (
        <DeploymentsPanel deployments={deployments} deployable={deployable} config={config} locale={locale} />
      )}
    </div>
  );
}

function DeploymentsPanel({ deployments, deployable, config, locale }: { deployments: DokployDeployment[]; deployable: boolean; config: ConnectedDokployConfig; locale: string }) {
  const { t } = useTranslation();

  if (!deployable) {
    return <p className="text-sm text-muted-foreground">{t('integrations.dokploy.databaseHint')}</p>;
  }
  if (deployments.length === 0) {
    return <p className="text-sm text-muted-foreground">{t('integrations.dokploy.neverDeployed')}</p>;
  }

  return (
    <ScrollArea className="h-full">
      <div className="flex min-w-0 flex-col rounded-md border">
        {deployments.map((d) => (
          <DeploymentRow key={d.id} deployment={d} config={config} locale={locale} />
        ))}
      </div>
    </ScrollArea>
  );
}

function DeploymentRow({ deployment: d, config, locale }: { deployment: DokployDeployment; config: ConnectedDokployConfig; locale: string }) {
  const { t } = useTranslation();
  const [expanded, setExpanded] = useState(false);
  const [logs, setLogs] = useState('');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const duration = formatDuration(d.startedAt, d.finishedAt);

  const toggleLogs = useCallback(async () => {
    if (expanded) {
      setExpanded(false);
      return;
    }
    setExpanded(true);
    if (logs) {
      return;
    }
    setLoading(true);
    setError(null);
    try {
      const apiCfg = dokployModel.Config.createFrom({ baseUrl: config.baseUrl, apiKey: config.apiKey });
      const result = await GetDokployDeploymentLogs(apiCfg, d.id, 500);
      setLogs(result ?? '');
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setLoading(false);
    }
  }, [expanded, logs, config.baseUrl, config.apiKey, d.id]);

  const Chevron = expanded ? ChevronDown : ChevronRight;

  return (
    <div className="border-b last:border-b-0">
      <button type="button" onClick={() => void toggleLogs()} className="flex w-full items-center gap-2 px-3 py-2 text-left text-sm hover:bg-muted/50">
        <Chevron className="size-3.5 shrink-0 text-muted-foreground" />
        <Badge variant={STATUS_VARIANT[d.status] ?? 'outline'} className="w-16 shrink-0 justify-center capitalize">
          {d.status}
        </Badge>
        <span className="min-w-0 flex-1 truncate">{d.title || t('integrations.dokploy.deployment')}</span>
        <span className="shrink-0 text-xs text-muted-foreground">
          {duration ? `${duration} · ` : ''}
          {formatAgo(epochSeconds(d.createdAt), locale)}
        </span>
      </button>
      {d.status === 'error' && d.errorMessage && <p className="truncate px-3 pb-1 pl-10 text-xs text-destructive">{d.errorMessage}</p>}
      {expanded && (
        <div className="min-w-0 px-3 pb-3 pl-10">
          {loading && (
            <div className="flex items-center gap-2 py-2 text-xs text-muted-foreground">
              <Loader2 className="size-3 animate-spin" />
              {t('integrations.dokploy.logs.loading')}
            </div>
          )}
          {error && <p className="py-1 text-xs text-destructive">{error}</p>}
          {!loading && logs && <pre className="max-h-80 overflow-auto whitespace-pre-wrap break-all rounded-md bg-muted p-3 font-mono text-xs leading-relaxed">{logs}</pre>}
          {!loading && !logs && !error && <p className="py-1 text-xs text-muted-foreground">{t('integrations.dokploy.logs.empty')}</p>}
        </div>
      )}
    </div>
  );
}

function ServiceLogsPanel({ config, service, refreshKey }: { config: ConnectedDokployConfig; service: DokployService; refreshKey: number }) {
  const { t } = useTranslation();
  const [logs, setLogs] = useState('');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const fetchLogs = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const apiCfg = dokployModel.Config.createFrom({ baseUrl: config.baseUrl, apiKey: config.apiKey });
      const result = await GetDokployServiceLogs(apiCfg, service, 500);
      setLogs(result ?? '');
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setLoading(false);
    }
  }, [config.baseUrl, config.apiKey, service]);

  useEffect(() => {
    void fetchLogs();
  }, [fetchLogs]);

  return (
    <ScrollArea className="h-full">
      <div className="flex min-w-0 flex-col gap-2">
        {loading && (
          <div className="flex items-center gap-2 py-2 text-xs text-muted-foreground">
            <Loader2 className="size-3 animate-spin" />
            {t('integrations.dokploy.logs.loading')}
          </div>
        )}
        {error && <p className="text-xs text-destructive">{error}</p>}
        {!loading && logs && <pre className="overflow-auto whitespace-pre-wrap break-all rounded-md bg-muted p-3 font-mono text-xs leading-relaxed">{logs}</pre>}
        {!loading && !logs && !error && <p className="text-sm text-muted-foreground">{t('integrations.dokploy.logs.empty')}</p>}
      </div>
    </ScrollArea>
  );
}
