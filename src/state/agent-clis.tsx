import { createContext, type ReactNode, useCallback, useContext, useEffect, useMemo, useState } from 'react';
import { AGENT_KINDS, type AgentKindConfig } from '@/features/agents/agent-kinds';
import { DetectAgentClis } from '@/wailsjs/go/main/App';
import type { main } from '@/wailsjs/go/models';

export type AgentCliInfo = AgentKindConfig & {
  installed: boolean;
  binary: string;
  path?: string;
};

type State = {
  kinds: AgentCliInfo[];
  loading: boolean;
  refresh: () => Promise<void>;
};

const Ctx = createContext<State | null>(null);

function merge(clis: main.AgentCli[] | null): AgentCliInfo[] {
  const byKind = new Map<string, main.AgentCli>();
  for (const c of clis ?? []) {
    byKind.set(c.kind, c);
  }

  return AGENT_KINDS.map((k) => {
    const cli = byKind.get(k.id);
    return {
      ...k,
      installed: cli?.installed ?? false,
      binary: cli?.binary ?? k.id,
      path: cli?.path,
    };
  });
}

export function AgentClisProvider({ children }: { children: ReactNode }) {
  const [clis, setClis] = useState<main.AgentCli[] | null>(null);
  const [loading, setLoading] = useState(true);

  const refresh = useCallback(async () => {
    setLoading(true);
    try {
      const res = await DetectAgentClis();
      setClis(res);
    } catch {
      setClis([]);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  const kinds = useMemo(() => merge(clis), [clis]);

  const value = useMemo<State>(() => ({ kinds, loading, refresh }), [kinds, loading, refresh]);

  return <Ctx.Provider value={value}>{children}</Ctx.Provider>;
}

export function useAgentClis(): State {
  const ctx = useContext(Ctx);
  if (!ctx) {
    throw new Error('useAgentClis must be used inside <AgentClisProvider>');
  }

  return ctx;
}
