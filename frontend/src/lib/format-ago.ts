export function formatAgo(unixSeconds: number, locale: string): string {
  if (!unixSeconds) {
    return '—';
  }
  const diff = Math.max(0, Date.now() / 1000 - unixSeconds);
  const rtf = new Intl.RelativeTimeFormat(locale, { numeric: 'auto' });
  if (diff < 60) {
    return rtf.format(-Math.round(diff), 'second');
  }
  if (diff < 3600) {
    return rtf.format(-Math.round(diff / 60), 'minute');
  }
  if (diff < 86400) {
    return rtf.format(-Math.round(diff / 3600), 'hour');
  }
  return rtf.format(-Math.round(diff / 86400), 'day');
}
