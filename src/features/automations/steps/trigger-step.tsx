import { useTranslation } from 'react-i18next';
import type { TicketsStatus } from '@/collections/tickets.issues.collection';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { Switch } from '@/components/ui/switch';
import { FieldError, isInvalid } from '@/lib/form';
import type {
  AutomationTrigger,
  RepositoryIssueAssignedTrigger,
  RepositoryPullRequestBuildFailedTrigger,
  RepositoryPullRequestBuildSuccessTrigger,
  RepositoryPullRequestCommentTrigger,
  RepositoryPullRequestOpenedTrigger,
  SentryLevel,
  SentryNewIssueTrigger,
  TicketsAssignedTrigger,
  TicketsTransitionTrigger,
} from '@/types';
import { applyTriggerKind, DOKPLOY_TRIGGER_KINDS, type DokployTriggerKind, REPO_TRIGGER_KINDS, type RepoTriggerKind, TICKETS_TRIGGER_KINDS, type TicketsTriggerKind } from './triggers';
import type { AutomationForm } from './types';

const SENTRY_LEVELS: SentryLevel[] = ['warning', 'error', 'fatal'];

interface Props {
  form: AutomationForm;
  hasTickets: boolean;
  statuses: TicketsStatus[];
  statusesLoading: boolean;
  statusError: string | null;
}

export function TriggerStep({ form, hasTickets, statuses, statusesLoading, statusError }: Props) {
  const { t } = useTranslation();

  const switchTicketsTriggerKind = (kind: TicketsTriggerKind) => applyTriggerKind(form, kind);
  const switchRepoTriggerKind = (kind: RepoTriggerKind) => applyTriggerKind(form, kind);
  const switchDokployTriggerKind = (kind: DokployTriggerKind) => applyTriggerKind(form, kind);

  const toggleFromStatus = (ids: string[]) => {
    const trigger = form.state.values.trigger;
    if (trigger.kind !== 'tickets.transition') {
      return;
    }

    const current = trigger.fromStatusIds ?? [];
    const isActive = ids.some((id) => current.includes(id));
    const idsSet = new Set(ids);
    const next = isActive ? current.filter((x: string) => !idsSet.has(x)) : Array.from(new Set([...current, ...ids]));
    form.setFieldValue('trigger', { ...trigger, fromStatusIds: next });
  };

  return (
    <form.Subscribe selector={(state) => state.values}>
      {(values) => {
        const ticketsTransitionTrigger: TicketsTransitionTrigger | null = values.trigger.kind === 'tickets.transition' ? (values.trigger as TicketsTransitionTrigger) : null;
        const ticketsAssignedTrigger: TicketsAssignedTrigger | null = values.trigger.kind === 'tickets.assigned' ? (values.trigger as TicketsAssignedTrigger) : null;
        const repoOpenedTrigger: RepositoryPullRequestOpenedTrigger | null = values.trigger.kind === 'repository.pr_opened' ? (values.trigger as RepositoryPullRequestOpenedTrigger) : null;
        const repoCommentTrigger: RepositoryPullRequestCommentTrigger | null = values.trigger.kind === 'repository.pr_comment' ? (values.trigger as RepositoryPullRequestCommentTrigger) : null;
        const repoBuildFailedTrigger: RepositoryPullRequestBuildFailedTrigger | null = values.trigger.kind === 'repository.pr_build_failed' ? (values.trigger as RepositoryPullRequestBuildFailedTrigger) : null;
        const repoBuildSuccessTrigger: RepositoryPullRequestBuildSuccessTrigger | null = values.trigger.kind === 'repository.pr_build_success' ? (values.trigger as RepositoryPullRequestBuildSuccessTrigger) : null;
        const repoIssueAssignedTrigger: RepositoryIssueAssignedTrigger | null = values.trigger.kind === 'repository.issue_assigned' ? (values.trigger as RepositoryIssueAssignedTrigger) : null;
        const sentryTrigger: SentryNewIssueTrigger | null = values.trigger.kind === 'sentry.new_issue' ? (values.trigger as SentryNewIssueTrigger) : null;

        return (
          <div className="flex flex-col gap-4">
            {values.source === 'tickets' && hasTickets && (
              <div className="flex flex-col gap-2">
                <Label>{t('automations.ticketsTriggerKind')}</Label>
                <div className="flex flex-wrap gap-2">
                  {TICKETS_TRIGGER_KINDS.map((k) => {
                    const active = values.trigger.kind === k;
                    return (
                      <button key={k} type="button" onClick={() => switchTicketsTriggerKind(k)} className={`rounded-md border px-3 py-1.5 text-sm transition ${active ? 'border-blue-500 bg-blue-500/10 text-blue-200' : 'border-border text-muted-foreground hover:bg-accent'}`}>
                        {t(`automations.ticketsTriggerKinds.${k}`)}
                      </button>
                    );
                  })}
                </div>
              </div>
            )}

            {ticketsTransitionTrigger && hasTickets && (
              <>
                <form.Field
                  name="trigger"
                  validators={{
                    onChange: ({ value }: { value: AutomationTrigger }) => {
                      if (value.kind === 'tickets.transition' && !value.toStatusId) {
                        return t('automations.errors.missingTo');
                      }
                      return undefined;
                    },
                  }}
                >
                  {(field) => (
                    <div className="flex flex-col gap-2">
                      <Label>{t('automations.toStatus')}</Label>
                      <Select value={ticketsTransitionTrigger.toStatusId} onValueChange={(v) => field.handleChange({ ...ticketsTransitionTrigger, toStatusId: v })} disabled={statusesLoading && statuses.length === 0}>
                        <SelectTrigger aria-invalid={isInvalid(field)}>
                          <SelectValue placeholder={statusesLoading && statuses.length === 0 ? t('automations.statusesLoading') : t('automations.toStatusPlaceholder')} />
                        </SelectTrigger>
                        <SelectContent>
                          {statuses.map((s) => (
                            <SelectItem key={s.name} value={s.id}>
                              {s.name}
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                      <FieldError field={field} />
                      {statusError && <p className="text-xs text-destructive">{statusError}</p>}
                    </div>
                  )}
                </form.Field>

                <div className="flex flex-col gap-2">
                  <Label>{t('automations.fromStatus')}</Label>
                  <p className="text-xs text-muted-foreground">{t('automations.fromStatusHint')}</p>
                  <div className="flex flex-wrap gap-1.5">
                    {statuses.map((s) => {
                      const active = s.statusIds.some((id: string) => ticketsTransitionTrigger.fromStatusIds?.includes(id));
                      return (
                        <button key={s.name} type="button" onClick={() => toggleFromStatus(s.statusIds)} className={`rounded-full border px-2.5 py-1 text-xs ${active ? 'border-blue-500 bg-blue-500/10 text-blue-300' : 'border-border text-muted-foreground hover:bg-accent'}`}>
                          {s.name}
                        </button>
                      );
                    })}
                  </div>
                </div>

                <div className="flex flex-col gap-2">
                  <Label>{t('automations.assignee')}</Label>
                  <Input value={ticketsTransitionTrigger.assignee} onChange={(e) => form.setFieldValue('trigger', { ...ticketsTransitionTrigger, assignee: e.target.value })} placeholder={t('automations.assigneePlaceholder')} />
                  <p className="text-xs text-muted-foreground">{t('automations.assigneeHint')}</p>
                </div>
              </>
            )}

            {ticketsAssignedTrigger && hasTickets && (
              <div className="flex flex-col gap-2">
                <Label>{t('automations.assignee')}</Label>
                <Input value={ticketsAssignedTrigger.assignee} onChange={(e) => form.setFieldValue('trigger', { ...ticketsAssignedTrigger, assignee: e.target.value })} placeholder={t('automations.assigneePlaceholder')} />
                <p className="text-xs text-muted-foreground">{t('automations.assignedHint')}</p>
              </div>
            )}

            {values.source === 'repository' && (
              <div className="flex flex-col gap-2">
                <Label>{t('automations.repoTriggerKind')}</Label>
                <div className="flex flex-wrap gap-2">
                  {REPO_TRIGGER_KINDS.map((k) => {
                    const active = values.trigger.kind === k;
                    return (
                      <button key={k} type="button" onClick={() => switchRepoTriggerKind(k)} className={`rounded-md border px-3 py-1.5 text-sm transition ${active ? 'border-blue-500 bg-blue-500/10 text-blue-200' : 'border-border text-muted-foreground hover:bg-accent'}`}>
                        {t(`automations.repoTriggerKinds.${k}`)}
                      </button>
                    );
                  })}
                </div>
              </div>
            )}

            {repoOpenedTrigger && (
              <>
                <div className="flex flex-col gap-2">
                  <Label>{t('automations.repoAuthorFilter')}</Label>
                  <Input value={repoOpenedTrigger.authorFilter ?? ''} onChange={(e) => form.setFieldValue('trigger', { ...repoOpenedTrigger, authorFilter: e.target.value })} placeholder={t('automations.repoAuthorPlaceholder')} />
                  <p className="text-xs text-muted-foreground">{t('automations.repoAuthorHint')}</p>
                </div>

                <div className="flex items-center gap-3">
                  <Switch checked={Boolean(repoOpenedTrigger.includeDrafts)} onCheckedChange={(v) => form.setFieldValue('trigger', { ...repoOpenedTrigger, includeDrafts: v })} />
                  <span className="text-sm">{t('automations.includeDrafts')}</span>
                </div>
              </>
            )}

            {repoCommentTrigger && (
              <div className="flex flex-col gap-2">
                <p className="text-xs text-muted-foreground">{t('automations.repoCommentHint')}</p>
                <div className="flex items-center gap-3">
                  <Switch checked={repoCommentTrigger.excludeOwnComments !== false} onCheckedChange={(v) => form.setFieldValue('trigger', { ...repoCommentTrigger, excludeOwnComments: v })} />
                  <span className="text-sm">{t('automations.excludeOwnComments')}</span>
                </div>
              </div>
            )}

            {repoBuildFailedTrigger && (
              <div className="flex flex-col gap-2">
                <p className="text-xs text-muted-foreground">{t('automations.repoBuildFailedHint')}</p>
                <div className="flex items-center gap-3">
                  <Switch checked={repoBuildFailedTrigger.onlyMine !== false} onCheckedChange={(v) => form.setFieldValue('trigger', { ...repoBuildFailedTrigger, onlyMine: v })} />
                  <span className="text-sm">{t('automations.buildFailedOnlyMine')}</span>
                </div>
              </div>
            )}

            {repoBuildSuccessTrigger && (
              <div className="flex flex-col gap-2">
                <p className="text-xs text-muted-foreground">{t('automations.repoBuildSuccessHint')}</p>
                <div className="flex items-center gap-3">
                  <Switch checked={repoBuildSuccessTrigger.onlyMine !== false} onCheckedChange={(v) => form.setFieldValue('trigger', { ...repoBuildSuccessTrigger, onlyMine: v })} />
                  <span className="text-sm">{t('automations.buildFailedOnlyMine')}</span>
                </div>
              </div>
            )}

            {repoIssueAssignedTrigger && (
              <div className="flex flex-col gap-2">
                <p className="text-xs text-muted-foreground">{t('automations.repoIssueAssignedHint')}</p>
                <div className="flex items-center gap-3">
                  <Switch checked={repoIssueAssignedTrigger.onlyMine !== false} onCheckedChange={(v) => form.setFieldValue('trigger', { ...repoIssueAssignedTrigger, onlyMine: v })} />
                  <span className="text-sm">{t('automations.issueAssignedOnlyMine')}</span>
                </div>
              </div>
            )}

            {sentryTrigger && (
              <div className="flex flex-col gap-2">
                <Label>{t('automations.sentryMinLevel')}</Label>
                <Select value={sentryTrigger.minLevel ?? 'error'} onValueChange={(v) => form.setFieldValue('trigger', { ...sentryTrigger, minLevel: v as SentryLevel })}>
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {SENTRY_LEVELS.map((lvl) => (
                      <SelectItem key={lvl} value={lvl}>
                        {t(`automations.sentryLevels.${lvl}`)}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                <p className="text-xs text-muted-foreground">{t('automations.sentryMinLevelHint')}</p>
              </div>
            )}

            {values.source === 'dokploy' && (
              <div className="flex flex-col gap-2">
                <Label>{t('automations.dokployTriggerKind')}</Label>
                <div className="flex flex-wrap gap-2">
                  {DOKPLOY_TRIGGER_KINDS.map((k) => {
                    const active = values.trigger.kind === k;
                    return (
                      <button key={k} type="button" onClick={() => switchDokployTriggerKind(k)} className={`rounded-md border px-3 py-1.5 text-sm transition ${active ? 'border-blue-500 bg-blue-500/10 text-blue-200' : 'border-border text-muted-foreground hover:bg-accent'}`}>
                        {t(`automations.dokployTriggerKinds.${k}`)}
                      </button>
                    );
                  })}
                </div>
                <p className="text-xs text-muted-foreground">{t('automations.dokployTriggerHint')}</p>
              </div>
            )}
          </div>
        );
      }}
    </form.Subscribe>
  );
}
