import { Bug, GitBranch, KanbanSquare, Rocket } from 'lucide-react';
import type { Automation, AutomationAction, AutomationSource, AutomationTrigger, Project, TriggerKind } from '@/types';
import type { AutomationForm } from './types';

const AUTOMATABLE_SOURCES: AutomationSource[] = ['tickets', 'repository', 'sentry', 'dokploy'];

export const SOURCE_ICONS: Record<AutomationSource, typeof GitBranch> = {
  tickets: KanbanSquare,
  repository: GitBranch,
  sentry: Bug,
  dokploy: Rocket,
};

type TriggerDef = {
  source: AutomationSource;
  defaultTrigger: () => AutomationTrigger;
  defaultTemplate: string;
  placeholders: string[];
};

export const TRIGGERS: Record<TriggerKind, TriggerDef> = {
  'tickets.transition': {
    source: 'tickets',
    defaultTrigger: () => ({ kind: 'tickets.transition', fromStatusIds: [], toStatusId: '' }),
    defaultTemplate: 'Work on {{key}}: {{summary}}\n\n{{url}}',
    placeholders: ['{{key}}', '{{summary}}', '{{fromStatus}}', '{{toStatus}}', '{{assignee}}', '{{url}}', '{{lastComment}}'],
  },
  'tickets.assigned': {
    source: 'tickets',
    defaultTrigger: () => ({ kind: 'tickets.assigned' }),
    defaultTemplate: 'Work on {{key}}: {{summary}}\n\n{{url}}',
    placeholders: ['{{key}}', '{{summary}}', '{{toStatus}}', '{{assignee}}', '{{url}}', '{{lastComment}}'],
  },
  'repository.pr_opened': {
    source: 'repository',
    defaultTrigger: () => ({ kind: 'repository.pr_opened', includeDrafts: false, authorFilter: '' }),
    defaultTemplate: 'Review PR #{{number}} ({{title}}) by @{{author}}\n\n{{url}}',
    placeholders: ['{{number}}', '{{title}}', '{{author}}', '{{url}}', '{{branch}}', '{{base}}'],
  },
  'repository.pr_comment': {
    source: 'repository',
    defaultTrigger: () => ({ kind: 'repository.pr_comment', excludeOwnComments: true }),
    defaultTemplate: [
      'New comment on your PR #{{number}} ({{title}}) by @{{commentAuthor}}:',
      '',
      '{{comment}}',
      '',
      'PR: {{url}}',
      '',
      'Decide what to do with this feedback:',
      '- If it is not a valid concern or actionable request, do nothing.',
      '- If it is, apply the change in the code and push it.',
      '',
      'When you push, use fixup commits (`git commit --fixup=<sha>`) targeting the original commit so the history can be cleaned up later via `git rebase -i --autosquash`.',
    ].join('\n'),
    placeholders: ['{{number}}', '{{title}}', '{{prAuthor}}', '{{commentAuthor}}', '{{comment}}', '{{url}}'],
  },
  'repository.pr_build_failed': {
    source: 'repository',
    defaultTrigger: () => ({ kind: 'repository.pr_build_failed' }),
    defaultTemplate: [
      'Build {{workflow}} failed on PR #{{number}} ({{title}}) — branch `{{branch}}`.',
      '',
      'Run: {{runUrl}}',
      'PR: {{url}}',
      '',
      'Investigate the failing job, read the logs, and decide:',
      '- If the failure is unrelated to your changes (flaky test, infra), do nothing.',
      '- Otherwise, fix the root cause in the code and push.',
      '',
      'When you push, use fixup commits (`git commit --fixup=<sha>`) targeting the commit that introduced the regression so the history can be cleaned up later via `git rebase -i --autosquash`.',
    ].join('\n'),
    placeholders: ['{{number}}', '{{title}}', '{{workflow}}', '{{conclusion}}', '{{branch}}', '{{url}}', '{{runUrl}}'],
  },
  'repository.pr_build_success': {
    source: 'repository',
    defaultTrigger: () => ({ kind: 'repository.pr_build_success' }),
    defaultTemplate: 'Build {{workflow}} passed on PR #{{number}} ({{title}}) — branch `{{branch}}`.\n\nRun: {{runUrl}}\nPR: {{url}}',
    placeholders: ['{{number}}', '{{title}}', '{{workflow}}', '{{conclusion}}', '{{branch}}', '{{url}}', '{{runUrl}}'],
  },
  'repository.issue_assigned': {
    source: 'repository',
    defaultTrigger: () => ({ kind: 'repository.issue_assigned' }),
    defaultTemplate: 'Issue #{{number}} ({{title}}) was assigned to @{{assignee}}.\n\n{{url}}',
    placeholders: ['{{number}}', '{{title}}', '{{author}}', '{{assignee}}', '{{url}}'],
  },
  'sentry.new_issue': {
    source: 'sentry',
    defaultTrigger: () => ({ kind: 'sentry.new_issue', minLevel: 'error' }),
    defaultTemplate: ['A new {{level}} issue popped up in Sentry project `{{project}}`:', '', '{{title}}', 'Location: {{culprit}}', '', '{{permalink}}', '', 'Investigate the root cause from the stack trace and fix it.'].join('\n'),
    placeholders: ['{{shortId}}', '{{title}}', '{{level}}', '{{culprit}}', '{{project}}', '{{permalink}}'],
  },
  'dokploy.deployment_failed': {
    source: 'dokploy',
    defaultTrigger: () => ({ kind: 'dokploy.deployment_failed' }),
    defaultTemplate: ['Deployment of `{{service}}` (project `{{project}}`) failed.', '', 'Status: {{status}}', '{{errorMessage}}', '', 'Investigate the deployment failure and fix the root cause in the code or the deployment configuration.'].join('\n'),
    placeholders: ['{{project}}', '{{service}}', '{{title}}', '{{status}}', '{{errorMessage}}'],
  },
  'dokploy.deployment_succeeded': {
    source: 'dokploy',
    defaultTrigger: () => ({ kind: 'dokploy.deployment_succeeded' }),
    defaultTemplate: 'Deployment of `{{service}}` (project `{{project}}`) succeeded.',
    placeholders: ['{{project}}', '{{service}}', '{{title}}', '{{status}}'],
  },
};

export const TICKETS_TRIGGER_KINDS = (Object.keys(TRIGGERS) as TriggerKind[]).filter((k) => TRIGGERS[k].source === 'tickets');
export type TicketsTriggerKind = (typeof TICKETS_TRIGGER_KINDS)[number];

export const REPO_TRIGGER_KINDS = (Object.keys(TRIGGERS) as TriggerKind[]).filter((k) => TRIGGERS[k].source === 'repository');
export type RepoTriggerKind = (typeof REPO_TRIGGER_KINDS)[number];

export const DOKPLOY_TRIGGER_KINDS = (Object.keys(TRIGGERS) as TriggerKind[]).filter((k) => TRIGGERS[k].source === 'dokploy');
export type DokployTriggerKind = (typeof DOKPLOY_TRIGGER_KINDS)[number];

export const DEFAULT_KIND_FOR_SOURCE: Record<AutomationSource, TriggerKind> = {
  tickets: 'tickets.transition',
  repository: 'repository.pr_opened',
  sentry: 'sentry.new_issue',
  dokploy: 'dokploy.deployment_failed',
};

export function buildDefaultAutomation(projectId: string, source: AutomationSource, kind: TriggerKind = DEFAULT_KIND_FOR_SOURCE[source]): Automation {
  const def = TRIGGERS[kind];
  return {
    id: '',
    projectId,
    name: '',
    enabled: true,
    source,
    trigger: def.defaultTrigger(),
    actions: [],
    pollIntervalSec: 60,
  };
}

export function defaultActionForKind(actionKind: AutomationAction['kind'], taskTemplate = ''): AutomationAction {
  switch (actionKind) {
    case 'spawn_agent':
      return { kind: 'spawn_agent', agentKind: 'claude-code', model: '', taskTemplate };
    case 'tickets_transition':
      return { kind: 'tickets_transition', ticketsToStatusId: '', ticketsIssueKey: '' };
    case 'notification':
      return { kind: 'notification', notifyTitle: '', notifyKind: 'info' };
    case 'send_email':
      return { kind: 'send_email', emailTo: '', emailSubject: '', emailBody: '' };
    case 'send_message':
      return { kind: 'send_message', messageProvider: 'slack', messageTitle: '', messageBody: '' };
  }
}

export function projectIntegrations(project: Project | null): AutomationSource[] {
  if (!project?.integrations) {
    return [];
  }
  return AUTOMATABLE_SOURCES.filter((s) => Object.hasOwn(project.integrations ?? {}, s));
}

export function applyTriggerKind(form: AutomationForm, kind: TriggerKind): void {
  if (form.state.values.trigger.kind === kind) {
    return;
  }
  const def = TRIGGERS[kind];
  form.setFieldValue('source', def.source);
  form.setFieldValue('trigger', def.defaultTrigger());
  const actions = form.state.values.actions;
  const firstSpawn = actions.findIndex((a) => a.kind === 'spawn_agent');
  const updated = actions.map((a, i) => (i === firstSpawn && a.kind === 'spawn_agent' ? { ...a, taskTemplate: def.defaultTemplate } : a));
  form.setFieldValue('actions', updated);
}
