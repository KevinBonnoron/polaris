export function formatRelative(epochSeconds: number, now = Date.now() / 1000): string {
  const diff = Math.max(0, now - epochSeconds);
  if (diff < 60) {
    return 'just now';
  }
  if (diff < 3600) {
    return `${Math.round(diff / 60)}m ago`;
  }
  if (diff < 86400) {
    return `${Math.round(diff / 3600)}h ago`;
  }
  return `${Math.round(diff / 86400)}d ago`;
}
