import type { LucideIcon } from 'lucide-react';
import { Code2, Gem, Sparkles, Terminal, Wind } from 'lucide-react';
import type { AgentKind } from '@/types';

export interface AgentModel {
  value: string;
  label: string;
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
    models: [
      { value: 'sonnet', label: 'Claude Sonnet 4.5' },
      { value: 'opus', label: 'Claude Opus 4.7' },
      { value: 'haiku', label: 'Claude Haiku 4.5' },
      { value: 'opusplan', label: 'Opus plan / Sonnet exec' },
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
    installCmd: 'pip install mistralai',
    docsUrl: 'https://docs.mistral.ai/',
    models: [
      { value: 'codestral-latest', label: 'Codestral' },
      { value: 'mistral-large-latest', label: 'Mistral Large' },
      { value: 'mistral-medium-latest', label: 'Mistral Medium' },
      { value: 'devstral-medium-latest', label: 'Devstral Medium' },
    ],
  },
];

export function findAgentKind(id: string): AgentKindConfig | undefined {
  return AGENT_KINDS.find((k) => k.id === id);
}
