export const CARD_ANIMATION_STYLES = [
  { key: 'pulse', labelFr: 'Anneau' },
  { key: 'shimmer', labelFr: 'Reflet' },
  { key: 'progress', labelFr: 'Barre' },
  { key: 'glow', labelFr: 'Lueur' },
  { key: 'none', labelFr: 'Aucune' },
] as const;

export type CardAnimationStyleKey = (typeof CARD_ANIMATION_STYLES)[number]['key'];
export const DEFAULT_CARD_ANIMATION_STYLE: CardAnimationStyleKey = 'pulse';
