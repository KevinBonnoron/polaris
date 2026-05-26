import { createCollection } from '@tanstack/db';
import { useLiveQuery } from '@tanstack/react-db';
import { useEffect, useState } from 'react';
import { getJiraEntry, type JiraStatus } from '@/collections/jira.issues.collection';
import { ListJiraBoards } from '@/wailsjs/go/main/App';
import { tickets } from '@/wailsjs/go/models';
import type { ConnectedJiraConfig, Issue, Sprint } from './types';

export type SprintView = {
  issues: Issue[];
  meta: { name: string; boardId: number; columns: Sprint['columns'] } | null;
  loading: boolean;
  error: string | null;
  pendingKeys: Set<string>;
  reload: () => Promise<void>;
  transition: (issueKey: string, targetStatusIds: string[]) => Promise<void>;
};

export function useJiraSprint(cfg: ConnectedJiraConfig): SprintView {
  const entry = getJiraEntry(cfg);

  // Subscribe to meta/loading/error/pending changes that live outside the collection.
  const [, setTick] = useState(0);
  useEffect(() => entry.subscribe(() => setTick((t) => t + 1)), [entry]);

  const { data: issues } = useLiveQuery((q) => q.from({ i: entry.collection }), [entry]);

  return {
    issues: (issues ?? []) as Issue[],
    meta: entry.meta,
    loading: entry.loading,
    error: entry.error,
    pendingKeys: entry.pendingKeys,
    reload: entry.reload,
    transition: (issueKey, targetStatusIds) => entry.transitionIssue(issueKey, targetStatusIds),
  };
}

const emptyStatusesCollection = createCollection<JiraStatus, string>({
  getKey: (s) => s.name,
  sync: {
    sync: ({ markReady }) => {
      markReady();
      return () => {};
    },
  },
});

export type StatusesView = {
  statuses: JiraStatus[];
  loading: boolean;
  error: string | null;
};

export type BoardOption = { id: number; name: string; type: string };

type BoardsEntry = {
  boards: BoardOption[];
  loading: boolean;
  error: string | null;
  inflight: Promise<void> | null;
  listeners: Set<() => void>;
};

const boardsCache = new Map<string, BoardsEntry>();

function notifyBoards(entry: BoardsEntry) {
  entry.listeners.forEach((listener) => {
    listener();
  });
}

function fetchBoards(cfg: { baseUrl: string; email: string; token: string; projectKey: string }, entry: BoardsEntry) {
  if (entry.inflight) {
    return entry.inflight;
  }
  entry.loading = true;
  entry.error = null;
  notifyBoards(entry);
  const p = ListJiraBoards(tickets.Config.createFrom(cfg))
    .then((list) => {
      entry.boards = (list ?? []).map((b) => ({ id: b.id, name: b.name, type: b.type }));
      entry.error = null;
    })
    .catch((err) => {
      entry.error = err instanceof Error ? err.message : String(err);
    })
    .finally(() => {
      entry.loading = false;
      entry.inflight = null;
      notifyBoards(entry);
    });
  entry.inflight = p;
  return p;
}

export function useJiraBoards(cfg: { baseUrl?: string; email?: string; token?: string; projectKey?: string } | null): {
  boards: BoardOption[];
  loading: boolean;
  error: string | null;
} {
  const key = cfg?.baseUrl && cfg.email && cfg.token && cfg.projectKey ? `${cfg.baseUrl}|${cfg.email}|${cfg.projectKey}` : null;

  const [, setTick] = useState(0);

  useEffect(() => {
    if (!key || !cfg?.baseUrl || !cfg.email || !cfg.token || !cfg.projectKey) {
      return;
    }

    let entry = boardsCache.get(key);
    if (!entry) {
      entry = { boards: [], loading: false, error: null, inflight: null, listeners: new Set() };
      boardsCache.set(key, entry);
    }
    const listener = () => setTick((t) => t + 1);
    entry.listeners.add(listener);
    if (entry.boards.length === 0 && !entry.inflight && !entry.error) {
      fetchBoards({ baseUrl: cfg.baseUrl, email: cfg.email, token: cfg.token, projectKey: cfg.projectKey }, entry);
    }
    return () => {
      entry?.listeners.delete(listener);
    };
  }, [key, cfg?.baseUrl, cfg?.email, cfg?.token, cfg?.projectKey]);

  const entry = key ? boardsCache.get(key) : null;
  return {
    boards: entry?.boards ?? [],
    loading: entry?.loading ?? false,
    error: entry?.error ?? null,
  };
}

export function useJiraStatuses(cfg: ConnectedJiraConfig | null): StatusesView {
  const entry = cfg ? getJiraEntry(cfg) : null;

  const [, setTick] = useState(0);
  useEffect(() => entry?.subscribe(() => setTick((t) => t + 1)), [entry]);

  const { data: statuses = [] } = useLiveQuery((q) => (entry ? q.from({ s: entry.statusesCollection }) : q.from({ s: emptyStatusesCollection })), [entry]);

  return {
    statuses,
    loading: entry?.loading ?? false,
    error: entry?.error ?? null,
  };
}
