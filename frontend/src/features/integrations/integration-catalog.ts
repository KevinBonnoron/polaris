import type { LucideIcon } from 'lucide-react';
import { Boxes, Bug, Container, Hash, ListChecks, Mail, Package, Rocket } from 'lucide-react';
import type { ComponentType } from 'react';
import { BitbucketIcon, DiscordIcon, GitHubIcon, GitLabIcon, JiraIcon, LinearIcon, SlackIcon, TelegramIcon } from '@/components/brand-icons';
import type { IntegrationConfig } from '@/types';
import { DetectAllCSharpProjects, DetectAllDockerProjects, DetectAllNodeProjects, DetectAllPythonProjects, DetectAllTaskfileProjects, DetectGitRemote, DetectProviderToken, ListCSharpScripts, ListDokployProjectNames, ListNodeScripts, ListPythonScripts, ListTaskfileTasks } from '@/wailsjs/go/main/App';
import { dokploy } from '@/wailsjs/go/models';

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
  helpUrl?: string;
  loadOptions?: (values: Record<string, string>) => Promise<IntegrationFieldOption[]>;
  /** For 'autocomplete' fields: allow picking several values, stored as a comma-separated string. */
  multiple?: boolean;
}

export interface Integration {
  id: string;
  name: string;
  icon: LucideIcon | ComponentType<{ className?: string }>;
  tint: string;
  fields: IntegrationField[];
  /** When set, config is stored under this key instead of `id`. */
  storageKey?: string;
  /** Values always merged into saved config (not shown as form fields). */
  fixedValues?: Record<string, string>;
  multi?: boolean;
  instanceLabel?: (config: IntegrationConfig, projectPath: string) => string;
  detect?: (projectPath: string) => Promise<IntegrationConfig | IntegrationConfig[] | null | undefined>;
}

const TICKETS_FIELDS_JIRA: IntegrationField[] = [
  { key: 'baseUrl', label: 'Site URL', type: 'url', required: true, placeholder: 'https://your-org.atlassian.net' },
  { key: 'email', label: 'Account email', type: 'text', required: true, placeholder: 'you@example.com' },
  { key: 'token', label: 'API token', type: 'password', required: true, help: 'Create an API token', helpUrl: 'https://id.atlassian.com/manage-profile/security/api-tokens' },
  { key: 'projectKey', label: 'Project key', type: 'text', required: true, placeholder: 'AUTH', help: 'The short key shown in issue IDs (e.g. AUTH-123 → AUTH).' },
  { key: 'pollIntervalSec', label: 'Poll interval (seconds)', type: 'number', defaultValue: '60', help: 'How often the active sprint is refreshed. Shared across every automation on this ticket project.' },
];

const TICKETS_FIELDS_LINEAR: IntegrationField[] = [
  { key: 'token', label: 'API key', type: 'password', required: true, help: 'Create an API key', helpUrl: 'https://linear.app/settings/api' },
  { key: 'projectKey', label: 'Team key', type: 'text', required: true, placeholder: 'ENG', help: 'The short identifier for your Linear team (e.g. ENG-123 → ENG).' },
  { key: 'pollIntervalSec', label: 'Poll interval (seconds)', type: 'number', defaultValue: '60', help: 'How often the active sprint is refreshed. Shared across every automation on this ticket project.' },
];

const REPOSITORY_FIELDS: IntegrationField[] = [
  { key: 'baseUrl', label: 'Instance URL', type: 'url', placeholder: 'https://github.com', help: 'Leave blank for the provider default.' },
  { key: 'owner', label: 'Owner / org', type: 'text', required: true, placeholder: 'octocat' },
  { key: 'repo', label: 'Repository', type: 'text', required: true, placeholder: 'hello-world' },
  { key: 'token', label: 'Personal access token', type: 'password', help: 'Only needed for private repos, write actions, or to lift API rate limits.' },
  { key: 'pollIntervalSec', label: 'Poll interval (seconds)', type: 'number', defaultValue: '60', help: 'How often PRs, issues and recent workflow runs are refreshed. Shared across every automation on this repository.' },
];

export const INTEGRATIONS: Integration[] = [
  {
    id: 'jira',
    name: 'Jira',
    icon: JiraIcon,
    tint: 'text-blue-400 bg-blue-500/10',
    storageKey: 'tickets',
    fixedValues: { provider: 'jira' },
    fields: TICKETS_FIELDS_JIRA,
  },
  {
    id: 'linear',
    name: 'Linear',
    icon: LinearIcon,
    tint: 'text-violet-400 bg-violet-500/10',
    storageKey: 'tickets',
    fixedValues: { provider: 'linear' },
    fields: TICKETS_FIELDS_LINEAR,
  },
  {
    id: 'github',
    name: 'GitHub',
    icon: GitHubIcon,
    tint: 'text-neutral-400 bg-neutral-500/10',
    storageKey: 'repository',
    fixedValues: { provider: 'github' },
    fields: REPOSITORY_FIELDS,
    detect: detectRepository,
  },
  {
    id: 'gitlab',
    name: 'GitLab',
    icon: GitLabIcon,
    tint: 'text-orange-400 bg-orange-500/10',
    storageKey: 'repository',
    fixedValues: { provider: 'gitlab' },
    fields: REPOSITORY_FIELDS,
  },
  {
    id: 'bitbucket',
    name: 'Bitbucket',
    icon: BitbucketIcon,
    tint: 'text-blue-500 bg-blue-500/10',
    storageKey: 'repository',
    fixedValues: { provider: 'bitbucket' },
    fields: REPOSITORY_FIELDS,
  },
  {
    id: 'nodejs',
    name: 'Node.js',
    icon: Package,
    tint: 'text-green-400 bg-green-500/10',
    multi: true,
    instanceLabel: (config, projectPath) => relativePath(String(config.manifestPath ?? ''), projectPath),
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
          { value: 'devcontainer', label: 'Devcontainer' },
        ],
      },
      { key: 'manifestPath', label: 'package.json path', type: 'text', placeholder: '/path/to/package.json' },
      { key: 'startScript', label: 'Start script', type: 'autocomplete', placeholder: 'dev', loadOptions: loadScriptOptions },
      { key: 'testScript', label: 'Test script', type: 'autocomplete', placeholder: 'test', loadOptions: loadScriptOptions },
      { key: 'buildScript', label: 'Build script', type: 'autocomplete', placeholder: 'build', loadOptions: loadScriptOptions },
      { key: 'extraScripts', label: 'Extra scripts', type: 'autocomplete', multiple: true, placeholder: 'preview, storybook', loadOptions: loadScriptOptions },
    ],
    detect: detectNode,
  },
  {
    id: 'python',
    name: 'Python',
    icon: Boxes,
    tint: 'text-yellow-400 bg-yellow-500/10',
    multi: true,
    instanceLabel: (config, projectPath) => relativePath(String(config.manifestPath ?? ''), projectPath),
    fields: [
      {
        key: 'packageManager',
        label: 'Package manager',
        type: 'select',
        options: [
          { value: 'uv', label: 'uv' },
          { value: 'poetry', label: 'Poetry' },
          { value: 'pdm', label: 'PDM' },
          { value: 'pipenv', label: 'Pipenv' },
          { value: 'pip', label: 'pip' },
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
          { value: 'devcontainer', label: 'Devcontainer' },
        ],
      },
      { key: 'manifestPath', label: 'Manifest path', type: 'text', placeholder: '/path/to/pyproject.toml' },
      { key: 'startScript', label: 'Start command', type: 'autocomplete', placeholder: 'python -m app', loadOptions: loadPythonScriptOptions },
      { key: 'testScript', label: 'Test command', type: 'autocomplete', placeholder: 'pytest', loadOptions: loadPythonScriptOptions },
      { key: 'buildScript', label: 'Build command', type: 'autocomplete', placeholder: 'build', loadOptions: loadPythonScriptOptions },
    ],
    detect: detectPython,
  },
  {
    id: 'csharp',
    name: 'C#',
    icon: Hash,
    tint: 'text-purple-400 bg-purple-500/10',
    multi: true,
    instanceLabel: (config, projectPath) => relativePath(String(config.manifestPath ?? ''), projectPath),
    fields: [
      {
        key: 'packageManager',
        label: 'Tooling',
        type: 'select',
        defaultValue: 'dotnet',
        options: [{ value: 'dotnet', label: '.NET CLI' }],
      },
      {
        key: 'runEnv',
        label: 'Run environment',
        type: 'select',
        defaultValue: 'direct',
        options: [
          { value: 'direct', label: 'Direct (PATH)' },
          { value: 'nix', label: 'Nix develop' },
          { value: 'devcontainer', label: 'Devcontainer' },
        ],
      },
      { key: 'manifestPath', label: 'Project path', type: 'text', placeholder: '/path/to/App.csproj' },
      { key: 'startScript', label: 'Start command', type: 'autocomplete', placeholder: 'run', loadOptions: loadCSharpScriptOptions },
      { key: 'testScript', label: 'Test command', type: 'autocomplete', placeholder: 'test', loadOptions: loadCSharpScriptOptions },
      { key: 'buildScript', label: 'Build command', type: 'autocomplete', placeholder: 'build', loadOptions: loadCSharpScriptOptions },
    ],
    detect: detectCSharp,
  },
  {
    id: 'taskfile',
    name: 'Taskfile',
    icon: ListChecks,
    tint: 'text-cyan-400 bg-cyan-500/10',
    multi: true,
    instanceLabel: (config, projectPath) => relativePath(String(config.manifestPath ?? ''), projectPath),
    fields: [
      {
        key: 'runEnv',
        label: 'Run environment',
        type: 'select',
        defaultValue: 'direct',
        options: [
          { value: 'direct', label: 'Direct (PATH)' },
          { value: 'nix', label: 'Nix develop' },
          { value: 'devcontainer', label: 'Devcontainer' },
        ],
      },
      { key: 'manifestPath', label: 'Taskfile path', type: 'text', placeholder: '/path/to/Taskfile.yml' },
      { key: 'startTask', label: 'Start task', type: 'autocomplete', placeholder: 'dev', loadOptions: loadTaskfileTaskOptions },
      { key: 'testTask', label: 'Test task', type: 'autocomplete', placeholder: 'test', loadOptions: loadTaskfileTaskOptions },
      { key: 'buildTask', label: 'Build task', type: 'autocomplete', placeholder: 'build', loadOptions: loadTaskfileTaskOptions },
    ],
    detect: detectTaskfile,
  },
  {
    id: 'docker',
    name: 'Docker',
    icon: Container,
    tint: 'text-sky-400 bg-sky-500/10',
    multi: true,
    instanceLabel: (config, projectPath) => relativePath(String(config.dockerfilePath ?? ''), projectPath),
    fields: [{ key: 'dockerfilePath', label: 'Dockerfile path', type: 'text', placeholder: '/path/to/Dockerfile' }],
    detect: detectDocker,
  },
  {
    id: 'sentry',
    name: 'Sentry',
    icon: Bug,
    tint: 'text-red-400 bg-red-500/10',
    fields: [
      { key: 'token', label: 'Auth token', type: 'password', required: true, help: 'Create a token on sentry.io', helpUrl: 'https://sentry.io/settings/account/api/auth-tokens/new-token/' },
      { key: 'org', label: 'Organization slug', type: 'text', required: true },
      { key: 'projects', label: 'Project slugs', type: 'text', required: true, placeholder: 'backend, frontend', help: 'Comma-separated Sentry project slugs' },
      { key: 'url', label: 'Instance URL', type: 'url', placeholder: 'https://sentry.io', help: 'Override for self-hosted Sentry' },
      { key: 'pollIntervalSec', label: 'Poll interval (seconds)', type: 'number', defaultValue: '60', help: 'How often the watched projects are polled for new issues. Shared across every automation on this Sentry org.' },
    ],
  },
  {
    id: 'dokploy',
    name: 'Dokploy',
    icon: Rocket,
    tint: 'text-teal-400 bg-teal-500/10',
    fields: [
      { key: 'baseUrl', label: 'Instance URL', type: 'url', required: true, placeholder: 'https://dokploy.example.com', help: 'The root URL of your Dokploy instance.' },
      { key: 'apiKey', label: 'API key', type: 'password', required: true, help: 'Generate one under Settings → Profile → API/CLI in Dokploy.' },
      { key: 'projects', label: 'Projects', type: 'autocomplete', multiple: true, placeholder: 'All projects', help: 'Pick the projects to watch. Leave empty to watch every project. Fill the URL and API key first to load the list.', loadOptions: loadDokployProjectOptions },
      { key: 'pollIntervalSec', label: 'Poll interval (seconds)', type: 'number', defaultValue: '60', help: 'How often deployments are refreshed. Shared across every automation on this instance.' },
    ],
  },
  {
    id: 'resend',
    name: 'Resend',
    icon: Mail,
    tint: 'text-blue-400 bg-blue-500/10',
    fields: [
      { key: 'apiKey', label: 'API key', type: 'password', required: true, placeholder: 're_...', helpUrl: 'https://resend.com/api-keys' },
      { key: 'fromEmail', label: 'From email', type: 'text', required: true, placeholder: 'noreply@example.com', help: 'Must be from a verified domain in your Resend account.' },
    ],
  },
  {
    id: 'slack',
    name: 'Slack',
    icon: SlackIcon,
    tint: 'text-[#4A154B] bg-[#4A154B]/10',
    fields: [
      {
        key: 'webhook',
        label: 'Webhook URL',
        type: 'url',
        required: true,
        placeholder: 'https://hooks.slack.com/services/...',
        help: 'Create an Incoming Webhook in your Slack app settings.',
        helpUrl: 'https://api.slack.com/apps',
      },
    ],
  },
  {
    id: 'discord',
    name: 'Discord',
    icon: DiscordIcon,
    tint: 'text-[#5865F2] bg-[#5865F2]/10',
    fields: [
      {
        key: 'webhook',
        label: 'Webhook URL',
        type: 'url',
        required: true,
        placeholder: 'https://discord.com/api/webhooks/...',
        help: 'Create a webhook in Discord channel settings',
        helpUrl: 'https://support.discord.com/hc/articles/228383668',
      },
    ],
  },
  {
    id: 'telegram',
    name: 'Telegram',
    icon: TelegramIcon,
    tint: 'text-[#26A5E4] bg-[#26A5E4]/10',
    fields: [
      {
        key: 'token',
        label: 'Bot token',
        type: 'password',
        required: true,
        placeholder: '123456:ABC-DEF...',
        help: 'Create a bot with @BotFather on Telegram to get your token.',
      },
      {
        key: 'channel',
        label: 'Chat ID',
        type: 'text',
        required: true,
        placeholder: '-1001234567890',
        help: 'Negative for groups/channels (-100...), positive for DMs. Forward any message from the chat to @userinfobot to get the ID.',
      },
    ],
  },
];

export function findIntegration(id: string): Integration | undefined {
  return INTEGRATIONS.find((i) => i.id === id);
}

/** Find the active integration entry for a given storageKey, using the saved provider value. */
export function findIntegrationForStorageKey(storageKey: string, config?: Record<string, unknown> | null): Integration | undefined {
  const provider = typeof config?.provider === 'string' ? config.provider : undefined;
  if (provider) {
    const byProvider = INTEGRATIONS.find((i) => i.id === provider && (i.storageKey ?? i.id) === storageKey);
    if (byProvider) {
      return byProvider;
    }
  }
  return INTEGRATIONS.find((i) => (i.storageKey ?? i.id) === storageKey);
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
      const detected = await DetectProviderToken(String(config.provider));
      if (detected?.token) {
        config.token = detected.token;
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

async function loadDokployProjectOptions(values: Record<string, string>): Promise<IntegrationFieldOption[]> {
  if (!values.baseUrl || !values.apiKey) {
    return [];
  }
  const names = await ListDokployProjectNames(dokploy.Config.createFrom({ baseUrl: values.baseUrl, apiKey: values.apiKey }));
  return (names ?? []).map((name) => ({ value: name, label: name }));
}

function pickScript(names: string[], candidates: string[]): string | undefined {
  for (const name of candidates) {
    if (names.includes(name)) {
      return name;
    }
  }
  return undefined;
}

async function detectNode(projectPath: string): Promise<IntegrationConfig | IntegrationConfig[] | null> {
  const projects = await DetectAllNodeProjects(projectPath);
  if (!projects?.length) {
    return null;
  }

  const configs = projects.flatMap((p) => {
    if (!p) {
      return [];
    }
    const c: IntegrationConfig = { manifestPath: p.manifestPath, packageManager: p.packageManager };
    if (p.runEnv) {
      c.runEnv = p.runEnv;
    }
    const names = (p.scripts ?? []).map((s) => s.name);
    const start = pickScript(names, ['dev', 'start', 'serve', 'develop']);
    const test = pickScript(names, ['test']);
    const build = pickScript(names, ['build']);
    if (start) {
      c.startScript = start;
    }
    if (test) {
      c.testScript = test;
    }
    if (build) {
      c.buildScript = build;
    }
    return [c];
  });
  return configs.length === 1 ? configs[0] : configs;
}

async function loadPythonScriptOptions(values: Record<string, string>): Promise<IntegrationFieldOption[]> {
  if (!values.manifestPath) {
    return [];
  }

  try {
    const scripts = await ListPythonScripts(values.manifestPath);
    return (scripts ?? []).map((s) => ({ value: s.name, label: s.name }));
  } catch {
    return [];
  }
}

async function detectPython(projectPath: string): Promise<IntegrationConfig | IntegrationConfig[] | null> {
  const projects = await DetectAllPythonProjects(projectPath);
  if (!projects?.length) {
    return null;
  }

  const configs = projects.flatMap((p) => {
    if (!p) {
      return [];
    }
    const c: IntegrationConfig = { manifestPath: p.manifestPath, packageManager: p.packageManager };
    if (p.runEnv) {
      c.runEnv = p.runEnv;
    }
    const names = (p.scripts ?? []).map((s) => s.name);
    const start = pickScript(names, ['start', 'dev', 'serve', 'run']);
    const test = pickScript(names, ['test', 'pytest']);
    const build = pickScript(names, ['build', 'compile']);
    if (start) {
      c.startScript = start;
    }
    if (test) {
      c.testScript = test;
    }
    if (build) {
      c.buildScript = build;
    }
    return [c];
  });
  return configs.length === 1 ? configs[0] : configs;
}

async function loadCSharpScriptOptions(values: Record<string, string>): Promise<IntegrationFieldOption[]> {
  if (!values.manifestPath) {
    return [];
  }

  try {
    const scripts = await ListCSharpScripts(values.manifestPath);
    return (scripts ?? []).map((s) => ({ value: s.name, label: s.name }));
  } catch {
    return [];
  }
}

async function detectCSharp(projectPath: string): Promise<IntegrationConfig | IntegrationConfig[] | null> {
  const projects = await DetectAllCSharpProjects(projectPath);
  if (!projects?.length) {
    return null;
  }

  const configs = projects.flatMap((p) => {
    if (!p) {
      return [];
    }
    const c: IntegrationConfig = { manifestPath: p.manifestPath, packageManager: p.packageManager || 'dotnet', startScript: 'run', testScript: 'test', buildScript: 'build' };
    if (p.runEnv) {
      c.runEnv = p.runEnv;
    }
    return [c];
  });
  return configs.length === 1 ? configs[0] : configs;
}

async function loadTaskfileTaskOptions(values: Record<string, string>): Promise<IntegrationFieldOption[]> {
  if (!values.manifestPath) {
    return [];
  }

  try {
    const tasks = await ListTaskfileTasks(values.manifestPath);
    return (tasks ?? []).map((s) => ({ value: s.name, label: s.name }));
  } catch {
    return [];
  }
}

async function detectTaskfile(projectPath: string): Promise<IntegrationConfig | IntegrationConfig[] | null> {
  const projects = await DetectAllTaskfileProjects(projectPath);
  if (!projects?.length) {
    return null;
  }

  const configs = projects.flatMap((p) => {
    if (!p) {
      return [];
    }
    const c: IntegrationConfig = { manifestPath: p.manifestPath, packageManager: 'task' };
    if (p.runEnv) {
      c.runEnv = p.runEnv;
    }
    const names = (p.scripts ?? []).map((s) => s.name);
    const start = pickScript(names, ['dev', 'start', 'serve', 'run', 'develop']);
    const test = pickScript(names, ['test']);
    const build = pickScript(names, ['build']);
    if (start) {
      c.startTask = start;
    }
    if (test) {
      c.testTask = test;
    }
    if (build) {
      c.buildTask = build;
    }
    return [c];
  });
  return configs.length === 1 ? configs[0] : configs;
}

async function detectDocker(projectPath: string): Promise<IntegrationConfig | IntegrationConfig[] | null> {
  const projects = await DetectAllDockerProjects(projectPath);
  if (!projects?.length) {
    return null;
  }

  const configs = projects.flatMap((p) => (p ? [{ dockerfilePath: p.dockerfilePath, composePath: p.composePath }] : []));
  return configs.length === 1 ? configs[0] : configs;
}

function relativePath(fullPath: string, projectPath: string): string {
  if (!fullPath || !projectPath) {
    return fullPath;
  }

  const prefix = projectPath.endsWith('/') ? projectPath : projectPath + '/';
  return fullPath.startsWith(prefix) ? fullPath.slice(prefix.length) : fullPath;
}
