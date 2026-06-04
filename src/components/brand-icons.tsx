import { Bot, Brain, Code2, Cpu, Globe, Server, Sparkles, SquareTerminal, Zap } from 'lucide-react';
import type * as React from 'react';
import { siAnthropic, siCursor, siDeepseek, siDiscord, siGithubcopilot, siGooglegemini, siHuggingface, siMeta, siMistralai, siOllama, siPerplexity, siTelegram } from 'simple-icons';

function BrandIcon({ path, ...props }: { path: string } & React.SVGProps<SVGSVGElement>) {
  return (
    <svg role="img" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg" fill="currentColor" aria-hidden {...props}>
      <title>Brand Icon</title>
      <path d={path} />
    </svg>
  );
}

export function AnthropicIcon(props: React.SVGProps<SVGSVGElement>) {
  return <BrandIcon path={siAnthropic.path} {...props} />;
}

export function CopilotIcon(props: React.SVGProps<SVGSVGElement>) {
  return <BrandIcon path={siGithubcopilot.path} {...props} />;
}

export function GeminiIcon(props: React.SVGProps<SVGSVGElement>) {
  return <BrandIcon path={siGooglegemini.path} {...props} />;
}

export function MistralIcon(props: React.SVGProps<SVGSVGElement>) {
  return <BrandIcon path={siMistralai.path} {...props} />;
}

// OpenAI dropped from simple-icons v13+; path sourced from their public brand assets
const OPENAI_PATH =
  'M22.282 9.821a5.985 5.985 0 0 0-.516-4.91 6.046 6.046 0 0 0-6.51-2.9A6.065 6.065 0 0 0 4.981 4.18a5.985 5.985 0 0 0-3.998 2.9 6.046 6.046 0 0 0 .743 7.097 5.98 5.98 0 0 0 .51 4.911 6.051 6.051 0 0 0 6.515 2.9A5.985 5.985 0 0 0 13.26 24a6.056 6.056 0 0 0 5.772-4.206 5.99 5.99 0 0 0 3.997-2.9 6.056 6.056 0 0 0-.747-7.073zM13.26 22.43a4.476 4.476 0 0 1-2.876-1.04l.141-.081 4.779-2.758a.795.795 0 0 0 .392-.681v-6.737l2.02 1.168a.071.071 0 0 1 .038.052v5.583a4.504 4.504 0 0 1-4.494 4.494zM3.6 18.304a4.47 4.47 0 0 1-.535-3.014l.142.085 4.783 2.759a.771.771 0 0 0 .78 0l5.843-3.369v2.332a.08.08 0 0 1-.033.062L9.74 19.95a4.5 4.5 0 0 1-6.14-1.646zM2.34 7.896a4.485 4.485 0 0 1 2.366-1.973V11.6a.766.766 0 0 0 .388.676l5.815 3.355-2.02 1.168a.076.076 0 0 1-.071 0l-4.83-2.786A4.504 4.504 0 0 1 2.34 7.872zm16.597 3.855-5.833-3.387L15.119 7.2a.076.076 0 0 1 .071 0l4.83 2.791a4.494 4.494 0 0 1-.676 8.105v-5.678a.79.79 0 0 0-.407-.667zm2.01-3.023-.141-.085-4.774-2.782a.776.776 0 0 0-.785 0L9.409 9.23V6.897a.066.066 0 0 1 .028-.061l4.83-2.787a4.5 4.5 0 0 1 6.68 4.66zm-12.64 4.135-2.02-1.164a.08.08 0 0 1-.038-.057V6.075a4.5 4.5 0 0 1 7.375-3.453l-.142.08L8.704 5.46a.795.795 0 0 0-.393.681zm1.097-2.365 2.602-1.5 2.607 1.5v2.999l-2.597 1.5-2.607-1.5z';

export function CursorIcon(props: React.SVGProps<SVGSVGElement>) {
  return <BrandIcon path={siCursor.path} {...props} />;
}

export function OpenAIIcon(props: React.SVGProps<SVGSVGElement>) {
  return <BrandIcon path={OPENAI_PATH} {...props} />;
}

export function DiscordIcon(props: React.SVGProps<SVGSVGElement>) {
  return <BrandIcon path={siDiscord.path} {...props} />;
}

export function TelegramIcon(props: React.SVGProps<SVGSVGElement>) {
  return <BrandIcon path={siTelegram.path} {...props} />;
}

function OllamaIcon(props: React.SVGProps<SVGSVGElement>) {
  return <BrandIcon path={siOllama.path} {...props} />;
}

function DeepSeekIcon(props: React.SVGProps<SVGSVGElement>) {
  return <BrandIcon path={siDeepseek.path} {...props} />;
}

function HuggingFaceIcon(props: React.SVGProps<SVGSVGElement>) {
  return <BrandIcon path={siHuggingface.path} {...props} />;
}

function MetaIcon(props: React.SVGProps<SVGSVGElement>) {
  return <BrandIcon path={siMeta.path} {...props} />;
}

function PerplexityIcon(props: React.SVGProps<SVGSVGElement>) {
  return <BrandIcon path={siPerplexity.path} {...props} />;
}

// biome-ignore lint/suspicious/noExplicitAny: icon props vary
type IconComponent = React.ComponentType<any>;

export const PROVIDER_ICON_REGISTRY: { key: string; label: string; icon: IconComponent }[] = [
  { key: 'anthropic', label: 'Anthropic', icon: AnthropicIcon },
  { key: 'openai', label: 'OpenAI', icon: OpenAIIcon },
  { key: 'gemini', label: 'Gemini', icon: GeminiIcon },
  { key: 'mistral', label: 'Mistral', icon: MistralIcon },
  { key: 'ollama', label: 'Ollama', icon: OllamaIcon },
  { key: 'deepseek', label: 'DeepSeek', icon: DeepSeekIcon },
  { key: 'meta', label: 'Meta', icon: MetaIcon },
  { key: 'perplexity', label: 'Perplexity', icon: PerplexityIcon },
  { key: 'huggingface', label: 'HuggingFace', icon: HuggingFaceIcon },
  { key: 'copilot', label: 'Copilot', icon: CopilotIcon },
  { key: 'cursor', label: 'Cursor', icon: CursorIcon },
  { key: 'bot', label: 'Bot', icon: Bot },
  { key: 'brain', label: 'Brain', icon: Brain },
  { key: 'cpu', label: 'CPU', icon: Cpu },
  { key: 'server', label: 'Server', icon: Server },
  { key: 'terminal', label: 'Terminal', icon: SquareTerminal },
  { key: 'globe', label: 'Globe', icon: Globe },
  { key: 'zap', label: 'Zap', icon: Zap },
  { key: 'sparkles', label: 'Sparkles', icon: Sparkles },
  { key: 'code', label: 'Code', icon: Code2 },
];

const REGISTRY_MAP = new Map(PROVIDER_ICON_REGISTRY.map((e) => [e.key, e.icon]));

export function resolveProviderIcon(key: string | undefined): IconComponent | undefined {
  if (!key) { return undefined; }
  return REGISTRY_MAP.get(key);
}
