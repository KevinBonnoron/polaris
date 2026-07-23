import { type Collection, createCollection } from '@tanstack/db';
import { ReadAgentLogFrom } from '@/wailsjs/go/main/App';
import type { polaris } from '@/wailsjs/go/models';
import { wailsAppendCollectionOptions } from '@/lib/wails-db-collection';

export type AgentLogEvent = polaris.StreamEvent & { _seq: number };

const cache = new Map<string, Collection<AgentLogEvent, string>>();

export function getAgentLogsCollection(agentId: string): Collection<AgentLogEvent, string> {
  const cached = cache.get(agentId);
  if (cached) return cached;

  const collection = createCollection(
    wailsAppendCollectionOptions<polaris.StreamEvent>(`agent-log-${agentId}`, async (offset) => {
      const tail = await ReadAgentLogFrom(agentId, offset);
      return { items: tail.events ?? [], nextOffset: tail.offset };
    }),
  ) as unknown as Collection<AgentLogEvent, string>;

  cache.set(agentId, collection);
  return collection;
}

export function evictAgentLogsCollection(agentId: string): void {
  cache.delete(agentId);
}
