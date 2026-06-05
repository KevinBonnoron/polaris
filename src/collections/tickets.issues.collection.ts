import { type Collection, createCollection } from '@tanstack/db';
import type { ConnectedTicketsConfig } from '@/features/integrations/tickets/types';
import { FetchTicketsSprint, TransitionTicketsIssue } from '@/wailsjs/go/main/App';
import { tickets } from '@/wailsjs/go/models';

type SprintMeta = {
  name: string;
  boardId: number;
  boardUrl: string;
  columns: tickets.Column[];
};

export type TicketsStatus = {
  name: string;
  id: string;
  statusIds: string[];
};

type Listener = () => void;

type Entry = {
  collection: Collection<tickets.Issue, string>;
  statusesCollection: Collection<TicketsStatus, string>;
  meta: SprintMeta | null;
  loading: boolean;
  error: string | null;
  pendingKeys: Set<string>;
  pendingTargets: Map<string, string[]>;
  reload: () => Promise<void>;
  transitionIssue: (issueKey: string, targetStatusIDs: string[]) => Promise<void>;
  subscribe: (l: Listener) => () => void;
};

function configKey(cfg: ConnectedTicketsConfig): string {
  return `${cfg.baseUrl}|${cfg.email}|${cfg.projectKey}|${cfg.boardId ?? ''}|${cfg.token}`;
}

function computeStatuses(sprint: tickets.Sprint): TicketsStatus[] {
  const byName = new Map<string, TicketsStatus>();
  const knownIds = new Set<string>();

  for (const issue of sprint.issues ?? []) {
    if (!issue.status) {
      continue;
    }
    const existing = byName.get(issue.status);
    if (!existing) {
      byName.set(issue.status, { name: issue.status, id: issue.statusId, statusIds: [issue.statusId] });
    } else if (!existing.statusIds.includes(issue.statusId)) {
      existing.statusIds.push(issue.statusId);
    }
    knownIds.add(issue.statusId);
  }

  for (const col of sprint.columns ?? []) {
    for (const id of col.statusIds ?? []) {
      if (knownIds.has(id)) {
        continue;
      }
      const existing = byName.get(col.name);
      if (!existing) {
        byName.set(col.name, { name: col.name, id, statusIds: [id] });
      } else if (!existing.statusIds.includes(id)) {
        existing.statusIds.push(id);
      }
      knownIds.add(id);
    }
  }

  return Array.from(byName.values());
}

const entries = new Map<string, Entry>();

export function getTicketsEntry(cfg: ConnectedTicketsConfig): Entry {
  const key = configKey(cfg);
  const existing = entries.get(key);
  if (existing) {
    return existing;
  }

  const listeners = new Set<Listener>();
  const notify = () => {
    listeners.forEach((l) => {
      l();
    });
  };
  const currentIssues = new Map<string, tickets.Issue>();
  const currentStatuses = new Map<string, TicketsStatus>();

  let issuesBegin: (() => void) | null = null;
  let issuesWrite: ((op: { type: 'insert' | 'update' | 'delete'; value: tickets.Issue }) => void) | null = null;
  let issuesCommit: (() => void) | null = null;

  let statusesBegin: (() => void) | null = null;
  let statusesWrite: ((op: { type: 'insert' | 'update' | 'delete'; value: TicketsStatus }) => void) | null = null;
  let statusesCommit: (() => void) | null = null;

  const entry: Entry = {
    collection: null as unknown as Collection<tickets.Issue, string>,
    statusesCollection: null as unknown as Collection<TicketsStatus, string>,
    meta: null,
    loading: false,
    error: null,
    pendingKeys: new Set<string>(),
    pendingTargets: new Map<string, string[]>(),
    reload: async () => {},
    transitionIssue: async () => {},
    subscribe: (l) => {
      listeners.add(l);
      return () => {
        listeners.delete(l);
      };
    },
  };

  const syncIssues = (sprint: tickets.Sprint) => {
    if (!issuesBegin || !issuesWrite || !issuesCommit) {
      return;
    }
    issuesBegin();
    const seen = new Set<string>();
    for (const issue of sprint.issues ?? []) {
      seen.add(issue.key);
      const prev = currentIssues.get(issue.key);
      if (!prev) {
        issuesWrite({ type: 'insert', value: issue });
        currentIssues.set(issue.key, issue);
      } else if (JSON.stringify(prev) !== JSON.stringify(issue)) {
        issuesWrite({ type: 'update', value: issue });
        currentIssues.set(issue.key, issue);
      }
    }
    for (const [k, v] of Array.from(currentIssues)) {
      if (!seen.has(k)) {
        issuesWrite({ type: 'delete', value: v });
        currentIssues.delete(k);
      }
    }
    issuesCommit();
  };

  const syncStatuses = (sprint: tickets.Sprint) => {
    if (!statusesBegin || !statusesWrite || !statusesCommit) {
      return;
    }
    const next = computeStatuses(sprint);
    statusesBegin();
    const seen = new Set<string>();
    for (const status of next) {
      seen.add(status.name);
      const prev = currentStatuses.get(status.name);
      if (!prev) {
        statusesWrite({ type: 'insert', value: status });
        currentStatuses.set(status.name, status);
      } else if (prev.id !== status.id || prev.statusIds.join('|') !== status.statusIds.join('|')) {
        statusesWrite({ type: 'update', value: status });
        currentStatuses.set(status.name, status);
      }
    }
    for (const [k, v] of Array.from(currentStatuses)) {
      if (!seen.has(k)) {
        statusesWrite({ type: 'delete', value: v });
        currentStatuses.delete(k);
      }
    }
    statusesCommit();
  };

  let inflight: Promise<void> | null = null;
  const refresh = () => {
    if (inflight) {
      return inflight;
    }
    entry.loading = true;
    entry.error = null;
    notify();
    inflight = (async () => {
      try {
        const sprint = await FetchTicketsSprint(tickets.Config.createFrom(cfg));
        entry.meta = { name: sprint.name, boardId: sprint.boardId, boardUrl: sprint.boardUrl ?? '', columns: sprint.columns ?? [] };
        syncIssues(sprint);
        syncStatuses(sprint);
      } catch (err) {
        entry.error = err instanceof Error ? err.message : String(err);
      } finally {
        entry.loading = false;
        inflight = null;
        notify();
      }
    })();
    return inflight;
  };
  entry.reload = refresh;

  const writeIssue = (next: tickets.Issue) => {
    if (!issuesBegin || !issuesWrite || !issuesCommit) {
      return;
    }
    issuesBegin();
    issuesWrite({ type: 'update', value: next });
    issuesCommit();
    currentIssues.set(next.key, next);
  };

  entry.transitionIssue = async (issueKey, targetStatusIDs) => {
    const original = currentIssues.get(issueKey);
    if (!original || targetStatusIDs.length === 0) {
      return;
    }
    const optimisticStatusId = targetStatusIDs.includes(original.statusId) ? original.statusId : targetStatusIDs[0];
    if (optimisticStatusId !== original.statusId) {
      writeIssue({ ...original, statusId: optimisticStatusId });
    }
    entry.pendingKeys.add(issueKey);
    entry.pendingTargets.set(issueKey, targetStatusIDs);
    notify();
    try {
      await TransitionTicketsIssue(tickets.Config.createFrom(cfg), issueKey, targetStatusIDs);
    } catch (err) {
      writeIssue(original);
      entry.error = err instanceof Error ? err.message : String(err);
      throw err;
    } finally {
      entry.pendingKeys.delete(issueKey);
      entry.pendingTargets.delete(issueKey);
      notify();
    }
  };

  entry.collection = createCollection({
    getKey: (i: tickets.Issue) => i.key,
    sync: {
      sync: ({ begin, write, commit, markReady }) => {
        issuesBegin = begin;
        issuesWrite = write as typeof issuesWrite;
        issuesCommit = commit;
        currentIssues.clear();
        refresh().finally(() => markReady());
        return () => {
          issuesBegin = null;
          issuesWrite = null;
          issuesCommit = null;
        };
      },
    },
  }) as unknown as Collection<tickets.Issue, string>;

  entry.statusesCollection = createCollection({
    getKey: (s: TicketsStatus) => s.name,
    sync: {
      sync: ({ begin, write, commit, markReady }) => {
        statusesBegin = begin;
        statusesWrite = write as typeof statusesWrite;
        statusesCommit = commit;
        currentStatuses.clear();
        refresh().finally(() => markReady());
        return () => {
          statusesBegin = null;
          statusesWrite = null;
          statusesCommit = null;
        };
      },
    },
  }) as unknown as Collection<TicketsStatus, string>;

  entries.set(key, entry);
  return entry;
}
