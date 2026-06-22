// Labels live in i18n under settings.appearance.cardStyles.<key>.
export const CARD_ANIMATION_STYLES = [{ key: 'pulse' }, { key: 'shimmer' }, { key: 'progress' }, { key: 'glow' }, { key: 'none' }] as const;

export type CardAnimationStyleKey = (typeof CARD_ANIMATION_STYLES)[number]['key'];
export const DEFAULT_CARD_ANIMATION_STYLE: CardAnimationStyleKey = 'pulse';
