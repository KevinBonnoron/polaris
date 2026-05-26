export const THINKING_STYLES = [
  { key: 'dots', labelFr: 'Points', labelEn: 'Dots' },
  { key: 'spinner', labelFr: 'Spinner', labelEn: 'Spinner' },
  { key: 'pill', labelFr: 'Badge', labelEn: 'Badge' },
  { key: 'bar', labelFr: 'Barre', labelEn: 'Bar' },
  { key: 'wave', labelFr: 'Vague', labelEn: 'Wave' },
  { key: 'orbit', labelFr: 'Orbite', labelEn: 'Orbit' },
  { key: 'typing', labelFr: 'Curseur', labelEn: 'Cursor' },
  { key: 'breathing', labelFr: 'Respiration', labelEn: 'Breathing' },
  { key: 'sine', labelFr: 'Sinus', labelEn: 'Sine wave' },
  { key: 'ellipsis', labelFr: 'Ellipsis', labelEn: 'Ellipsis' },
  { key: 'ring', labelFr: 'Anneau', labelEn: 'Ring' },
  { key: 'cascade', labelFr: 'Cascade', labelEn: 'Cascade' },
  { key: 'gradient', labelFr: 'Dégradé', labelEn: 'Gradient' },
  { key: 'bounce', labelFr: 'Rebond', labelEn: 'Bounce' },
  { key: 'flicker', labelFr: 'Scintillement', labelEn: 'Flicker' },
] as const;

export type ThinkingStyleKey = (typeof THINKING_STYLES)[number]['key'];
export const DEFAULT_THINKING_STYLE: ThinkingStyleKey = 'dots';
