// Labels live in i18n under settings.appearance.thinkingStyles.<key>.
export const THINKING_STYLES = [
  { key: 'dots' },
  { key: 'spinner' },
  { key: 'pill' },
  { key: 'bar' },
  { key: 'wave' },
  { key: 'orbit' },
  { key: 'typing' },
  { key: 'breathing' },
  { key: 'sine' },
  { key: 'ellipsis' },
  { key: 'ring' },
  { key: 'cascade' },
  { key: 'gradient' },
  { key: 'bounce' },
  { key: 'flicker' },
] as const;

export type ThinkingStyleKey = (typeof THINKING_STYLES)[number]['key'];
export const DEFAULT_THINKING_STYLE: ThinkingStyleKey = 'dots';
