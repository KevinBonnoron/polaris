import type { LucideIcon } from 'lucide-react';
import { Code2, Gem, Sparkles, SquareTerminal, Terminal, Wind } from 'lucide-react';
import type { AgentKind } from '@/types';

export interface AgentModel {
  value: string;
  label: string;
  description?: string;
}

export interface AgentKindConfig {
  id: Exclude<AgentKind, 'other'>;
  label: string;
  icon: LucideIcon;
  iconClass: string;
  models: AgentModel[];
  installCmd: string;
  docsUrl: string;
}

export const AGENT_KINDS: AgentKindConfig[] = [
  {
    id: 'claude-code',
    label: 'Claude Code',
    icon: Sparkles,
    iconClass: 'bg-blue-500 text-white',
    installCmd: 'npm i -g @anthropic-ai/claude-code',
    docsUrl: 'https://docs.anthropic.com/en/docs/claude-code/quickstart',
    // Fallback list when /v1/models can't be reached; agent-clis replaces these
    // with the latest model per family resolved live from the Claude API.
    models: [
      { value: 'opus', label: 'Opus' },
      { value: 'sonnet', label: 'Sonnet' },
      { value: 'haiku', label: 'Haiku' },
    ],
  },
  {
    id: 'copilot',
    label: 'Copilot',
    icon: Code2,
    iconClass: 'bg-purple-500 text-white',
    installCmd: 'gh extension install github/gh-copilot',
    docsUrl: 'https://docs.github.com/en/copilot/how-tos/set-up/install-copilot-cli',
    models: [
      { value: 'claude-sonnet-4.5', label: 'Claude Sonnet 4.5' },
      { value: 'claude-opus-4', label: 'Claude Opus 4' },
      { value: 'claude-haiku-4.5', label: 'Claude Haiku 4.5' },
      { value: 'gpt-5', label: 'GPT-5' },
      { value: 'gpt-5-mini', label: 'GPT-5 mini' },
      { value: 'gpt-4.1', label: 'GPT-4.1' },
      { value: 'gpt-4o', label: 'GPT-4o' },
      { value: 'o3', label: 'o3' },
      { value: 'o4-mini', label: 'o4-mini' },
      { value: 'gemini-2.5-pro', label: 'Gemini 2.5 Pro' },
    ],
  },
  {
    id: 'codex',
    label: 'Codex',
    icon: Terminal,
    iconClass: 'bg-emerald-500 text-white',
    installCmd: 'npm i -g @openai/codex',
    docsUrl: 'https://github.com/openai/codex',
    models: [
      { value: 'gpt-5-codex', label: 'GPT-5 Codex' },
      { value: 'gpt-5', label: 'GPT-5' },
      { value: 'gpt-5-mini', label: 'GPT-5 mini' },
      { value: 'o3', label: 'o3' },
      { value: 'o4-mini', label: 'o4-mini' },
    ],
  },
  {
    id: 'gemini',
    label: 'Gemini',
    icon: Gem,
    iconClass: 'bg-amber-500 text-white',
    installCmd: 'npm i -g @google/gemini-cli',
    docsUrl: 'https://github.com/google-gemini/gemini-cli',
    models: [
      { value: 'gemini-2.5-pro', label: 'Gemini 2.5 Pro' },
      { value: 'gemini-2.5-flash', label: 'Gemini 2.5 Flash' },
      { value: 'gemini-2.5-flash-lite', label: 'Gemini 2.5 Flash Lite' },
    ],
  },
  {
    id: 'mistral',
    label: 'Mistral',
    icon: Wind,
    iconClass: 'bg-orange-500 text-white',
    installCmd: 'uv tool install mistral-vibe',
    docsUrl: 'https://docs.mistral.ai/vibe/code/cli/install-setup',
    models: [
      { value: 'mistral-medium-3.5', label: 'Mistral Medium 3.5' },
      { value: 'devstral-small', label: 'Devstral Small' },
    ],
  },
];

export function findAgentKind(id: string): AgentKindConfig | undefined {
  return AGENT_KINDS.find((k) => k.id === id);
}

// opencode is the harness for custom providers, not a standalone spawnable kind,
// so it stays out of AGENT_KINDS (the picker). This descriptor only feeds the
// settings "installed CLIs" list.
export const OPENCODE_DESCRIPTOR: AgentKindConfig = {
  id: 'opencode',
  label: 'opencode',
  icon: SquareTerminal,
  iconClass: 'bg-foreground text-background',
  installCmd: 'curl -fsSL https://opencode.ai/install | bash',
  docsUrl: 'https://opencode.ai/docs/',
  models: [],
};
