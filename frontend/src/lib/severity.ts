import { cn } from '@/lib/utils';

/**
 * The app-wide vocabulary for "how did it go / how important is it". Every
 * domain that has its own status words (Sentry levels, agent statuses,
 * notification severities, …) maps into this set so colours stay consistent.
 */
export type Severity = 'neutral' | 'info' | 'success' | 'warning' | 'error';

/** Soft tone — tinted background + readable foreground. For badges, pills, list rows. */
export const SEVERITY_TONE: Record<Severity, string> = {
  neutral: 'bg-muted text-muted-foreground',
  info: 'bg-blue-500/15 text-blue-600 dark:text-blue-400',
  success: 'bg-emerald-500/15 text-emerald-600 dark:text-emerald-400',
  warning: 'bg-amber-500/15 text-amber-600 dark:text-amber-400',
  error: 'bg-red-500/15 text-red-600 dark:text-red-400',
};

/** Solid fill — for status dots and small indicators. */
export const SEVERITY_DOT: Record<Severity, string> = {
  neutral: 'bg-muted-foreground',
  info: 'bg-blue-500',
  success: 'bg-emerald-500',
  warning: 'bg-amber-500',
  error: 'bg-red-500',
};

/** className for a soft-tone Badge — overrides the Badge's default fill and border. */
export function severityBadgeClassName(severity: Severity): string {
  return cn('border-transparent', SEVERITY_TONE[severity]);
}

const SENTRY_LEVEL_SEVERITY: Record<string, Severity> = {
  debug: 'neutral',
  info: 'info',
  warning: 'warning',
  error: 'error',
  fatal: 'error',
};

export function sentryLevelSeverity(level: string | undefined): Severity {
  return SENTRY_LEVEL_SEVERITY[(level ?? '').toLowerCase()] ?? 'error';
}

const AGENT_STATUS_SEVERITY: Record<string, Severity> = {
  working: 'info',
  waiting: 'warning',
  error: 'error',
  completed: 'success',
  stopped: 'neutral',
  idle: 'neutral',
  archived: 'neutral',
};

export function agentStatusSeverity(status: string | undefined): Severity {
  return AGENT_STATUS_SEVERITY[status ?? ''] ?? 'neutral';
}

/** Notifications treat "info" as low-weight, so it maps to the neutral tone. */
export function notificationSeverity(severity: 'info' | 'success' | 'warning' | 'error' | undefined): Severity {
  return severity && severity !== 'info' ? severity : 'neutral';
}
