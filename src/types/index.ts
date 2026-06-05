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

export type AgentStatus = 'working' | 'idle' | 'waiting' | 'error' | 'completed' | 'draft';

export type AgentKind = 'claude-code' | 'copilot' | 'codex' | 'gemini' | 'mistral' | 'cursor' | 'opencode' | 'other';

// SpawnTarget identifies what a "new agent" spawn runs against: either a
// built-in CLI kind, or a user-defined custom provider (run via opencode).
// Exactly one field is set.
export type SpawnTarget = { kindId?: AgentKind; providerId?: string };

export type TokenUsage = {
  input: number;
  output: number;
  cacheCreation: number;
  cacheRead: number;
};

export type AgentWorktree = {
  branch?: string;
  path?: string;
  issueKey?: string;
  prUrl?: string;
};

export type AgentPendingQuestion = {
  toolUseId?: string;
  input?: unknown;
};

export type Agent = {
  id: string;
  projectId: string;
  kind: AgentKind;
  summary?: string;
  status: AgentStatus | string;
  startedAt: number;
  updatedAt?: number;
  tokens: TokenUsage;
  sessionId?: string;
  source?: string;
  costUsd?: number;
  filesModified?: number;
  toolsUsed?: number;
  worktree?: AgentWorktree;
  model?: string;
  providerId?: string;
  pendingQuestion?: AgentPendingQuestion;
  allowedTools?: string[];
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
  icon?: string;
  endpoint: string;
  apiKey: string;
  apiType: CustomProviderApiType | string;
  models: string[];
};

export type AutomationSource = 'tickets' | 'repository' | 'sentry' | 'dokploy';

export type SentryLevel = 'warning' | 'error' | 'fatal';

export type TicketsTransitionTrigger = {
  kind: 'tickets.transition';
  fromStatusIds?: string[];
  toStatusId: string;
};

export type TicketsAssignedTrigger = {
  kind: 'tickets.assigned';
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
};

export type RepositoryPullRequestBuildSuccessTrigger = {
  kind: 'repository.pr_build_success';
};

export type RepositoryIssueAssignedTrigger = {
  kind: 'repository.issue_assigned';
};

export type SentryNewIssueTrigger = {
  kind: 'sentry.new_issue';
  minLevel?: SentryLevel;
};

export type DokployDeploymentFailedTrigger = {
  kind: 'dokploy.deployment_failed';
};

export type DokployDeploymentSucceededTrigger = {
  kind: 'dokploy.deployment_succeeded';
};

export type AutomationTrigger =
  | TicketsTransitionTrigger
  | TicketsAssignedTrigger
  | RepositoryPullRequestOpenedTrigger
  | RepositoryPullRequestCommentTrigger
  | RepositoryPullRequestBuildFailedTrigger
  | RepositoryPullRequestBuildSuccessTrigger
  | RepositoryIssueAssignedTrigger
  | SentryNewIssueTrigger
  | DokployDeploymentFailedTrigger
  | DokployDeploymentSucceededTrigger;

export type TriggerKind = AutomationTrigger['kind'];

export type ActionKind = 'spawn_agent' | 'tickets_transition' | 'notification' | 'send_email' | 'send_message';

export type SpawnAgentAction = {
  kind: 'spawn_agent';
  agentKind: string;
  model?: string;
  taskTemplate: string;
};

export type TicketsTransitionAction = {
  kind: 'tickets_transition';
  ticketsToStatusId: string;
  ticketsIssueKey?: string;
};

export type NotificationAction = {
  kind: 'notification';
  notifyTitle: string;
  notifyKind?: NotificationSeverity;
};

export type SendEmailAction = {
  kind: 'send_email';
  emailTo: string;
  emailSubject: string;
  emailBody?: string;
};

export type SendMessageAction = {
  kind: 'send_message';
  messageProvider: 'slack' | 'discord' | 'telegram';
  messageTitle?: string;
  messageBody?: string;
};

export type AutomationAction = SpawnAgentAction | TicketsTransitionAction | NotificationAction | SendEmailAction | SendMessageAction;

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
