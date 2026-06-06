import { useTranslation } from 'react-i18next';
import type { TicketsStatus } from '@/collections/tickets.issues.collection';
import { Label } from '@/components/ui/label';
import type { useAgentClis } from '@/state/agent-clis';
import type { Automation, TicketsTransitionTrigger } from '@/types';

type AgentKindInfo = ReturnType<typeof useAgentClis>['kinds'][number];

interface Props {
  values: Automation;
  agentKinds: AgentKindInfo[];
  statuses: TicketsStatus[];
}

export function ReviewStep({ values, agentKinds, statuses }: Props) {
  const { t } = useTranslation();

  const trigger = values.trigger;
  const ticketsTransition: TicketsTransitionTrigger | null = trigger.kind === 'tickets.transition' ? trigger : null;
  const triggerLabel = trigger.kind === 'tickets.transition' || trigger.kind === 'tickets.assigned' ? t(`automations.ticketsTriggerKinds.${trigger.kind}`) : t(`automations.repoTriggerKinds.${trigger.kind}`);

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

        {ticketsTransition && (
          <>
            <ReviewRow label={t('automations.toStatus')} value={statuses.find((s) => s.statusIds.includes(ticketsTransition.toStatusId))?.name ?? ticketsTransition.toStatusId ?? '—'} />
            <ReviewRow
              label={t('automations.fromStatus')}
              value={
                ticketsTransition.fromStatusIds && ticketsTransition.fromStatusIds.length > 0
                  ? statuses
                      .filter((s) => s.statusIds.some((id) => ticketsTransition.fromStatusIds?.includes(id)))
                      .map((s) => s.name)
                      .join(', ') || ticketsTransition.fromStatusIds.join(', ')
                  : t('automations.review.anyStatus')
              }
            />
          </>
        )}

        {trigger.kind === 'repository.pr_opened' && (
          <>
            <ReviewRow label={t('automations.repoAuthorFilter')} value={trigger.authorFilter || t('automations.review.anyAuthor')} />
            <ReviewRow label="" value={trigger.includeDrafts ? t('automations.includeDrafts') : t('automations.review.noDrafts')} />
          </>
        )}

        {trigger.kind === 'repository.pr_comment' && <ReviewRow label="" value={t('automations.excludeOwnComments')} />}
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

function ActionReview({ index, action, agentKinds, statuses }: { index: number; action: Automation['actions'][number]; agentKinds: AgentKindInfo[]; statuses: TicketsStatus[] }) {
  const { t } = useTranslation();
  if (action.kind === 'resume_pr_agent') {
    return (
      <div className="rounded-md border bg-muted/20 px-3 py-2">
        <div className="text-xs font-medium">
          #{index + 1} · {t('automations.actions.kinds.resume_pr_agent')}
        </div>
        <pre className="mt-1 max-h-32 overflow-auto font-mono text-[11px] whitespace-pre-wrap">{action.taskTemplate}</pre>
      </div>
    );
  }
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
  if (action.kind === 'tickets_transition') {
    const statusName = statuses.find((s) => s.statusIds.includes(action.ticketsToStatusId))?.name ?? action.ticketsToStatusId;
    return (
      <div className="rounded-md border bg-muted/20 px-3 py-2">
        <div className="text-xs font-medium">
          #{index + 1} · {t('automations.actions.kinds.tickets_transition')} → {statusName}
        </div>
        {action.ticketsIssueKey && <div className="mt-1 font-mono text-[11px] text-muted-foreground">{action.ticketsIssueKey}</div>}
      </div>
    );
  }
  if (action.kind === 'send_email') {
    return (
      <div className="rounded-md border bg-muted/20 px-3 py-2">
        <div className="text-xs font-medium">
          #{index + 1} · {t('automations.actions.kinds.send_email')} → {action.emailTo || '?'}
        </div>
        {action.emailSubject && <div className="mt-1 font-mono text-[11px] text-muted-foreground">{action.emailSubject}</div>}
      </div>
    );
  }
  if (action.kind === 'send_message') {
    return (
      <div className="rounded-md border bg-muted/20 px-3 py-2">
        <div className="text-xs font-medium">
          #{index + 1} · {t('automations.actions.kinds.send_message')} → {action.messageProvider}
        </div>
        {action.messageTitle && <div className="mt-1 font-mono text-[11px] text-muted-foreground">{action.messageTitle}</div>}
      </div>
    );
  }
  if (action.kind === 'trigger_workflow') {
    return (
      <div className="rounded-md border bg-muted/20 px-3 py-2">
        <div className="text-xs font-medium">
          #{index + 1} · {t('automations.actions.kinds.trigger_workflow')} · {action.workflowFile || '?'}@{action.workflowRef || '?'}
        </div>
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
