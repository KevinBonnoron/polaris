import { SquareTerminal } from 'lucide-react';
import type * as React from 'react';
import { AnthropicIcon, CopilotIcon, CursorIcon, GeminiIcon, MistralIcon, OpenAIIcon } from '@/components/brand-icons';
import type { AgentKind } from '@/types';

export interface AgentModel {
  value: string;
  label: string;
  description?: string;
}

export interface AgentKindConfig {
  id: Exclude<AgentKind, 'other'>;
  label: string;
  // biome-ignore lint/suspicious/noExplicitAny: icon props vary between lucide and brand SVG components
  icon: React.ComponentType<any>;
  iconClass: string;
  models: AgentModel[];
  installCmd: string;
  docsUrl: string;
  spawnable?: boolean;
}

export const AGENT_KINDS: AgentKindConfig[] = [
  {
    id: 'claude-code',
    label: 'Claude Code',
    icon: AnthropicIcon,
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
    icon: CopilotIcon,
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
    icon: OpenAIIcon,
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
    icon: GeminiIcon,
    iconClass: 'bg-amber-500 text-white',
    installCmd: 'npm i -g @google/gemini-cli',
    docsUrl: 'https://github.com/google-gemini/gemini-cli',
    models: [
      { value: 'gemini-2.5-flash', label: 'Gemini 2.5 Flash' },
      { value: 'gemini-2.5-pro', label: 'Gemini 2.5 Pro' },
      { value: 'gemini-2.5-flash-lite', label: 'Gemini 2.5 Flash Lite' },
      { value: 'gemini-3-pro-preview', label: 'Gemini 3 Pro Preview' },
      { value: 'gemini-3-flash-preview', label: 'Gemini 3 Flash Preview' },
    ],
  },
  {
    id: 'mistral',
    label: 'Mistral',
    icon: MistralIcon,
    iconClass: 'bg-orange-500 text-white',
    installCmd: 'uv tool install mistral-vibe',
    docsUrl: 'https://docs.mistral.ai/vibe/code/cli/install-setup',
    models: [
      { value: 'mistral-medium-3.5', label: 'Mistral Medium 3.5' },
      { value: 'devstral-small', label: 'Devstral Small' },
    ],
  },
  {
    id: 'cursor',
    label: 'Cursor',
    icon: CursorIcon,
    iconClass: 'bg-zinc-800 text-white',
    installCmd: 'curl https://cursor.com/install -fsSL | bash',
    docsUrl: 'https://docs.cursor.com/',
    models: [
      { value: 'auto', label: 'Auto' },
      { value: 'composer-2.5', label: 'Composer 2.5' },
      { value: 'opus-4.8', label: 'Opus 4.8', description: 'Thinking · 300K' },
      { value: 'sonnet-4.6', label: 'Sonnet 4.6', description: 'Thinking · 200K' },
      { value: 'opus-4.7', label: 'Opus 4.7', description: 'Thinking · 300K' },
      { value: 'opus-4.6', label: 'Opus 4.6', description: 'Thinking · 200K' },
      { value: 'opus-4.5', label: 'Opus 4.5', description: 'Thinking' },
      { value: 'sonnet-4.5', label: 'Sonnet 4.5', description: 'Thinking' },
      { value: 'haiku-4.5', label: 'Haiku 4.5', description: 'Thinking' },
      { value: 'gpt-5.5', label: 'GPT-5.5' },
      { value: 'gpt-5.4', label: 'GPT-5.4' },
      { value: 'gpt-5.2', label: 'GPT-5.2' },
      { value: 'gpt-5.1', label: 'GPT-5.1' },
      { value: 'gpt-5-mini', label: 'GPT-5 Mini' },
      { value: 'codex-5.3', label: 'Codex 5.3' },
      { value: 'codex-5.2', label: 'Codex 5.2' },
      { value: 'codex-5.1-max', label: 'Codex 5.1 Max' },
      { value: 'codex-5.1-mini', label: 'Codex 5.1 Mini' },
      { value: 'gemini-3.1-pro', label: 'Gemini 3.1 Pro' },
      { value: 'gemini-3.5-flash', label: 'Gemini 3.5 Flash' },
      { value: 'gemini-3-flash', label: 'Gemini 3 Flash' },
      { value: 'grok-build-0.1', label: 'Grok Build 0.1' },
      { value: 'grok-4.3', label: 'Grok 4.3' },
      { value: 'kimi-k2.5', label: 'Kimi K2.5' },
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
