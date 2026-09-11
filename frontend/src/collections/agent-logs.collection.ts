import { type Collection, createCollection } from '@tanstack/db';
import { ReadAgentLogFrom } from '@/wailsjs/go/main/App';
import type { polaris } from '@/wailsjs/go/models';
import { wailsAppendCollectionOptions } from '@/lib/wails-db-collection';

export type AgentLogEvent = polaris.StreamEvent & { _seq: number };

// Branded as non-single-result so useLiveQuery resolves to the array overload
// without requiring a cast at the call site.
type AgentLogsCollection = Collection<AgentLogEvent, string> & { singleResult?: never };

const cache = new Map<string, AgentLogsCollection>();

export function getAgentLogsCollection(agentId: string): AgentLogsCollection {
  const cached = cache.get(agentId);
  if (cached) return cached;

  const collection = createCollection(
    wailsAppendCollectionOptions<polaris.StreamEvent>(`agent-log-${agentId}`, async (offset) => {
      const tail = await ReadAgentLogFrom(agentId, offset);
      return { items: tail.events ?? [], nextOffset: tail.offset };
    }),
  ) as unknown as AgentLogsCollection;

  cache.set(agentId, collection);
  return collection;
}

export function evictAgentLogsCollection(agentId: string): void {
  cache.delete(agentId);
}
