import { useTranslation } from 'react-i18next';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { Switch } from '@/components/ui/switch';
import type { JiraStatus } from '@/db/jira-issues';
import { FieldError, isInvalid } from '@/lib/form';
import type { AutomationTrigger, JiraTransitionTrigger, RepositoryIssueAssignedTrigger, RepositoryPullRequestBuildFailedTrigger, RepositoryPullRequestBuildSuccessTrigger, RepositoryPullRequestCommentTrigger, RepositoryPullRequestOpenedTrigger } from '@/types';
import { applyTriggerKind, REPO_TRIGGER_KINDS, type RepoTriggerKind } from './triggers';
import type { AutomationForm } from './types';

interface Props {
  form: AutomationForm;
  hasJira: boolean;
  statuses: JiraStatus[];
  statusesLoading: boolean;
  statusError: string | null;
}

export function TriggerStep({ form, hasJira, statuses, statusesLoading, statusError }: Props) {
  const { t } = useTranslation();

  const switchRepoTriggerKind = (kind: RepoTriggerKind) => applyTriggerKind(form, kind);

  const toggleFromStatus = (ids: string[]) => {
    const trigger = form.state.values.trigger;
    if (trigger.kind !== 'jira.transition') {
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
        const jiraTrigger: JiraTransitionTrigger | null = values.trigger.kind === 'jira.transition' ? (values.trigger as JiraTransitionTrigger) : null;
        const repoOpenedTrigger: RepositoryPullRequestOpenedTrigger | null = values.trigger.kind === 'repository.pr_opened' ? (values.trigger as RepositoryPullRequestOpenedTrigger) : null;
        const repoCommentTrigger: RepositoryPullRequestCommentTrigger | null = values.trigger.kind === 'repository.pr_comment' ? (values.trigger as RepositoryPullRequestCommentTrigger) : null;
        const repoBuildFailedTrigger: RepositoryPullRequestBuildFailedTrigger | null = values.trigger.kind === 'repository.pr_build_failed' ? (values.trigger as RepositoryPullRequestBuildFailedTrigger) : null;
        const repoBuildSuccessTrigger: RepositoryPullRequestBuildSuccessTrigger | null = values.trigger.kind === 'repository.pr_build_success' ? (values.trigger as RepositoryPullRequestBuildSuccessTrigger) : null;
        const repoIssueAssignedTrigger: RepositoryIssueAssignedTrigger | null = values.trigger.kind === 'repository.issue_assigned' ? (values.trigger as RepositoryIssueAssignedTrigger) : null;

        return (
          <div className="flex flex-col gap-4">
            {jiraTrigger && hasJira && (
              <>
                <form.Field
                  name="trigger"
                  validators={{
                    onChange: ({ value }: { value: AutomationTrigger }) => {
                      if (value.kind === 'jira.transition' && !value.toStatusId) {
                        return t('automations.errors.missingTo');
                      }
                      return undefined;
                    },
                  }}
                >
                  {(field) => (
                    <div className="flex flex-col gap-2">
                      <Label>{t('automations.toStatus')}</Label>
                      <Select value={jiraTrigger.toStatusId} onValueChange={(v) => field.handleChange({ ...jiraTrigger, toStatusId: v })} disabled={statusesLoading && statuses.length === 0}>
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
                      const active = s.statusIds.some((id: string) => jiraTrigger.fromStatusIds?.includes(id));
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
                  <Input value={jiraTrigger.assignee} onChange={(e) => form.setFieldValue('trigger', { ...jiraTrigger, assignee: e.target.value })} placeholder={t('automations.assigneePlaceholder')} />
                  <p className="text-xs text-muted-foreground">{t('automations.assigneeHint')}</p>
                </div>

                <div className="flex items-center gap-3">
                  <Switch checked={Boolean(jiraTrigger.alsoOnReassignment)} onCheckedChange={(v) => form.setFieldValue('trigger', { ...jiraTrigger, alsoOnReassignment: v })} />
                  <span className="text-sm">{t('automations.alsoOnReassignment')}</span>
                </div>
              </>
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
          </div>
        );
      }}
    </form.Subscribe>
  );
}
