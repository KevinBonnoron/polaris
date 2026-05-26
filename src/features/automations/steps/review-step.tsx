import { useTranslation } from 'react-i18next';
import { Label } from '@/components/ui/label';
import type { JiraStatus } from '@/db/jira-issues';
import type { useAgentClis } from '@/state/agent-clis';
import type { Automation, JiraTransitionTrigger } from '@/types';

type AgentKindInfo = ReturnType<typeof useAgentClis>['kinds'][number];

interface Props {
  values: Automation;
  agentKinds: AgentKindInfo[];
  statuses: JiraStatus[];
}

export function ReviewStep({ values, agentKinds, statuses }: Props) {
  const { t } = useTranslation();

  const trigger = values.trigger;
  const jiraTrigger: JiraTransitionTrigger | null = trigger.kind === 'jira.transition' ? trigger : null;
  const triggerLabel = trigger.kind === 'jira.transition' ? t('automations.review.triggerJira') : t(`automations.repoTriggerKinds.${trigger.kind}`);

  return (
    <div className="flex flex-col gap-3">
      <div>
        <h3 className="text-sm font-medium">{t('automations.review.title')}</h3>
        <p className="text-xs text-muted-foreground">{t('automations.review.desc')}</p>
      </div>

      <dl className="grid grid-cols-[max-content_1fr] gap-x-4 gap-y-2 rounded-md border bg-muted/20 px-4 py-3 text-sm">
        <ReviewRow label={t('automations.name')} value={values.name || '—'} />
        <ReviewRow label={t('automations.integration')} value={t(`automations.sources.${values.source}`)} />
        <ReviewRow label={t('automations.review.trigger')} value={triggerLabel} />

        {jiraTrigger && (
          <>
            <ReviewRow label={t('automations.toStatus')} value={statuses.find((s) => s.statusIds.includes(jiraTrigger.toStatusId))?.name ?? jiraTrigger.toStatusId ?? '—'} />
            <ReviewRow
              label={t('automations.fromStatus')}
              value={
                jiraTrigger.fromStatusIds && jiraTrigger.fromStatusIds.length > 0
                  ? statuses
                      .filter((s) => s.statusIds.some((id) => jiraTrigger.fromStatusIds?.includes(id)))
                      .map((s) => s.name)
                      .join(', ') || jiraTrigger.fromStatusIds.join(', ')
                  : t('automations.review.anyStatus')
              }
            />
            <ReviewRow label={t('automations.assignee')} value={jiraTrigger.assignee || '—'} />
            {jiraTrigger.alsoOnReassignment && <ReviewRow label="" value={t('automations.alsoOnReassignment')} />}
          </>
        )}

        {trigger.kind === 'repository.pr_opened' && (
          <>
            <ReviewRow label={t('automations.repoAuthorFilter')} value={trigger.authorFilter || t('automations.review.anyAuthor')} />
            <ReviewRow label="" value={trigger.includeDrafts ? t('automations.includeDrafts') : t('automations.review.noDrafts')} />
          </>
        )}

        {trigger.kind === 'repository.pr_comment' && <ReviewRow label="" value={trigger.excludeOwnComments !== false ? t('automations.excludeOwnComments') : t('automations.review.includeOwnComments')} />}

        {trigger.kind === 'repository.pr_build_failed' && <ReviewRow label="" value={trigger.onlyMine !== false ? t('automations.buildFailedOnlyMine') : t('automations.review.anyPr')} />}
        {trigger.kind === 'repository.pr_build_success' && <ReviewRow label="" value={trigger.onlyMine !== false ? t('automations.buildFailedOnlyMine') : t('automations.review.anyPr')} />}
        {trigger.kind === 'repository.issue_assigned' && <ReviewRow label="" value={trigger.onlyMine !== false ? t('automations.issueAssignedOnlyMine') : t('automations.review.anyAssignee')} />}
      </dl>

      <div className="flex flex-col gap-2">
        <Label className="text-xs text-muted-foreground">{t('automations.actions.title')}</Label>
        <div className="flex flex-col gap-2">
          {values.actions.map((action, idx) => (
            // biome-ignore lint/suspicious/noArrayIndexKey: actions are an ordered list with no stable id
            <ActionReview key={idx} index={idx} action={action} agentKinds={agentKinds} statuses={statuses} />
          ))}
        </div>
      </div>
    </div>
  );
}

function ActionReview({ index, action, agentKinds, statuses }: { index: number; action: Automation['actions'][number]; agentKinds: AgentKindInfo[]; statuses: JiraStatus[] }) {
  const { t } = useTranslation();
  if (action.kind === 'spawn_agent') {
    const agent = agentKinds.find((k) => k.id === action.agentKind);
    const model = agent?.models.find((m) => m.value === action.model);
    return (
      <div className="rounded-md border bg-muted/20 px-3 py-2">
        <div className="text-xs font-medium">
          #{index + 1} · {t('automations.actions.kinds.spawn_agent')} · {agent?.label ?? action.agentKind}
          {model ? ` · ${model.label}` : ''}
        </div>
        <pre className="mt-1 max-h-32 overflow-auto font-mono text-[11px] whitespace-pre-wrap">{action.taskTemplate}</pre>
      </div>
    );
  }
  if (action.kind === 'jira_transition') {
    const statusName = statuses.find((s) => s.statusIds.includes(action.jiraToStatusId))?.name ?? action.jiraToStatusId;
    return (
      <div className="rounded-md border bg-muted/20 px-3 py-2">
        <div className="text-xs font-medium">
          #{index + 1} · {t('automations.actions.kinds.jira_transition')} → {statusName}
        </div>
        {action.jiraIssueKey && <div className="mt-1 font-mono text-[11px] text-muted-foreground">{action.jiraIssueKey}</div>}
      </div>
    );
  }
  return (
    <div className="rounded-md border bg-muted/20 px-3 py-2">
      <div className="text-xs font-medium">
        #{index + 1} · {t('automations.actions.kinds.notification')} · {t(`automations.actions.notifyKinds.${action.notifyKind ?? 'info'}`)}
      </div>
      <div className="mt-1 font-mono text-[11px] whitespace-pre-wrap">{action.notifyTitle}</div>
    </div>
  );
}

interface ReviewRowProps {
  label: string;
  value: string;
}

function ReviewRow({ label, value }: ReviewRowProps) {
  return (
    <>
      <dt className="text-xs text-muted-foreground">{label}</dt>
      <dd className="text-sm">{value}</dd>
    </>
  );
}
