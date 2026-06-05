import type { Automation, AutomationTrigger } from '@/types';

export function triggerSummary(trigger: AutomationTrigger, statusName?: string, fromNames?: string[]): string {
  if (trigger.kind === 'tickets.transition') {
    const to = statusName ?? trigger.toStatusId;
    const from = fromNames && fromNames.length > 0 ? fromNames.join(', ') : 'any';
    return `${from} → ${to}`;
  }

  if (trigger.kind === 'tickets.assigned') {
    return 'assigned to me';
  }

  if (trigger.kind === 'repository.pr_opened') {
    const author = trigger.authorFilter ? `@${trigger.authorFilter}` : 'any author';
    const drafts = trigger.includeDrafts ? ' · incl. drafts' : '';
    return `new PR · ${author}${drafts}`;
  }

  if (trigger.kind === 'repository.pr_comment') {
    const excl = trigger.excludeOwnComments === false ? '' : ' · excl. mine';
    return `comment on my PRs${excl}`;
  }

  if (trigger.kind === 'repository.pr_build_failed') {
    return 'failed build on my PRs';
  }

  if (trigger.kind === 'repository.pr_build_success') {
    return 'successful build on my PRs';
  }

  if (trigger.kind === 'repository.issue_assigned') {
    return 'issue assigned to me';
  }

  if (trigger.kind === 'sentry.new_issue') {
    return `new issue · ${trigger.minLevel ?? 'any'}+`;
  }

  if (trigger.kind === 'dokploy.deployment_failed') {
    return 'failed deployment';
  }

  if (trigger.kind === 'dokploy.deployment_succeeded') {
    return 'successful deployment';
  }

  return (trigger as AutomationTrigger).kind;
}

export function actionsSummary(a: Automation): string {
  if (a.actions.length === 0) {
    return '—';
  }
  return a.actions
    .map((action) => {
      if (action.kind === 'spawn_agent') {
        const model = action.model ? ` (${action.model})` : '';
        return `${action.agentKind}${model}`;
      }

      if (action.kind === 'tickets_transition') {
        return `tickets → ${action.ticketsToStatusId || '?'}`;
      }

      if (action.kind === 'send_email') {
        return `email → ${action.emailTo || '?'}`;
      }

      if (action.kind === 'send_message') {
        return `message → ${action.messageProvider || '?'}`;
      }

      return `notify (${action.notifyKind ?? 'info'})`;
    })
    .join(' · ');
}
