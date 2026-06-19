import { useCallback, useSyncExternalStore } from 'react';
import type { Agent, Project } from '@/types';

export const PROJECT_SORT_KEY = 'polaris:project-sort';
const PROJECT_SORT_EVENT = 'polaris:project-sort-changed';

export type ProjectSort = 'recent' | 'alphabetical';

export function readStoredProjectSort(): ProjectSort {
  if (typeof window === 'undefined') {
    return 'recent';
  }
  try {
    const raw = window.localStorage.getItem(PROJECT_SORT_KEY);
    return raw === 'alphabetical' ? 'alphabetical' : 'recent';
  } catch {
    return 'recent';
  }
}

export function writeStoredProjectSort(mode: ProjectSort): void {
  try {
    window.localStorage.setItem(PROJECT_SORT_KEY, mode);
  } catch {
    // ignore quota errors
  }
  window.dispatchEvent(new Event(PROJECT_SORT_EVENT));
}

function subscribeProjectSort(onChange: () => void): () => void {
  const onStorage = (event: StorageEvent) => {
    if (event.key === PROJECT_SORT_KEY) {
      onChange();
    }
  };
  window.addEventListener(PROJECT_SORT_EVENT, onChange);
  window.addEventListener('storage', onStorage);
  return () => {
    window.removeEventListener(PROJECT_SORT_EVENT, onChange);
    window.removeEventListener('storage', onStorage);
  };
}

export function useProjectSort(): [ProjectSort, (mode: ProjectSort) => void] {
  const sort = useSyncExternalStore(subscribeProjectSort, readStoredProjectSort, () => 'recent' as ProjectSort);
  const setSort = useCallback((mode: ProjectSort) => writeStoredProjectSort(mode), []);
  return [sort, setSort];
}

export function sortProjects(projects: Project[], agents: Agent[], mode: ProjectSort): Project[] {
  const collator = new Intl.Collator(undefined, { sensitivity: 'base' });
  if (mode === 'alphabetical') {
    return [...projects].sort((a, b) => collator.compare(a.name, b.name));
  }
  const lastActivity = new Map<string, number>();
  for (const a of agents) {
    if (a.status === 'draft') {
      continue;
    }
    const ts = a.updatedAt ?? a.startedAt;
    const prev = lastActivity.get(a.projectId) ?? 0;
    if (ts > prev) {
      lastActivity.set(a.projectId, ts);
    }
  }
  return [...projects].sort((a, b) => {
    const diff = (lastActivity.get(b.id) ?? 0) - (lastActivity.get(a.id) ?? 0);
    return diff !== 0 ? diff : collator.compare(a.name, b.name);
  });
}
