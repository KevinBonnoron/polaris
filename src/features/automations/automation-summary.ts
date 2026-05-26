import type { Automation, AutomationTrigger } from '@/types';

export function triggerSummary(trigger: AutomationTrigger, statusName?: string, fromNames?: string[]): string {
  if (trigger.kind === 'jira.transition') {
    const to = statusName ?? trigger.toStatusId;
    const from = fromNames && fromNames.length > 0 ? fromNames.join(', ') : 'any';
    const assignee = trigger.assignee === 'me' ? '@me' : trigger.assignee === 'any' ? 'anyone' : `@${trigger.assignee}`;
    const reassign = trigger.alsoOnReassignment ? ' · also on reassignment' : '';
    return `${from} → ${to} · ${assignee}${reassign}`;
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
    const scope = trigger.onlyMine === false ? 'any PR' : 'my PRs';
    return `failed build on ${scope}`;
  }

  if (trigger.kind === 'repository.pr_build_success') {
    const scope = trigger.onlyMine === false ? 'any PR' : 'my PRs';
    return `successful build on ${scope}`;
  }

  if (trigger.kind === 'repository.issue_assigned') {
    const scope = trigger.onlyMine === false ? 'anyone' : 'me';
    return `issue assigned to ${scope}`;
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

      if (action.kind === 'jira_transition') {
        return `jira → ${action.jiraToStatusId || '?'}`;
      }

      if (action.kind === 'send_email') {
        return `email → ${action.emailTo || '?'}`;
      }

      return `notify (${action.notifyKind ?? 'info'})`;
    })
    .join(' · ');
}
