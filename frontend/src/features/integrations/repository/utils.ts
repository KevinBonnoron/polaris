import type { RepoConfig, WorkflowRun } from './types';

export { formatAgo } from '@/lib/format-ago';

export function buildRemoteUrl(config: RepoConfig): string | null {
  if (!config.owner || !config.repo) {
    return null;
  }
  const base = (config.baseUrl ?? defaultBaseUrl(config.provider)).replace(/\/+$/, '');
  return `${base}/${config.owner}/${config.repo}`;
}

function defaultBaseUrl(provider?: string): string {
  switch (provider) {
    case 'gitlab':
      return 'https://gitlab.com';
    case 'bitbucket':
      return 'https://bitbucket.org';
    default:
      return 'https://github.com';
  }
}

export function humanizeError(msg: string): string {
  if (msg.includes('gh CLI not installed')) {
    return 'Install the gh CLI and run `gh auth login` to enable live data.';
  }
  return msg;
}

export function runDurationSeconds(run: WorkflowRun): number | null {
  const start = run.runStartedAt || run.createdAt;
  if (!start) {
    return null;
  }
  const end = run.status === 'completed' ? run.updatedAt : Math.floor(Date.now() / 1000);
  if (!end || end < start) {
    return null;
  }
  return end - start;
}

export function formatDuration(seconds: number): string {
  if (seconds < 1) {
    return '0s';
  }
  const h = Math.floor(seconds / 3600);
  const m = Math.floor((seconds % 3600) / 60);
  const s = seconds % 60;
  if (h > 0) {
    return `${h}h ${m}m`;
  }
  if (m > 0) {
    return `${m}m ${s}s`;
  }
  return `${s}s`;
}

export function runColor(run: WorkflowRun): string {
  if (run.status !== 'completed') {
    return 'text-amber-500';
  }
  switch (run.conclusion) {
    case 'success':
      return 'text-emerald-500';
    case 'failure':
    case 'timed_out':
    case 'startup_failure':
      return 'text-destructive';
    case 'cancelled':
    case 'skipped':
      return 'text-muted-foreground';
    default:
      return 'text-muted-foreground';
  }
}

export function runLabel(run: WorkflowRun): string {
  if (run.status !== 'completed') {
    return run.status.replace(/_/g, ' ');
  }
  return run.conclusion || 'completed';
}

export function readableTextColor(hex: string): string {
  if (!hex || hex.length !== 6) {
    return '#fff';
  }
  const r = parseInt(hex.slice(0, 2), 16);
  const g = parseInt(hex.slice(2, 4), 16);
  const b = parseInt(hex.slice(4, 6), 16);
  // perceived luminance (Rec. 601)
  const luminance = (0.299 * r + 0.587 * g + 0.114 * b) / 255;
  return luminance > 0.6 ? '#111' : '#fff';
}
