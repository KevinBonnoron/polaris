import { createCollection } from '@tanstack/db';
import { DeleteAgent, ListAgents, UpsertAgent } from '@/wailsjs/go/main/App';
import { wailsCollectionOptions } from '../lib/wails-db-collection';
import type { Agent } from '../types';

export const agentsCollection = createCollection(
  wailsCollectionOptions<Agent>({
    name: 'agents',
    list: () => ListAgents('') as unknown as Promise<Agent[]>,
    upsert: (a) => UpsertAgent(a as never) as unknown as Promise<Agent>,
    remove: (id) => DeleteAgent(id),
  }),
);
