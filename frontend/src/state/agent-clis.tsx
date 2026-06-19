import { createContext, type ReactNode, useCallback, useContext, useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { AGENT_KINDS, type AgentKindConfig, type AgentModel, OPENCODE_DESCRIPTOR } from '@/features/agents/agent-kinds';
import { DetectAgentClis, FetchClaudeModels, ListCliModels, ListOpencodeModels } from '@/wailsjs/go/main/App';
import type { main, polaris } from '@/wailsjs/go/models';

export type AgentCliInfo = AgentKindConfig & {
  models: AgentModel[];
  installed: boolean;
  binary: string;
  path?: string;
};

type State = {
  kinds: AgentCliInfo[];
  opencode: AgentCliInfo;
  opencodeInstalled: boolean;
  loading: boolean;
  refresh: () => Promise<void>;
};

function mergeOne(descriptor: AgentKindConfig, clis: main.AgentCli[] | null, models: AgentModel[] = []): AgentCliInfo {
  const cli = (clis ?? []).find((c) => c.kind === descriptor.id);
  return { ...descriptor, models, installed: cli?.installed ?? false, binary: cli?.binary ?? descriptor.id, path: cli?.path };
}

const Ctx = createContext<State | null>(null);

type Translate = (key: string) => string;

function modelInfoToOption(m: polaris.ModelInfo, t: Translate): AgentModel {
  if (m.family) {
    const label = m.name.replace(/^Claude\s+/, ''); // e.g. "Opus 4.8"
    const taglineKey = `agents.modelTagline.${m.family}`;
    const tagline = t(taglineKey);
    return { value: m.value, label, description: tagline !== taglineKey ? tagline : undefined };
  }
  return { value: m.value, label: m.name };
}

function merge(clis: main.AgentCli[] | null, claudeModels: polaris.ModelInfo[], cliModels: Map<string, polaris.ModelInfo[]>, t: Translate): AgentCliInfo[] {
  const byKind = new Map<string, main.AgentCli>();
  for (const c of clis ?? []) {
    byKind.set(c.kind, c);
  }

  return AGENT_KINDS.map((k) => {
    const cli = byKind.get(k.id);
    const infos = k.id === 'claude-code' ? claudeModels : (cliModels.get(k.id) ?? []);
    return {
      ...k,
      models: infos.map((m) => modelInfoToOption(m, t)),
      installed: cli?.installed ?? false,
      binary: cli?.binary ?? k.id,
      path: cli?.path,
    };
  });
}

export function AgentClisProvider({ children }: { children: ReactNode }) {
  const { t } = useTranslation();
  const [clis, setClis] = useState<main.AgentCli[] | null>(null);
  const [opencodeModels, setOpencodeModels] = useState<string[]>([]);
  const [claudeModels, setClaudeModels] = useState<polaris.ModelInfo[]>([]);
  const [cliModels, setCliModels] = useState<Map<string, polaris.ModelInfo[]>>(new Map());
  const [loading, setLoading] = useState(true);

  const refresh = useCallback(async () => {
    setLoading(true);
    try {
      const res = await DetectAgentClis();
      setClis(res);

      const opencodeInstalled = res.some((c) => c.kind === 'opencode' && c.installed);
      const claudeInstalled = res.some((c) => c.kind === 'claude-code' && c.installed);
      const otherKinds = res.filter((c) => c.installed && c.kind !== 'claude-code' && c.kind !== 'opencode');

      const [opencode, claude, otherResults] = await Promise.all([
        opencodeInstalled ? ListOpencodeModels().catch(() => []) : Promise.resolve<string[]>([]),
        claudeInstalled ? FetchClaudeModels(false).catch(() => []) : Promise.resolve<polaris.ModelInfo[]>([]),
        Promise.allSettled(otherKinds.map((c) => ListCliModels(c.kind).then((models) => [c.kind, models] as const))),
      ]);

      setOpencodeModels(opencode);
      setClaudeModels(claude);
      const map = new Map<string, polaris.ModelInfo[]>();
      for (const r of otherResults) {
        if (r.status === 'fulfilled' && r.value[1].length > 0) {
          map.set(r.value[0], r.value[1]);
        }
      }
      setCliModels(map);
    } catch {
      setClis([]);
      setOpencodeModels([]);
      setClaudeModels([]);
      setCliModels(new Map());
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  const kinds = useMemo(() => merge(clis, claudeModels, cliModels, t), [clis, claudeModels, cliModels, t]);
  const opencode = useMemo<AgentCliInfo>(
    () =>
      mergeOne(
        OPENCODE_DESCRIPTOR,
        clis,
        opencodeModels.map((m) => ({ value: m, label: m })),
      ),
    [clis, opencodeModels],
  );

  const value = useMemo<State>(() => ({ kinds, opencode, opencodeInstalled: opencode.installed, loading, refresh }), [kinds, opencode, loading, refresh]);

  return <Ctx.Provider value={value}>{children}</Ctx.Provider>;
}

export function useAgentClis(): State {
  const ctx = useContext(Ctx);
  if (!ctx) {
    throw new Error('useAgentClis must be used inside <AgentClisProvider>');
  }

  return ctx;
}
