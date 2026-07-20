import type { tickets } from '@/wailsjs/go/models';

export type TicketsConfig = {
  baseUrl?: string;
  email?: string;
  token?: string;
  projectKey?: string;
  boardId?: number;
  hiddenColumnsByBoard?: Record<string, string[]>;
  customFields?: Array<{ id: string; label: string }>;
};

export type ConnectedTicketsConfig = {
  baseUrl: string;
  email: string;
  token: string;
  projectKey: string;
  boardId?: number;
  customFields?: Array<{ id: string; label: string }>;
};

export type Issue = tickets.Issue;
export type Column = tickets.Column;
export type Sprint = tickets.Sprint;

export type BoardColumn = {
  name: string;
  issues: Issue[];
};

const FALLBACK_COLUMNS: { name: string; categories: string[]; statuses: string[] }[] = [
  { name: 'To Do', categories: ['new', 'undefined'], statuses: ['to do', 'open', 'backlog'] },
  { name: 'In Progress', categories: ['indeterminate'], statuses: ['in progress', 'doing'] },
  { name: 'Review', categories: [], statuses: ['review', 'in review', 'code review', 'qa', 'testing'] },
  { name: 'Done', categories: ['done'], statuses: ['done', 'closed', 'resolved'] },
];

export function groupIssues(issues: Issue[], columns: Column[] | undefined): BoardColumn[] {
  if (columns?.length) {
    const buckets: BoardColumn[] = columns.map((c) => ({ name: c.name, issues: [] }));
    const lookup = new Map<string, number>();
    columns.forEach((c, idx) => {
      for (const id of c.statusIds ?? []) {
        lookup.set(id, idx);
      }
    });
    const overflow: BoardColumn = { name: 'Other', issues: [] };
    for (const issue of issues) {
      const idx = lookup.get(issue.statusId);
      if (idx === undefined) {
        overflow.issues.push(issue);
      } else {
        buckets[idx].issues.push(issue);
      }
    }
    if (overflow.issues.length) {
      buckets.push(overflow);
    }
    return buckets;
  }

  const buckets: BoardColumn[] = FALLBACK_COLUMNS.map((c) => ({ name: c.name, issues: [] }));
  for (const issue of issues) {
    const status = issue.status?.toLowerCase() ?? '';
    const cat = issue.statusCategory?.toLowerCase() ?? '';
    const idx = FALLBACK_COLUMNS.findIndex((c) => c.statuses.includes(status) || c.categories.includes(cat));
    buckets[idx === -1 ? 0 : idx].issues.push(issue);
  }
  return buckets;
}
