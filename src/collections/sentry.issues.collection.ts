import { type Collection, createCollection } from '@tanstack/db';
import type { ConnectedSentryConfig } from '@/features/integrations/sentry/types';
import { FetchSentryIssues } from '@/wailsjs/go/main/App';
import { sentry } from '@/wailsjs/go/models';

export type SentryIssue = sentry.Issue & { projectSlug: string };

type Listener = () => void;

export type SentryEntry = {
  collection: Collection<SentryIssue, string>;
  loading: boolean;
  error: string | null;
  reload: () => Promise<void>;
  subscribe: (l: Listener) => () => void;
};

const issueKey = (i: SentryIssue): string => `${i.projectSlug}:${i.id}`;

function configKey(cfg: ConnectedSentryConfig): string {
  return `${cfg.url ?? ''}|${cfg.org}|${[...cfg.projects].sort().join(',')}`;
}

const entries = new Map<string, SentryEntry>();

function createSentryEntry(cfg: ConnectedSentryConfig): SentryEntry {
  const listeners = new Set<Listener>();
  const notify = () => {
    for (const l of listeners) {
      l();
    }
  };
  const current = new Map<string, SentryIssue>();

  let syncBegin: (() => void) | null = null;
  let syncWrite: ((op: { type: 'insert' | 'update' | 'delete'; value: SentryIssue }) => void) | null = null;
  let syncCommit: (() => void) | null = null;

  const entry: SentryEntry = {
    collection: null as unknown as Collection<SentryIssue, string>,
    loading: false,
    error: null,
    reload: async () => {},
    subscribe: (l) => {
      listeners.add(l);
      return () => {
        listeners.delete(l);
      };
    },
  };

  const applyIssues = (next: SentryIssue[]) => {
    if (!syncBegin || !syncWrite || !syncCommit) {
      return;
    }
    syncBegin();
    const seen = new Set<string>();
    for (const item of next) {
      const k = issueKey(item);
      seen.add(k);
      const prev = current.get(k);
      if (!prev) {
        syncWrite({ type: 'insert', value: item });
        current.set(k, item);
      } else if (JSON.stringify(prev) !== JSON.stringify(item)) {
        syncWrite({ type: 'update', value: item });
        current.set(k, item);
      }
    }
    for (const [k, v] of Array.from(current)) {
      if (!seen.has(k)) {
        syncWrite({ type: 'delete', value: v });
        current.delete(k);
      }
    }
    syncCommit();
  };

  const refresh = async () => {
    entry.loading = true;
    entry.error = null;
    notify();
    try {
      const perProject = await Promise.all(
        cfg.projects.map(async (project) => {
          const issues = (await FetchSentryIssues(sentry.Config.createFrom({ token: cfg.token, org: cfg.org, project, url: cfg.url }))) ?? [];
          return issues.map((i) => ({ ...i, projectSlug: project }) as SentryIssue);
        }),
      );
      applyIssues(perProject.flat());
    } catch (err) {
      entry.error = err instanceof Error ? err.message : String(err);
    } finally {
      entry.loading = false;
      notify();
    }
  };
  entry.reload = refresh;

  entry.collection = createCollection({
    getKey: issueKey,
    sync: {
      sync: ({ begin, write, commit, markReady }) => {
        syncBegin = begin;
        syncWrite = write as typeof syncWrite;
        syncCommit = commit;
        current.clear();
        refresh().finally(() => markReady());
        return () => {
          syncBegin = null;
          syncWrite = null;
          syncCommit = null;
        };
      },
    },
  }) as unknown as Collection<SentryIssue, string>;

  return entry;
}

export function getSentryEntry(cfg: ConnectedSentryConfig): SentryEntry {
  const key = configKey(cfg);
  const cached = entries.get(key);
  if (cached) {
    return cached;
  }
  const entry = createSentryEntry(cfg);
  entries.set(key, entry);
  return entry;
}
