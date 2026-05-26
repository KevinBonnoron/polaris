import { agentsCollection } from '@/collections/agents.collection';
import { selectAgent } from '@/state/agent-selection';
import type { SpawnTarget } from '@/types';

export async function startDraftAgent(projectId: string, target: SpawnTarget) {
  const id = crypto.randomUUID();
  await agentsCollection.insert({
    id,
    projectId,
    kind: target.kindId ?? 'opencode',
    providerId: target.providerId,
    status: 'draft',
    startedAt: Date.now(),
    tokens: { input: 0, output: 0, cacheCreation: 0, cacheRead: 0 },
  });
  selectAgent(id);
}
