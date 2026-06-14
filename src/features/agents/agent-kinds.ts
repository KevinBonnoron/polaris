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
  },
  {
    id: 'copilot',
    label: 'Copilot',
    icon: CopilotIcon,
    iconClass: 'bg-purple-500 text-white',
    installCmd: 'gh extension install github/gh-copilot',
    docsUrl: 'https://docs.github.com/en/copilot/how-tos/set-up/install-copilot-cli',
  },
  {
    id: 'codex',
    label: 'Codex',
    icon: OpenAIIcon,
    iconClass: 'bg-emerald-500 text-white',
    installCmd: 'npm i -g @openai/codex',
    docsUrl: 'https://github.com/openai/codex',
  },
  {
    id: 'gemini',
    label: 'Gemini',
    icon: GeminiIcon,
    iconClass: 'bg-amber-500 text-white',
    installCmd: 'npm i -g @google/gemini-cli',
    docsUrl: 'https://github.com/google-gemini/gemini-cli',
  },
  {
    id: 'mistral',
    label: 'Mistral',
    icon: MistralIcon,
    iconClass: 'bg-orange-500 text-white',
    installCmd: 'uv tool install mistral-vibe',
    docsUrl: 'https://docs.mistral.ai/vibe/code/cli/install-setup',
  },
  {
    id: 'cursor',
    label: 'Cursor',
    icon: CursorIcon,
    iconClass: 'bg-zinc-800 text-white',
    installCmd: 'curl https://cursor.com/install -fsSL | bash',
    docsUrl: 'https://docs.cursor.com/',
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
};
