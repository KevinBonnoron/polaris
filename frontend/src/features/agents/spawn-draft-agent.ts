import { createOptimisticAction } from '@tanstack/db';
import { agentsCollection } from '@/collections/agents.collection';
import { SpawnAgent } from '@/wailsjs/go/main/App';

type SpawnInput = Parameters<typeof SpawnAgent>[0];

// Promote a draft agent to a running one through TanStack DB's native optimism:
// onMutate flips the draft to "working" in the collection instantly, while the
// transaction's mutationFn runs the real spawn in the background. Running the
// mutation inside the transaction bypasses the collection's generic UpsertAgent
// handler, and SpawnAgent reuses the draft id so the existing row is updated in
// place and reconciled by the next sync — no new row, no flicker, no delete.
export const spawnDraftAgent = createOptimisticAction<{ id: string; input: SpawnInput }>({
  onMutate: ({ id }) => {
    agentsCollection.update(id, (d) => {
      d.status = 'working';
    });
  },
  mutationFn: async ({ id, input }) => {
    await SpawnAgent({ ...input, id });
  },
});
