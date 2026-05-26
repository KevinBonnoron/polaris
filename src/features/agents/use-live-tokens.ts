import { useEffect, useState } from 'react';
import { EventsOn } from '@/wailsjs/runtime/runtime';

export interface TokenParts {
  input: number;
  output: number;
  cacheCreation: number;
  cacheRead: number;
}

interface LiveTokens {
  tokens: number;
  costUsd: number;
  parts: TokenParts;
}

const EVENT = 'agent:tokens:updated';

// useLiveTokens overlays the mid-turn token/cost snapshots pushed over the Wails
// event bus on top of the persisted agent row. The live value is dropped
// whenever the persisted count changes, since persistTurnStats then holds the
// authoritative end-of-turn figure. The backend already folds in the agent's
// prior-turn baseline, so the snapshot is the running total, not just this turn.
export function useLiveTokens(agentId: string | null | undefined, persistedTokens: number): LiveTokens | null {
  const [live, setLive] = useState<LiveTokens | null>(null);

  // biome-ignore lint/correctness/useExhaustiveDependencies: persistedTokens is the reset trigger, read intentionally
  useEffect(() => {
    setLive(null);
  }, [persistedTokens]);

  useEffect(() => {
    if (!agentId) {
      return;
    }
    const handler = (payload: { agentId: string; tokens: number; costUsd: number; parts: TokenParts }) => {
      if (payload.agentId !== agentId) {
        return;
      }
      setLive({ tokens: payload.tokens, costUsd: payload.costUsd, parts: payload.parts });
    };
    // EventsOn returns a per-listener unsubscribe; EventsOff would drop every
    // card's handler for this event, so we must use the returned function.
    return EventsOn(EVENT, handler);
  }, [agentId]);

  return live;
}
