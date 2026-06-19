import { useEffect, useState } from 'react';
import { GetRepoWorkflowDispatch } from '@/wailsjs/go/main/App';
import type { WorkflowDispatchSpec } from './types';

const cache = new Map<string, WorkflowDispatchSpec | null>();

export function useWorkflowDispatches(owner: string, repo: string, workflowIds: number[]): Map<number, WorkflowDispatchSpec | null | undefined> {
  const key = workflowIds.join(',');
  const [specs, setSpecs] = useState<Map<number, WorkflowDispatchSpec | null | undefined>>(() => seed(owner, repo, workflowIds));

  // biome-ignore lint/correctness/useExhaustiveDependencies: workflowIds joined into stable key
  useEffect(() => {
    if (workflowIds.length === 0) {
      setSpecs(new Map());
      return;
    }
    let cancelled = false;
    setSpecs(seed(owner, repo, workflowIds));
    Promise.all(
      workflowIds.map(async (id) => {
        const cacheKey = `${owner}/${repo}#${id}`;
        const cached = cache.get(cacheKey);
        if (cached !== undefined) {
          return [id, cached] as const;
        }
        try {
          const spec = await GetRepoWorkflowDispatch(owner, repo, id);
          cache.set(cacheKey, spec);
          return [id, spec] as const;
        } catch {
          cache.set(cacheKey, null);
          return [id, null] as const;
        }
      }),
    ).then((entries) => {
      if (cancelled) {
        return;
      }

      setSpecs(new Map(entries));
    });
    return () => {
      cancelled = true;
    };
  }, [owner, repo, key]);

  return specs;
}

function seed(owner: string, repo: string, ids: number[]): Map<number, WorkflowDispatchSpec | null | undefined> {
  const map = new Map<number, WorkflowDispatchSpec | null | undefined>();
  for (const id of ids) {
    map.set(id, cache.get(`${owner}/${repo}#${id}`));
  }
  return map;
}
