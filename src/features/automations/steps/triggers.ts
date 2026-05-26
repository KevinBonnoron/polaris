import { GitBranch, KanbanSquare } from 'lucide-react';
import type { Automation, AutomationAction, AutomationSource, AutomationTrigger, Project, TriggerKind } from '@/types';
import type { AutomationForm } from './types';

const AUTOMATABLE_SOURCES: AutomationSource[] = ['jira', 'repository'];

export const SOURCE_ICONS: Record<AutomationSource, typeof GitBranch> = {
  jira: KanbanSquare,
  repository: GitBranch,
};

type TriggerDef = {
  source: AutomationSource;
  defaultTrigger: () => AutomationTrigger;
  defaultTemplate: string;
  placeholders: string[];
};

export const TRIGGERS: Record<TriggerKind, TriggerDef> = {
  'jira.transition': {
    source: 'jira',
    defaultTrigger: () => ({ kind: 'jira.transition', fromStatusIds: [], toStatusId: '', assignee: 'me', alsoOnReassignment: false }),
    defaultTemplate: 'Work on {{key}}: {{summary}}\n\n{{url}}',
    placeholders: ['{{key}}', '{{summary}}', '{{fromStatus}}', '{{toStatus}}', '{{assignee}}', '{{url}}', '{{lastComment}}'],
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
    defaultTrigger: () => ({ kind: 'repository.pr_build_failed', onlyMine: true }),
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
    defaultTrigger: () => ({ kind: 'repository.pr_build_success', onlyMine: true }),
    defaultTemplate: 'Build {{workflow}} passed on PR #{{number}} ({{title}}) — branch `{{branch}}`.\n\nRun: {{runUrl}}\nPR: {{url}}',
    placeholders: ['{{number}}', '{{title}}', '{{workflow}}', '{{conclusion}}', '{{branch}}', '{{url}}', '{{runUrl}}'],
  },
  'repository.issue_assigned': {
    source: 'repository',
    defaultTrigger: () => ({ kind: 'repository.issue_assigned', onlyMine: true }),
    defaultTemplate: 'Issue #{{number}} ({{title}}) was assigned to @{{assignee}}.\n\n{{url}}',
    placeholders: ['{{number}}', '{{title}}', '{{author}}', '{{assignee}}', '{{url}}'],
  },
};

export const REPO_TRIGGER_KINDS = (Object.keys(TRIGGERS) as TriggerKind[]).filter((k) => TRIGGERS[k].source === 'repository');
export type RepoTriggerKind = (typeof REPO_TRIGGER_KINDS)[number];

export const DEFAULT_KIND_FOR_SOURCE: Record<AutomationSource, TriggerKind> = {
  jira: 'jira.transition',
  repository: 'repository.pr_opened',
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
    case 'jira_transition':
      return { kind: 'jira_transition', jiraToStatusId: '', jiraIssueKey: '' };
    case 'notification':
      return { kind: 'notification', notifyTitle: '', notifyKind: 'info' };
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
