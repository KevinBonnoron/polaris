import { ExternalLink } from 'lucide-react';
import type { ComponentProps, PropsWithChildren } from 'react';
import { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import { Button } from '@/components/ui/button';
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogTrigger } from '@/components/ui/dialog';
import { ScrollArea } from '@/components/ui/scroll-area';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { type DialogModeProps, useDialogMode } from '@/lib/use-dialog-mode';
import { FetchJiraIssueDetail, ListJiraIssueComments, ListJiraIssueHistory } from '@/wailsjs/go/main/App';
import { tickets } from '@/wailsjs/go/models';
import { BrowserOpenURL } from '@/wailsjs/runtime/runtime';
import type { ConnectedJiraConfig } from './types';

interface Props extends DialogModeProps {
  config: ConnectedJiraConfig;
  issueKey: string;
}

export function JiraIssueDetailModal({ config, issueKey, children, ...modeProps }: PropsWithChildren<Props>) {
  const { t, i18n } = useTranslation();
  const { open, setOpen } = useDialogMode(modeProps);

  const [detail, setDetail] = useState<tickets.IssueDetail | null>(null);
  const [comments, setComments] = useState<tickets.Comment[] | null>(null);
  const [history, setHistory] = useState<tickets.HistoryEntry[] | null>(null);
  const [detailErr, setDetailErr] = useState<string | null>(null);
  const [commentsErr, setCommentsErr] = useState<string | null>(null);
  const [historyErr, setHistoryErr] = useState<string | null>(null);
  const [detailLoading, setDetailLoading] = useState(true);
  const [commentsLoading, setCommentsLoading] = useState(true);
  const [historyLoading, setHistoryLoading] = useState(true);

  const formatDate = useMemo(() => makeDateFormatter(i18n.language), [i18n.language]);

  useEffect(() => {
    if (!open || !issueKey) {
      return;
    }
    let cancelled = false;
    const cfg = tickets.Config.createFrom(config);
    setDetailLoading(true);
    setCommentsLoading(true);
    setHistoryLoading(true);
    FetchJiraIssueDetail(cfg, issueKey)
      .then((d) => {
        if (!cancelled) {
          setDetail(d);
        }
      })
      .catch((err) => {
        if (!cancelled) {
          setDetailErr(err instanceof Error ? err.message : String(err));
        }
      })
      .finally(() => {
        if (!cancelled) {
          setDetailLoading(false);
        }
      });
    ListJiraIssueComments(cfg, issueKey)
      .then((list) => {
        if (!cancelled) {
          setComments(list ?? []);
        }
      })
      .catch((err) => {
        if (!cancelled) {
          setCommentsErr(err instanceof Error ? err.message : String(err));
        }
      })
      .finally(() => {
        if (!cancelled) {
          setCommentsLoading(false);
        }
      });
    ListJiraIssueHistory(cfg, issueKey)
      .then((list) => {
        if (!cancelled) {
          setHistory(list ?? []);
        }
      })
      .catch((err) => {
        if (!cancelled) {
          setHistoryErr(err instanceof Error ? err.message : String(err));
        }
      })
      .finally(() => {
        if (!cancelled) {
          setHistoryLoading(false);
        }
      });
    return () => {
      cancelled = true;
    };
  }, [config, issueKey, open]);

  const url = detail?.url ?? `${config.baseUrl.replace(/\/$/, '')}/browse/${issueKey}`;

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      {children !== undefined && <DialogTrigger asChild>{children}</DialogTrigger>}
      <DialogContent className="flex max-h-[85vh] w-[min(95vw,960px)] flex-col gap-4 sm:max-w-[960px]">
        <DialogHeader>
          <div className="flex items-start justify-between gap-3 pr-8">
            <div className="flex min-w-0 flex-col gap-1">
              <DialogTitle className="text-base">{t('integrations.jira.detail.title', { key: issueKey })}</DialogTitle>
              {detail && <p className="text-sm text-muted-foreground">{detail.summary}</p>}
            </div>
            <Button variant="outline" size="sm" onClick={() => BrowserOpenURL(url)}>
              {t('integrations.jira.openBoard')} <ExternalLink className="size-3.5" />
            </Button>
          </div>
        </DialogHeader>

        <Tabs defaultValue="details" className="flex min-h-0 flex-1 flex-col">
          <TabsList>
            <TabsTrigger value="details">{t('integrations.jira.detail.tabDetails')}</TabsTrigger>
            <TabsTrigger value="comments">
              {t('integrations.jira.detail.tabComments')}
              {comments && comments.length > 0 && <span className="ml-1 text-muted-foreground">({comments.length})</span>}
            </TabsTrigger>
            <TabsTrigger value="history">
              {t('integrations.jira.detail.tabHistory')}
              {history && history.length > 0 && <span className="ml-1 text-muted-foreground">({history.length})</span>}
            </TabsTrigger>
          </TabsList>

          <TabsContent value="details" className="min-h-0">
            <ScrollArea className="h-[60vh] pr-3">
              {detailLoading && <p className="text-sm text-muted-foreground">{t('integrations.jira.detail.loading')}</p>}
              {detailErr && <p className="text-sm text-destructive">{detailErr}</p>}
              {detail && <DetailBody detail={detail} formatDate={formatDate} />}
            </ScrollArea>
          </TabsContent>

          <TabsContent value="comments" className="min-h-0">
            <ScrollArea className="h-[60vh] pr-3">
              {commentsLoading && <p className="text-sm text-muted-foreground">{t('integrations.jira.detail.loadingComments')}</p>}
              {commentsErr && <p className="text-sm text-destructive">{commentsErr}</p>}
              {comments && comments.length === 0 && !commentsLoading && <p className="text-sm text-muted-foreground">{t('integrations.jira.detail.noComments')}</p>}
              {comments && comments.length > 0 && (
                <ul className="flex flex-col gap-3">
                  {comments.map((c) => (
                    <li key={c.id} className="rounded-md border border-border/60 bg-card/40 p-3">
                      <div className="mb-2 flex items-center justify-between gap-2">
                        <span className="text-sm font-medium">{c.author || '—'}</span>
                        <span className="text-xs text-muted-foreground">{formatDate(c.createdAt)}</span>
                      </div>
                      <Markdown source={c.body} />
                    </li>
                  ))}
                </ul>
              )}
            </ScrollArea>
          </TabsContent>

          <TabsContent value="history" className="min-h-0">
            <ScrollArea className="h-[60vh] pr-3">
              {historyLoading && <p className="text-sm text-muted-foreground">{t('integrations.jira.detail.loadingHistory')}</p>}
              {historyErr && <p className="text-sm text-destructive">{historyErr}</p>}
              {history && history.length === 0 && !historyLoading && <p className="text-sm text-muted-foreground">{t('integrations.jira.detail.noHistory')}</p>}
              {history && history.length > 0 && (
                <ul className="flex flex-col gap-3">
                  {history.map((entry) => (
                    <li key={entry.id} className="rounded-md border border-border/60 bg-card/40 p-3">
                      <div className="mb-2 flex items-center justify-between gap-2">
                        <span className="text-sm font-medium">{entry.author || '—'}</span>
                        <span className="text-xs text-muted-foreground">{formatDate(entry.createdAt)}</span>
                      </div>
                      <ul className="flex flex-col gap-1 text-sm text-foreground/85">
                        {entry.changes.map((change) => (
                          <li key={`${entry.id}-${change.field}-${change.from}-${change.to}`}>
                            <ChangeLine change={change} />
                          </li>
                        ))}
                      </ul>
                    </li>
                  ))}
                </ul>
              )}
            </ScrollArea>
          </TabsContent>
        </Tabs>
      </DialogContent>
    </Dialog>
  );
}

function ChangeLine({ change }: { change: tickets.HistoryChange }) {
  return (
    <span className="flex flex-wrap items-center gap-1.5">
      <span className="font-medium">{change.field}</span>
      <span className="text-muted-foreground">:</span>
      {change.from && <span className="max-w-full whitespace-pre-wrap break-words rounded bg-muted px-1.5 py-0.5 text-xs">{change.from}</span>}
      {change.from && change.to && <span className="text-muted-foreground">→</span>}
      {change.to && <span className="max-w-full whitespace-pre-wrap break-words rounded bg-muted px-1.5 py-0.5 text-xs">{change.to}</span>}
      {!change.from && !change.to && <span className="italic text-muted-foreground">—</span>}
    </span>
  );
}

function DetailBody({ detail, formatDate }: { detail: tickets.IssueDetail; formatDate: (unix: number) => string }) {
  const { t } = useTranslation();
  return (
    <div className="flex flex-col gap-4">
      <section className="grid grid-cols-2 gap-x-4 gap-y-2 text-sm">
        <Field label={t('integrations.jira.detail.status')} value={detail.status || '—'} />
        <Field label={t('integrations.jira.detail.priority')} value={detail.priority || '—'} />
        <Field label={t('integrations.jira.issueType')} value={detail.issueType || '—'} />
        <Field label={t('integrations.jira.detail.assignee')} value={detail.assignee || '—'} />
        <Field label={t('integrations.jira.detail.reporter')} value={detail.reporter || '—'} />
        <Field label={t('integrations.jira.detail.created')} value={formatDate(detail.createdAt)} />
        <Field label={t('integrations.jira.detail.updated')} value={formatDate(detail.updatedAt)} />
        {detail.labels && detail.labels.length > 0 && (
          <div className="col-span-2 flex flex-col gap-1">
            <span className="text-xs uppercase tracking-wide text-muted-foreground">{t('integrations.jira.detail.labels')}</span>
            <div className="flex flex-wrap gap-1">
              {detail.labels.map((l) => (
                <span key={l} className="rounded bg-muted px-2 py-0.5 text-xs text-muted-foreground">
                  {l}
                </span>
              ))}
            </div>
          </div>
        )}
      </section>

      <section className="flex flex-col gap-2">
        <span className="text-xs uppercase tracking-wide text-muted-foreground">{t('integrations.jira.detail.description')}</span>
        {detail.description ? <Markdown source={detail.description} /> : <p className="text-sm text-muted-foreground">{t('integrations.jira.detail.noDescription')}</p>}
      </section>
    </div>
  );
}

function Field({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex flex-col gap-0.5">
      <span className="text-xs uppercase tracking-wide text-muted-foreground">{label}</span>
      <span className="text-sm text-foreground">{value}</span>
    </div>
  );
}

const mdComponents: ComponentProps<typeof ReactMarkdown>['components'] = {
  p: ({ children }) => <p className="mb-2 text-sm text-foreground/85 last:mb-0">{children}</p>,
  h1: ({ children }) => <h1 className="mb-2 text-base font-semibold text-foreground">{children}</h1>,
  h2: ({ children }) => <h2 className="mb-1.5 text-sm font-semibold text-foreground">{children}</h2>,
  h3: ({ children }) => <h3 className="mb-1 text-sm font-semibold text-foreground">{children}</h3>,
  ul: ({ children }) => <ul className="mb-2 list-disc pl-5 last:mb-0 [&>li]:mb-0.5">{children}</ul>,
  ol: ({ children }) => <ol className="mb-2 list-decimal pl-5 last:mb-0 [&>li]:mb-0.5">{children}</ol>,
  li: ({ children }) => <li className="text-sm text-foreground/85">{children}</li>,
  a: ({ href, children }) => (
    <a
      href={href}
      className="text-blue-400 underline"
      onClick={(e) => {
        if (href) {
          e.preventDefault();
          BrowserOpenURL(href);
        }
      }}
    >
      {children}
    </a>
  ),
  code: ({ className, children, ...props }) => {
    const isBlock = className?.startsWith('language-');
    if (isBlock) {
      return (
        <code className={className} {...props}>
          {children}
        </code>
      );
    }
    return (
      <code className="rounded bg-muted px-1 py-0.5 font-mono text-xs text-foreground/90" {...props}>
        {children}
      </code>
    );
  },
  pre: ({ children }) => <pre className="mb-2 overflow-x-auto rounded bg-muted p-3 font-mono text-xs last:mb-0">{children}</pre>,
  blockquote: ({ children }) => <blockquote className="mb-2 border-l-2 border-border pl-3 text-sm text-muted-foreground last:mb-0">{children}</blockquote>,
  hr: () => <hr className="my-2 border-border" />,
  strong: ({ children }) => <strong className="font-semibold text-foreground">{children}</strong>,
  em: ({ children }) => <em className="italic">{children}</em>,
};

function Markdown({ source }: { source: string }) {
  if (!source) {
    return <p className="text-sm text-muted-foreground">—</p>;
  }
  return (
    <div className="text-sm leading-relaxed">
      <ReactMarkdown remarkPlugins={[remarkGfm]} components={mdComponents}>
        {source}
      </ReactMarkdown>
    </div>
  );
}

function makeDateFormatter(language: string) {
  const fmt = new Intl.DateTimeFormat(language, { dateStyle: 'medium', timeStyle: 'short' });
  return (unix: number) => {
    if (!unix) {
      return '—';
    }
    return fmt.format(new Date(unix * 1000));
  };
}
