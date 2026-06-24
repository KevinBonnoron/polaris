import type { ReactNode } from 'react';

export interface UsageLimitRow {
  label: string;
  percentUsed: number;
  resetAt?: string;
  valueLabel?: ReactNode;
}

export function usageBarColor(pct: number): string {
  if (pct >= 90) {
    return 'bg-red-500';
  }
  if (pct >= 70) {
    return 'bg-amber-500';
  }
  return 'bg-emerald-500';
}

export function formatUsageResetAt(iso: string | undefined, locale: string): string | null {
  if (!iso) {
    return null;
  }
  const target = new Date(iso);
  if (Number.isNaN(target.getTime())) {
    return null;
  }
  const now = Date.now();
  const diffMs = target.getTime() - now;

  if (diffMs > 0 && diffMs < 24 * 60 * 60 * 1000) {
    const rtf = new Intl.RelativeTimeFormat(locale, { numeric: 'auto' });
    const diffMin = Math.round(diffMs / 60_000);
    if (diffMin < 60) {
      return rtf.format(diffMin, 'minute');
    }
    return rtf.format(Math.round(diffMin / 60), 'hour');
  }

  return new Intl.DateTimeFormat(locale, { dateStyle: 'medium', timeStyle: 'short' }).format(target);
}

export function formatUsageWindow(minutes: number | undefined, fallback: string): string {
  if (!minutes || minutes <= 0) {
    return fallback;
  }
  if (minutes % (24 * 60) === 0) {
    return `${minutes / (24 * 60)}d`;
  }
  if (minutes % 60 === 0) {
    return `${minutes / 60}h`;
  }
  return `${minutes}m`;
}
