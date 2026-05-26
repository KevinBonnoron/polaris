import { type Collection, createCollection } from '@tanstack/db';
import { ListRepoBranches, ListRepoIssues, ListRepoPullRequests, ListRepoWorkflowRuns } from '@/wailsjs/go/main/App';
import type { gh } from '@/wailsjs/go/models';

type Listener = () => void;

export type GhEntry<T extends object> = {
  collection: Collection<T, string>;
  loading: boolean;
  error: string | null;
  hasMore: boolean;
  reload: () => Promise<void>;
  loadMore: () => Promise<void>;
  subscribe: (l: Listener) => () => void;
};

type Page<T> = { items: T[]; hasMore: boolean };

interface FactoryConfig<T extends object> {
  getKey: (item: T) => string;
  list: (page: number) => Promise<Page<T>>;
}

function createGhEntry<T extends object>(config: FactoryConfig<T>): GhEntry<T> {
  const listeners = new Set<Listener>();
  const notify = () => {
    for (const l of listeners) {
      l();
    }
  };
  const current = new Map<string, T>();
  let loadedPages = 1;

  let syncBegin: (() => void) | null = null;
  let syncWrite: ((op: { type: 'insert' | 'update' | 'delete'; value: T }) => void) | null = null;
  let syncCommit: (() => void) | null = null;

  const entry: GhEntry<T> = {
    collection: null as unknown as GhEntry<T>['collection'],
    loading: false,
    error: null,
    hasMore: false,
    reload: async () => {},
    loadMore: async () => {},
    subscribe: (l) => {
      listeners.add(l);
      return () => {
        listeners.delete(l);
      };
    },
  };

  const applyDiff = (next: T[], { replace }: { replace: boolean }) => {
    if (!syncBegin || !syncWrite || !syncCommit) {
      return;
    }
    syncBegin();
    const seen = new Set<string>();
    for (const item of next) {
      const k = config.getKey(item);
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
    if (replace) {
      for (const [k, v] of Array.from(current)) {
        if (!seen.has(k)) {
          syncWrite({ type: 'delete', value: v });
          current.delete(k);
        }
      }
    }
    syncCommit();
  };

  const refresh = async () => {
    entry.loading = true;
    entry.error = null;
    notify();
    try {
      const pages: T[] = [];
      let hasMore = false;
      for (let p = 1; p <= loadedPages; p++) {
        const page = await config.list(p);
        pages.push(...(page.items ?? []));
        hasMore = page.hasMore;
        if (!page.hasMore && p < loadedPages) {
          loadedPages = p;
          break;
        }
      }
      entry.hasMore = hasMore;
      applyDiff(pages, { replace: true });
    } catch (err) {
      entry.error = err instanceof Error ? err.message : String(err);
    } finally {
      entry.loading = false;
      notify();
    }
  };

  const loadMore = async () => {
    if (entry.loading || !entry.hasMore) {
      return;
    }
    entry.loading = true;
    entry.error = null;
    notify();
    try {
      const nextPage = loadedPages + 1;
      const page = await config.list(nextPage);
      loadedPages = nextPage;
      entry.hasMore = page.hasMore;
      applyDiff(page.items ?? [], { replace: false });
    } catch (err) {
      entry.error = err instanceof Error ? err.message : String(err);
    } finally {
      entry.loading = false;
      notify();
    }
  };

  entry.reload = refresh;
  entry.loadMore = loadMore;

  entry.collection = createCollection({
    getKey: config.getKey,
    sync: {
      sync: ({ markReady, begin, write, commit }) => {
        syncBegin = begin;
        syncWrite = write as typeof syncWrite;
        syncCommit = commit;
        refresh().finally(() => markReady());
        return () => {
          syncBegin = null;
          syncWrite = null;
          syncCommit = null;
        };
      },
    },
  }) as unknown as Collection<T, string>;

  return entry;
}

const singlePage = <T>(items: T[] | null | undefined): Page<T> => ({ items: items ?? [], hasMore: false });

export type Branch = { name: string };

const prCache = new Map<string, GhEntry<gh.PullRequest>>();
const issueCache = new Map<string, GhEntry<gh.Issue>>();
const runsCache = new Map<string, GhEntry<gh.WorkflowRun>>();
const branchCache = new Map<string, GhEntry<Branch>>();

const repoKey = (owner: string, repo: string) => `${owner}/${repo}`;

export function getPullRequestsEntry(owner: string, repo: string): GhEntry<gh.PullRequest> {
  const key = repoKey(owner, repo);
  const cached = prCache.get(key);
  if (cached) {
    return cached;
  }
  const entry = createGhEntry<gh.PullRequest>({
    getKey: (p) => String(p.number),
    list: async () => singlePage((await ListRepoPullRequests(owner, repo)) as gh.PullRequest[]),
  });
  prCache.set(key, entry);
  return entry;
}

export function getIssuesEntry(owner: string, repo: string): GhEntry<gh.Issue> {
  const key = repoKey(owner, repo);
  const cached = issueCache.get(key);
  if (cached) {
    return cached;
  }
  const entry = createGhEntry<gh.Issue>({
    getKey: (i) => String(i.number),
    list: async () => singlePage((await ListRepoIssues(owner, repo)) as gh.Issue[]),
  });
  issueCache.set(key, entry);
  return entry;
}

export function getWorkflowRunsEntry(owner: string, repo: string): GhEntry<gh.WorkflowRun> {
  const key = repoKey(owner, repo);
  const cached = runsCache.get(key);
  if (cached) {
    return cached;
  }
  const entry = createGhEntry<gh.WorkflowRun>({
    getKey: (r) => String(r.id),
    list: async (page) => {
      const result = await ListRepoWorkflowRuns(owner, repo, page);
      return { items: (result?.runs ?? []) as gh.WorkflowRun[], hasMore: Boolean(result?.hasMore) };
    },
  });
  runsCache.set(key, entry);
  return entry;
}

export function getBranchesEntry(owner: string, repo: string): GhEntry<Branch> {
  const key = repoKey(owner, repo);
  const cached = branchCache.get(key);
  if (cached) {
    return cached;
  }
  const entry = createGhEntry<Branch>({
    getKey: (b) => b.name,
    list: async () => singlePage(((await ListRepoBranches(owner, repo)) ?? []).map((name) => ({ name }))),
  });
  branchCache.set(key, entry);
  return entry;
}
