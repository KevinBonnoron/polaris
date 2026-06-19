import { Archive, Bug, CheckCircle2, ChevronDown, ChevronRight, Copy, ExternalLink, Loader2, Settings, Users } from 'lucide-react';
import { useCallback, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import { OpenExternalAction } from '@/components/atoms/open-external-action';
import { PageHeader } from '@/components/atoms/page-header';
import { RefreshAction } from '@/components/atoms/refresh-action';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import { ScrollArea } from '@/components/ui/scroll-area';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { Skeleton } from '@/components/ui/skeleton';
import { ConfigureIntegrationModal } from '@/features/integrations/configure-integration-modal';
import { formatAgo } from '@/lib/format-ago';
import { sentryLevelSeverity, severityBadgeClassName } from '@/lib/severity';
import { cn } from '@/lib/utils';
import type { Project } from '@/types';
import { FetchSentryLatestEvent, UpdateSentryIssueStatus } from '@/wailsjs/go/main/App';
import { sentry } from '@/wailsjs/go/models';
import { BrowserOpenURL } from '@/wailsjs/runtime/runtime';
import type { ConnectedSentryConfig } from './types';
import { type SentryIssue, useSentryIssues } from './use-sentry-issues';

const ALL = 'all';

export function ConnectedSentryDashboard({ project, config }: { project: Project; config: ConnectedSentryConfig }) {
  const { t, i18n } = useTranslation();
  const { issues, loading, error, reload } = useSentryIssues(config);
  const [level, setLevel] = useState<string>(ALL);
  const [projectSlug, setProjectSlug] = useState<string>(ALL);

  const base = (config.url ?? 'https://sentry.io').replace(/\/$/, '');
  const issuesUrl = `${base}/organizations/${config.org}/issues/`;

  const levels = useMemo(() => {
    const set = new Set<string>();
    for (const i of issues) {
      if (i.level) {
        set.add(i.level);
      }
    }
    return Array.from(set).sort();
  }, [issues]);

  const visible = useMemo(() => issues.filter((i) => (level === ALL || i.level === level) && (projectSlug === ALL || i.projectSlug === projectSlug)), [issues, level, projectSlug]);

  return (
    <div className="flex h-full flex-col gap-4 p-6">
      <PageHeader
        icon={<Bug className="size-5 text-muted-foreground" />}
        title={t('integrations.sentry.title')}
        subtitle={`${project.name} · ${config.org} · ${t('integrations.sentry.projectCount', { count: config.projects.length })}`}
        badges={<Badge variant="secondary">{t('integrations.sentry.unresolvedCount', { count: visible.length })}</Badge>}
        actions={
          <>
            <OpenExternalAction url={issuesUrl} label={t('integrations.sentry.openInSentry')} />
            <RefreshAction onRefresh={reload} loading={loading} />
            <ConfigureIntegrationModal projectId={project.id} integrationId="sentry">
              <Button variant="outline" size="sm">
                <Settings className="size-3.5" />
                {t('common.configure')}
              </Button>
            </ConfigureIntegrationModal>
          </>
        }
      />

      <div className="flex flex-wrap items-center gap-2">
        {config.projects.length > 1 && (
          <Select value={projectSlug} onValueChange={setProjectSlug}>
            <SelectTrigger size="sm">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value={ALL}>{t('integrations.sentry.allProjects')}</SelectItem>
              {config.projects.map((p) => (
                <SelectItem key={p} value={p}>
                  {p}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        )}
        {levels.length > 1 && (
          <Select value={level} onValueChange={setLevel}>
            <SelectTrigger size="sm">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value={ALL}>{t('integrations.sentry.allLevels')}</SelectItem>
              {levels.map((l) => (
                <SelectItem key={l} value={l}>
                  {l}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        )}
      </div>

      {error && (
        <Card className="border-destructive/60">
          <CardContent className="py-3 text-sm text-destructive">{error}</CardContent>
        </Card>
      )}

      <div className="min-h-0 flex-1 overflow-y-auto">
        {loading && issues.length === 0 ? (
          <div className="flex flex-col gap-2">
            {Array.from({ length: 6 }).map((_, i) => (
              // biome-ignore lint/suspicious/noArrayIndexKey: static skeleton placeholders
              <Skeleton key={i} className="h-12 w-full" />
            ))}
          </div>
        ) : visible.length === 0 ? (
          <p className="text-sm text-muted-foreground">{t('integrations.sentry.noIssues')}</p>
        ) : (
          <ScrollArea className="h-full" viewportProps={{ className: '[&>div]:!block' }}>
            <div className="flex min-w-0 flex-col rounded-md border">
              {visible.map((issue) => (
                <SentryIssueRow key={`${issue.projectSlug}:${issue.id}`} issue={issue} config={config} showProject={config.projects.length > 1} locale={i18n.language} onChanged={reload} />
              ))}
            </div>
          </ScrollArea>
        )}
      </div>
    </div>
  );
}

function SentryIssueRow({ issue, config, showProject, locale, onChanged }: { issue: SentryIssue; config: ConnectedSentryConfig; showProject: boolean; locale: string; onChanged: () => void }) {
  const { t } = useTranslation();
  const [expanded, setExpanded] = useState(false);
  const [event, setEvent] = useState<sentry.EventDetail | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [acting, setActing] = useState(false);

  const issueConfig = useCallback(() => sentry.Config.createFrom({ token: config.token, org: config.org, project: issue.projectSlug, url: config.url }), [config.token, config.org, config.url, issue.projectSlug]);

  const updateStatus = useCallback(
    async (status: 'resolved' | 'ignored') => {
      setActing(true);
      try {
        await UpdateSentryIssueStatus(issueConfig(), issue.id, status);
        toast.success(t(status === 'resolved' ? 'integrations.sentry.detail.resolved' : 'integrations.sentry.detail.archived'));
        onChanged();
      } catch (err) {
        toast.error(t('integrations.sentry.detail.actionError', { error: err instanceof Error ? err.message : String(err) }));
      } finally {
        setActing(false);
      }
    },
    [issueConfig, issue.id, onChanged, t],
  );

  const copyDetails = useCallback(async () => {
    await navigator.clipboard.writeText(formatIssueDetails(issue, event));
    toast.success(t('integrations.sentry.detail.copied'));
  }, [issue, event, t]);

  const toggleExpand = useCallback(async () => {
    if (expanded) {
      setExpanded(false);
      return;
    }
    setExpanded(true);
    if (event) {
      return;
    }
    setLoading(true);
    setError(null);
    try {
      const result = await FetchSentryLatestEvent(sentry.Config.createFrom({ token: config.token, org: config.org, project: issue.projectSlug, url: config.url }), issue.id);
      setEvent(result);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setLoading(false);
    }
  }, [expanded, event, config.token, config.org, config.url, issue.projectSlug, issue.id]);

  const Chevron = expanded ? ChevronDown : ChevronRight;

  return (
    <div className="border-b last:border-b-0">
      <button type="button" onClick={() => void toggleExpand()} className="flex w-full items-center gap-2 px-3 py-2 text-left text-sm hover:bg-muted/50">
        <Chevron className="size-3.5 shrink-0 text-muted-foreground" />
        <Badge className={cn('shrink-0 capitalize', severityBadgeClassName(sentryLevelSeverity(issue.level)))}>{issue.level || 'error'}</Badge>
        {showProject && (
          <Badge variant="outline" className="shrink-0">
            {issue.projectSlug}
          </Badge>
        )}
        <span className="min-w-0 flex-1 truncate font-medium">{issue.title}</span>
        {issue.shortId && <span className="shrink-0 font-mono text-xs text-muted-foreground">{issue.shortId}</span>}
        <span className="flex shrink-0 items-center gap-3 text-xs text-muted-foreground">
          <span>{t('integrations.sentry.events', { count: issue.count })}</span>
          {issue.userCount > 0 && (
            <span className="flex items-center gap-1">
              <Users className="size-3" />
              {issue.userCount}
            </span>
          )}
          <span>{formatAgo(Math.round(issue.lastSeen / 1000), locale)}</span>
        </span>
      </button>

      {expanded && (
        <div className="min-w-0 px-3 pb-3 pl-10">
          {issue.culprit && <p className="mb-2 truncate text-xs text-muted-foreground">{issue.culprit}</p>}

          <div className="mb-3 flex flex-wrap gap-x-6 gap-y-1 text-xs text-muted-foreground">
            <span>{t('integrations.sentry.events', { count: issue.count })}</span>
            <span>{t('integrations.sentry.detail.users', { count: issue.userCount })}</span>
            <span>
              {t('integrations.sentry.detail.firstSeen')}: {formatAgo(Math.round(issue.firstSeen / 1000), locale)}
            </span>
            <span>
              {t('integrations.sentry.detail.lastSeen')}: {formatAgo(Math.round(issue.lastSeen / 1000), locale)}
            </span>
          </div>

          {loading && (
            <div className="flex items-center gap-2 py-2 text-xs text-muted-foreground">
              <Loader2 className="size-3 animate-spin" />
              {t('common.loading')}
            </div>
          )}
          {error && <p className="py-1 text-xs text-destructive">{error}</p>}
          {!loading && event && <EventBody event={event} />}

          <div className="mt-3 flex flex-wrap items-center justify-end gap-2">
            <Button variant="outline" size="xs" onClick={() => void copyDetails()}>
              <Copy className="size-3" />
              {t('integrations.sentry.detail.copyDetails')}
            </Button>
            <Button variant="outline" size="xs" disabled={acting} onClick={() => void updateStatus('ignored')}>
              <Archive className="size-3" />
              {t('integrations.sentry.detail.archive')}
            </Button>
            <Button variant="outline" size="xs" disabled={acting} onClick={() => void updateStatus('resolved')}>
              <CheckCircle2 className="size-3" />
              {t('integrations.sentry.detail.resolve')}
            </Button>
            <Button variant="outline" size="xs" onClick={() => BrowserOpenURL(issue.permalink)}>
              {t('integrations.sentry.openInSentry')}
              <ExternalLink className="size-3" />
            </Button>
          </div>
        </div>
      )}
    </div>
  );
}

function EventBody({ event }: { event: sentry.EventDetail }) {
  return (
    <div className="flex flex-col gap-3">
      {event.exceptions?.map((ex, i) => (
        // biome-ignore lint/suspicious/noArrayIndexKey: exception order is stable within an event
        <div key={i} className="flex flex-col gap-2">
          <div className="text-sm">
            <span className="font-mono font-medium">{ex.type}</span>
            {ex.value && <span className="text-muted-foreground">: {ex.value}</span>}
          </div>
          {ex.frames && ex.frames.length > 0 && (
            <div className="overflow-hidden rounded-md border">
              {[...ex.frames].reverse().map((f, j) => (
                // biome-ignore lint/suspicious/noArrayIndexKey: frame order is stable
                <div key={j} className={cn('flex items-baseline gap-2 px-3 py-1.5 font-mono text-xs', f.inApp ? 'bg-card' : 'bg-muted/40 text-muted-foreground')}>
                  <span className="truncate">{f.filename || '?'}</span>
                  {f.lineNo > 0 && <span className="shrink-0 text-muted-foreground">:{f.lineNo}</span>}
                  {f.function && <span className="ml-auto shrink-0 truncate text-muted-foreground">{f.function}</span>}
                </div>
              ))}
            </div>
          )}
        </div>
      ))}
    </div>
  );
}

function formatIssueDetails(issue: SentryIssue, event: sentry.EventDetail | null): string {
  const iso = (ms: number) => (ms > 0 ? new Date(ms).toISOString() : '?');
  const lines = [
    `# Sentry issue: ${issue.title}`,
    '',
    `- Level: ${issue.level || 'error'}`,
    `- Project: ${issue.projectSlug}`,
    issue.shortId ? `- Short ID: ${issue.shortId}` : '',
    issue.culprit ? `- Culprit: ${issue.culprit}` : '',
    `- Events: ${issue.count} · Users: ${issue.userCount}`,
    `- First seen: ${iso(issue.firstSeen)} · Last seen: ${iso(issue.lastSeen)}`,
    issue.permalink ? `- Permalink: ${issue.permalink}` : '',
  ].filter(Boolean);

  for (const ex of event?.exceptions ?? []) {
    lines.push('', `## ${ex.type}${ex.value ? `: ${ex.value}` : ''}`);
    const frames = [...(ex.frames ?? [])].reverse();
    if (frames.length > 0) {
      lines.push('```');
      for (const f of frames) {
        const loc = `${f.filename || '?'}${f.lineNo > 0 ? `:${f.lineNo}` : ''}`;
        lines.push(`${loc}${f.function ? ` in ${f.function}` : ''}`);
      }
      lines.push('```');
    }
  }
  return lines.join('\n');
}
