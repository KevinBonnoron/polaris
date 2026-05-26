import { createContext, type ReactNode, useCallback, useContext, useEffect, useMemo, useState } from 'react';
import { GetAgentDefaultModels, SetAgentDefaultModel } from '@/wailsjs/go/main/App';

type State = {
  get: (id: string) => string | undefined;
  set: (id: string, model: string) => void;
};

const Ctx = createContext<State | null>(null);

export function AgentDefaultsProvider({ children }: { children: ReactNode }) {
  const [map, setMap] = useState<Record<string, string>>({});

  useEffect(() => {
    GetAgentDefaultModels()
      .then(setMap)
      .catch(() => {});
  }, []);

  const get = useCallback((id: string) => map[id], [map]);
  const set = useCallback((id: string, model: string) => {
    setMap((prev) => ({ ...prev, [id]: model }));
    SetAgentDefaultModel(id, model).catch(() => {});
  }, []);

  const value = useMemo<State>(() => ({ get, set }), [get, set]);
  return <Ctx.Provider value={value}>{children}</Ctx.Provider>;
}

export function useAgentDefaults(): State {
  const ctx = useContext(Ctx);
  if (!ctx) {
    throw new Error('useAgentDefaults must be used inside <AgentDefaultsProvider>');
  }
  return ctx;
}
