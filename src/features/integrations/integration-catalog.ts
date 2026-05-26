import type { LucideIcon } from 'lucide-react';
import { Activity, Bug, Cloud, Container, GitBranch, KanbanSquare, Layout, MessageSquare, Package } from 'lucide-react';
import type { IntegrationConfig } from '@/types';
import { DetectDockerProject, DetectGitRemote, DetectNodeProject, DetectProviderToken, ListNodeScripts } from '@/wailsjs/go/main/App';

export type IntegrationFieldType = 'text' | 'password' | 'url' | 'number' | 'select' | 'autocomplete';

export interface IntegrationFieldOption {
  value: string;
  label: string;
}

export interface IntegrationField {
  key: string;
  label: string;
  type: IntegrationFieldType;
  required?: boolean;
  placeholder?: string;
  help?: string;
  options?: IntegrationFieldOption[];
  defaultValue?: string;
  loadOptions?: (values: Record<string, string>) => Promise<IntegrationFieldOption[]>;
}

export interface Integration {
  id: string;
  name: string;
  icon: LucideIcon;
  tint: string;
  fields: IntegrationField[];
  /**
   * Best-effort auto-detection invoked when a project is opened. Returning
   * null/undefined means "not applicable here" and the integration stays
   * unconfigured.
   */
  detect?: (projectPath: string) => Promise<IntegrationConfig | null | undefined>;
}

export const INTEGRATIONS: Integration[] = [
  {
    id: 'jira',
    name: 'Jira',
    icon: KanbanSquare,
    tint: 'text-blue-400 bg-blue-500/10',
    fields: [
      { key: 'baseUrl', label: 'Site URL', type: 'url', required: true, placeholder: 'https://your-org.atlassian.net' },
      { key: 'email', label: 'Account email', type: 'text', required: true, placeholder: 'you@example.com' },
      { key: 'token', label: 'API token', type: 'password', required: true },
      { key: 'projectKey', label: 'Project key', type: 'text', required: true, placeholder: 'AUTH', help: 'The short key shown in issue IDs (e.g. AUTH-123 → AUTH).' },
      { key: 'pollIntervalSec', label: 'Poll interval (seconds)', type: 'number', defaultValue: '60', help: 'How often the active sprint is refreshed. Shared across every automation on this Jira project.' },
    ],
  },
  {
    id: 'repository',
    name: 'Repository',
    icon: GitBranch,
    tint: 'text-purple-400 bg-purple-500/10',
    fields: [
      {
        key: 'provider',
        label: 'Provider',
        type: 'select',
        required: true,
        defaultValue: 'github',
        options: [
          { value: 'github', label: 'GitHub' },
          { value: 'gitlab', label: 'GitLab' },
          { value: 'bitbucket', label: 'Bitbucket' },
        ],
      },
      { key: 'baseUrl', label: 'Instance URL', type: 'url', placeholder: 'https://github.com', help: 'Leave blank for the provider default.' },
      { key: 'owner', label: 'Owner / org', type: 'text', placeholder: 'octocat' },
      { key: 'repo', label: 'Repository', type: 'text', placeholder: 'hello-world' },
      { key: 'token', label: 'Personal access token', type: 'password', help: 'Only needed for private repos, write actions, or to lift API rate limits.' },
      { key: 'pollIntervalSec', label: 'Poll interval (seconds)', type: 'number', defaultValue: '60', help: 'How often PRs, issues and recent workflow runs are refreshed. Shared across every automation on this repository.' },
    ],
    detect: detectRepository,
  },
  {
    id: 'nodejs',
    name: 'Node.js',
    icon: Package,
    tint: 'text-green-400 bg-green-500/10',
    fields: [
      {
        key: 'packageManager',
        label: 'Package manager',
        type: 'select',
        options: [
          { value: 'bun', label: 'Bun' },
          { value: 'pnpm', label: 'pnpm' },
          { value: 'yarn', label: 'Yarn' },
          { value: 'npm', label: 'npm' },
          { value: 'deno', label: 'Deno' },
        ],
      },
      {
        key: 'runEnv',
        label: 'Run environment',
        type: 'select',
        defaultValue: 'direct',
        options: [
          { value: 'direct', label: 'Direct (PATH)' },
          { value: 'nix', label: 'Nix develop' },
        ],
      },
      { key: 'manifestPath', label: 'package.json path', type: 'text', placeholder: '/path/to/package.json' },
      { key: 'startScript', label: 'Start script', type: 'autocomplete', placeholder: 'dev', loadOptions: loadScriptOptions },
      { key: 'testScript', label: 'Test script', type: 'autocomplete', placeholder: 'test', loadOptions: loadScriptOptions },
      { key: 'buildScript', label: 'Build script', type: 'autocomplete', placeholder: 'build', loadOptions: loadScriptOptions },
    ],
    detect: detectNode,
  },
  {
    id: 'docker',
    name: 'Docker',
    icon: Container,
    tint: 'text-sky-400 bg-sky-500/10',
    fields: [{ key: 'dockerfilePath', label: 'Dockerfile path', type: 'text', placeholder: '/path/to/Dockerfile' }],
    detect: detectDocker,
  },
  {
    id: 'datadog',
    name: 'Datadog',
    icon: Activity,
    tint: 'text-emerald-400 bg-emerald-500/10',
    fields: [
      { key: 'apiKey', label: 'API key', type: 'password', required: true },
      { key: 'appKey', label: 'Application key', type: 'password', required: true },
      { key: 'site', label: 'Site', type: 'text', placeholder: 'datadoghq.com' },
    ],
  },
  {
    id: 'sentry',
    name: 'Sentry',
    icon: Bug,
    tint: 'text-red-400 bg-red-500/10',
    fields: [
      { key: 'token', label: 'Auth token', type: 'password', required: true },
      { key: 'org', label: 'Organization slug', type: 'text', required: true },
      { key: 'project', label: 'Project slug', type: 'text' },
    ],
  },
  {
    id: 'slack',
    name: 'Slack',
    icon: MessageSquare,
    tint: 'text-purple-400 bg-purple-500/10',
    fields: [
      { key: 'token', label: 'Bot token', type: 'password', required: true, placeholder: 'xoxb-...' },
      { key: 'channel', label: 'Default channel', type: 'text', placeholder: '#engineering' },
    ],
  },
  {
    id: 'linear',
    name: 'Linear',
    icon: Layout,
    tint: 'text-blue-400 bg-blue-500/10',
    fields: [
      { key: 'apiKey', label: 'API key', type: 'password', required: true, placeholder: 'lin_api_...' },
      { key: 'team', label: 'Default team key', type: 'text', placeholder: 'ENG' },
    ],
  },
  {
    id: 'aws',
    name: 'AWS',
    icon: Cloud,
    tint: 'text-purple-400 bg-purple-500/10',
    fields: [
      { key: 'accessKeyId', label: 'Access key ID', type: 'text', required: true },
      { key: 'secretAccessKey', label: 'Secret access key', type: 'password', required: true },
      { key: 'region', label: 'Default region', type: 'text', placeholder: 'eu-west-1' },
    ],
  },
];

export function findIntegration(id: string): Integration | undefined {
  return INTEGRATIONS.find((i) => i.id === id);
}

async function detectRepository(projectPath: string): Promise<IntegrationConfig | null> {
  const remote = await DetectGitRemote(projectPath);
  if (!remote?.url) {
    return null;
  }
  const config: IntegrationConfig = {};
  if (remote.provider && remote.provider !== 'unknown') {
    config.provider = remote.provider;
  }
  if (remote.owner) {
    config.owner = remote.owner;
  }
  if (remote.repo) {
    config.repo = remote.repo;
  }
  if (remote.baseUrl && remote.host !== 'github.com') {
    config.baseUrl = remote.baseUrl;
  }
  if (config.provider === 'github' || config.provider === 'gitlab') {
    try {
      const tok = await DetectProviderToken(String(config.provider));
      if (tok?.token) {
        config.token = tok.token;
      }
    } catch {
      /* token discovery is best-effort */
    }
  }
  return config;
}

async function loadScriptOptions(values: Record<string, string>): Promise<IntegrationFieldOption[]> {
  if (!values.manifestPath) {
    return [];
  }
  try {
    const scripts = await ListNodeScripts(values.manifestPath);
    return (scripts ?? []).map((s) => ({ value: s.name, label: s.name }));
  } catch {
    return [];
  }
}

async function detectNode(projectPath: string): Promise<IntegrationConfig | null> {
  const project = await DetectNodeProject(projectPath);
  if (!project?.manifestPath) {
    return null;
  }
  return {
    manifestPath: project.manifestPath,
    packageManager: project.packageManager,
  };
}

async function detectDocker(projectPath: string): Promise<IntegrationConfig | null> {
  const project = await DetectDockerProject(projectPath);
  if (!project?.dockerfilePath) {
    return null;
  }
  return {
    dockerfilePath: project.dockerfilePath,
    composePath: project.composePath,
  };
}
