export const PROJECT_COLORS = [
  '#3b82f6', // blue
  '#10b981', // emerald
  '#f59e0b', // amber
  '#ef4444', // red
  '#8b5cf6', // violet
  '#ec4899', // pink
  '#14b8a6', // teal
  '#f97316', // orange
] as const;

export function randomProjectColor(): string {
  const idx = Math.floor(Math.random() * PROJECT_COLORS.length);
  return PROJECT_COLORS[idx] ?? PROJECT_COLORS[0];
}

export function deriveProjectName(path: string): string {
  const trimmed = path.replace(/[/\\]+$/, '');
  const parts = trimmed.split(/[/\\]/);
  return parts[parts.length - 1] || trimmed;
}
