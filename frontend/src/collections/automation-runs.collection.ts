import { type Collection, createCollection } from '@tanstack/db';
import { ListAutomationRuns } from '@/wailsjs/go/main/App';
import type { polaris } from '@/wailsjs/go/models';
import { wailsCollectionOptions } from '@/lib/wails-db-collection';

const cache = new Map<string, Collection<polaris.AutomationRun, string>>();

export function getAutomationRunsCollection(automationId: string): Collection<polaris.AutomationRun, string> {
  const cached = cache.get(automationId);
  if (cached) return cached;

  const collection = createCollection(
    wailsCollectionOptions<polaris.AutomationRun>({
      name: `automationRuns-${automationId}`,
      list: () => ListAutomationRuns(automationId, 20) as unknown as Promise<polaris.AutomationRun[]>,
    }),
  );

  cache.set(automationId, collection);
  return collection;
}

export function evictAutomationRunsCollection(automationId: string): void {
  cache.delete(automationId);
}
