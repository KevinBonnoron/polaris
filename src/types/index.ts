export type IntegrationConfig = Record<string, unknown>;

export type ProjectIntegrations = Record<string, IntegrationConfig>;

export type Project = {
  id: string;
  name: string;
  color: string;
  logo?: string;
  path?: string;
  branch?: string;
  integrations?: ProjectIntegrations;
  agentsNum?: number;
  hasGit?: boolean;
  isolatedDefault?: boolean;
  branchPrefix?: string;
  updatedAt?: number;
};

export type AgentStatus = 'working' | 'idle' | 'waiting' | 'error' | 'completed';

export type AgentKind = 'claude-code' | 'copilot' | 'codex' | 'gemini' | 'mistral' | 'other';

export type Agent = {
  id: string;
  projectId: string;
  kind: AgentKind;
  summary?: string;
  status: AgentStatus | string;
  startedAt: number;
  tokens: number;
  tokensInput?: number;
  tokensOutput?: number;
  tokensCacheCreate?: number;
  tokensCacheRead?: number;
  sessionId?: string;
  source?: string;
  costUsd?: number;
  filesModified?: number;
  toolsUsed?: number;
  branch?: string;
  worktreePath?: string;
  issueKey?: string;
  prUrl?: string;
  model?: string;
  pendingQuestionId?: string;
  pendingQuestionInput?: string;
};

export type NotificationSeverity = 'info' | 'success' | 'warning' | 'error';

export type AgentEvent = 'waiting' | 'completed';

export type AgentNotificationPayload = {
  agentId: string;
  event: AgentEvent;
};

export type AutomationNotificationPayload = {
  automationId?: string;
  agentId?: string;
};

type NotificationBase = {
  id: string;
  projectId: string;
  severity: NotificationSeverity;
  title: string;
  createdAt: number;
  read: boolean;
};

export type Notification = (NotificationBase & { type: 'agent'; payload: AgentNotificationPayload }) | (NotificationBase & { type: 'automation'; payload: AutomationNotificationPayload }) | (NotificationBase & { type: 'user'; payload?: Record<string, unknown> });

export type CustomProviderApiType = 'OpenAI-compatible' | 'Anthropic-compatible' | 'Custom (raw HTTP)';

export type CustomProvider = {
  id: string;
  name: string;
  color: string;
  endpoint: string;
  apiKey: string;
  apiType: CustomProviderApiType | string;
  models: string[];
};

export type AutomationSource = 'jira' | 'repository';

export type JiraTransitionTrigger = {
  kind: 'jira.transition';
  fromStatusIds?: string[];
  toStatusId: string;
  assignee: 'me' | 'any' | string;
  alsoOnReassignment?: boolean;
};

export type RepositoryPullRequestOpenedTrigger = {
  kind: 'repository.pr_opened';
  includeDrafts?: boolean;
  authorFilter?: string;
};

export type RepositoryPullRequestCommentTrigger = {
  kind: 'repository.pr_comment';
  excludeOwnComments?: boolean;
};

export type RepositoryPullRequestBuildFailedTrigger = {
  kind: 'repository.pr_build_failed';
  onlyMine?: boolean;
};

export type RepositoryPullRequestBuildSuccessTrigger = {
  kind: 'repository.pr_build_success';
  onlyMine?: boolean;
};

export type RepositoryIssueAssignedTrigger = {
  kind: 'repository.issue_assigned';
  onlyMine?: boolean;
};

export type AutomationTrigger = JiraTransitionTrigger | RepositoryPullRequestOpenedTrigger | RepositoryPullRequestCommentTrigger | RepositoryPullRequestBuildFailedTrigger | RepositoryPullRequestBuildSuccessTrigger | RepositoryIssueAssignedTrigger;

export type TriggerKind = AutomationTrigger['kind'];

export type ActionKind = 'spawn_agent' | 'jira_transition' | 'notification';

export type SpawnAgentAction = {
  kind: 'spawn_agent';
  agentKind: string;
  model?: string;
  taskTemplate: string;
};

export type JiraTransitionAction = {
  kind: 'jira_transition';
  jiraToStatusId: string;
  jiraIssueKey?: string;
};

export type NotificationAction = {
  kind: 'notification';
  notifyTitle: string;
  notifyKind?: NotificationSeverity;
};

export type AutomationAction = SpawnAgentAction | JiraTransitionAction | NotificationAction;

export type Automation = {
  id: string;
  projectId: string;
  name: string;
  enabled: boolean;
  source: AutomationSource;
  trigger: AutomationTrigger;
  actions: AutomationAction[];
  pollIntervalSec: number;
  snapshotJson?: string;
};
