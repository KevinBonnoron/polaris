import { createContext, type ReactNode, useCallback, useContext, useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { AGENT_KINDS, type AgentKindConfig, type AgentModel, OPENCODE_DESCRIPTOR } from '@/features/agents/agent-kinds';
import { DetectAgentClis, FetchClaudeModels, ListOpencodeModels } from '@/wailsjs/go/main/App';
import type { main, polaris } from '@/wailsjs/go/models';

export type AgentCliInfo = AgentKindConfig & {
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

function mergeOne(descriptor: AgentKindConfig, clis: main.AgentCli[] | null): AgentCliInfo {
  const cli = (clis ?? []).find((c) => c.kind === descriptor.id);
  return { ...descriptor, installed: cli?.installed ?? false, binary: cli?.binary ?? descriptor.id, path: cli?.path };
}

const Ctx = createContext<State | null>(null);

type Translate = (key: string) => string;

function claudeModelsToOptions(models: polaris.ClaudeModel[], t: Translate): AgentModel[] {
  return models.map((m) => {
    const version = m.name.replace(/^Claude\s+/, '');
    const tagline = t(`agents.modelTagline.${m.family}`);
    return { value: m.value, label: t(`agents.modelFamily.${m.family}`), description: `${version} · ${tagline}` };
  });
}

function merge(clis: main.AgentCli[] | null, claudeModels: polaris.ClaudeModel[], t: Translate): AgentCliInfo[] {
  const byKind = new Map<string, main.AgentCli>();
  for (const c of clis ?? []) {
    byKind.set(c.kind, c);
  }

  const claudeOptions = claudeModels.length > 0 ? claudeModelsToOptions(claudeModels, t) : null;

  return AGENT_KINDS.map((k) => {
    const cli = byKind.get(k.id);
    return {
      ...k,
      models: k.id === 'claude-code' && claudeOptions ? claudeOptions : k.models,
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
  const [claudeModels, setClaudeModels] = useState<polaris.ClaudeModel[]>([]);
  const [loading, setLoading] = useState(true);

  const refresh = useCallback(async () => {
    setLoading(true);
    try {
      const res = await DetectAgentClis();
      setClis(res);
      if (res.some((c) => c.kind === 'opencode' && c.installed)) {
        ListOpencodeModels()
          .then(setOpencodeModels)
          .catch(() => {});
      }
      if (res.some((c) => c.kind === 'claude-code' && c.installed)) {
        FetchClaudeModels(false)
          .then(setClaudeModels)
          .catch(() => {});
      }
    } catch {
      setClis([]);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  const kinds = useMemo(() => merge(clis, claudeModels, t), [clis, claudeModels, t]);
  const opencode = useMemo<AgentCliInfo>(() => ({ ...mergeOne(OPENCODE_DESCRIPTOR, clis), models: opencodeModels.map((m) => ({ value: m, label: m })) }), [clis, opencodeModels]);

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
