export function projectInitials(name: string): string {
  const cleaned = name.trim();
  if (!cleaned) {
    return '?';
  }
  const tokens = cleaned.split(/[\s\-_.]+/).filter(Boolean);
  if (tokens.length >= 2) {
    const first = [...tokens[0]][0] ?? '';
    const second = [...tokens[1]][0] ?? '';
    return (first + second).toUpperCase();
  }
  const chars = [...cleaned];
  return chars.slice(0, Math.min(2, chars.length)).join('').toUpperCase();
}
