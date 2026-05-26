import { createContext, type ReactNode, useCallback, useContext, useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { getInstances } from '@/features/integrations/project-integrations';
import { toastError } from '@/lib/toast-error';
import { useCurrentProject } from '@/state/projects';
import { RunNodeCommand, StartNodeScript, StopNodeScript } from '@/wailsjs/go/main/App';
import { EventsOff, EventsOn } from '@/wailsjs/runtime/runtime';
import type { NodejsConfig, RunExitEvent, RunLineEvent } from './types';

export interface RunLine extends RunLineEvent {
  seq: number;
}

export interface RunState {
  runId: string;
  scriptName: string;
  lines: RunLine[];
  exited?: RunExitEvent;
}

interface NodejsRunContextValue {
  run: RunState | null;
  isRunning: boolean;
  config: NodejsConfig | null;
  instances: NodejsConfig[];
  instanceIndex: number;
  setInstanceIndex: (index: number) => void;
  terminalOpen: boolean;
  setTerminalOpen: (open: boolean) => void;
  startScript: (scriptName: string) => Promise<void>;
  runCommand: (args: string[], label: string, manifestPath?: string) => Promise<void>;
  runInWorkspaces: (commands: WorkspaceCommand[]) => Promise<void>;
  stop: () => Promise<void>;
  clear: () => void;
}

export interface WorkspaceCommand {
  args: string[];
  label: string;
  manifest: string;
}

const NodejsRunContext = createContext<NodejsRunContextValue | null>(null);

export function NodejsRunProvider({ children }: { children: ReactNode }) {
  const { t } = useTranslation();
  const { project } = useCurrentProject();
  const [run, setRun] = useState<RunState | null>(null);
  const [terminalOpen, setTerminalOpen] = useState(false);
  const openTerminalRef = useRef(setTerminalOpen);
  openTerminalRef.current = setTerminalOpen;
  const exitResolvers = useRef(new Map<string, () => void>());
  const [instanceIndex, setInstanceIndex] = useState(0);

  const instances = (project ? getInstances(project, 'nodejs') : []) as NodejsConfig[];
  const config = instances[instanceIndex] ?? null;

  useEffect(() => {
    const onLine = (data: RunLineEvent) => {
      setRun((prev) => {
        if (!prev || prev.runId !== data.runId) {
          return prev;
        }
        const seq = (prev.lines[prev.lines.length - 1]?.seq ?? 0) + 1;
        return { ...prev, lines: [...prev.lines, { ...data, seq }] };
      });
    };
    const onExit = (data: RunExitEvent) => {
      setRun((prev) => (prev && prev.runId === data.runId ? { ...prev, exited: data } : prev));
      const resolve = exitResolvers.current.get(data.runId);
      if (resolve) {
        exitResolvers.current.delete(data.runId);
        resolve();
      }
    };
    EventsOn('nodejs:run:line', onLine);
    EventsOn('nodejs:run:exit', onExit);
    return () => {
      EventsOff('nodejs:run:line');
      EventsOff('nodejs:run:exit');
    };
  }, []);

  const isRunning = run !== null && !run.exited;

  const startScript = useCallback(
    async (scriptName: string) => {
      if (!config?.manifestPath || isRunning) {
        return;
      }
      try {
        const runId = await StartNodeScript(config.manifestPath, config.packageManager ?? 'npm', scriptName, config.runEnv ?? '');
        setRun({ runId, scriptName, lines: [] });
        openTerminalRef.current(true);
      } catch (err) {
        toastError({ title: t('integrations.nodejs.couldNotStart'), err });
      }
    },
    [config, isRunning, t],
  );

  const launch = useCallback(
    async (args: string[], label: string, manifestPath?: string) => {
      const mp = manifestPath ?? config?.manifestPath;
      if (!mp) {
        return undefined;
      }
      const runId = await RunNodeCommand(mp, config?.packageManager ?? 'npm', config?.runEnv ?? '', args);
      setRun({ runId, scriptName: label, lines: [] });
      openTerminalRef.current(true);
      return runId;
    },
    [config],
  );

  const runCommand = useCallback(
    async (args: string[], label: string, manifestPath?: string) => {
      if (!config?.manifestPath || isRunning) {
        return;
      }
      try {
        await launch(args, label, manifestPath);
      } catch (err) {
        toastError({ title: t('integrations.nodejs.couldNotRunCommand'), err });
      }
    },
    [config, isRunning, launch, t],
  );

  const runInWorkspaces = useCallback(
    async (commands: WorkspaceCommand[]) => {
      if (!config?.manifestPath || isRunning) {
        return;
      }
      for (const command of commands) {
        let runId: string | undefined;
        try {
          runId = await launch(command.args, command.label, command.manifest);
        } catch (err) {
          toastError({ title: t('integrations.nodejs.couldNotRunCommand'), err });
          return;
        }
        if (!runId) {
          return;
        }
        await new Promise<void>((resolve) => {
          exitResolvers.current.set(runId, resolve);
        });
      }
    },
    [config, isRunning, launch, t],
  );

  const stop = useCallback(async () => {
    if (!run || run.exited) {
      return;
    }
    try {
      await StopNodeScript(run.runId);
    } catch (err) {
      toastError({ title: t('integrations.nodejs.couldNotStop'), err });
    }
  }, [run, t]);

  const clear = useCallback(() => {
    if (!isRunning) {
      setRun(null);
      setTerminalOpen(false);
    }
  }, [isRunning]);

  return <NodejsRunContext.Provider value={{ run, isRunning, config, instances, instanceIndex, setInstanceIndex, terminalOpen, setTerminalOpen, startScript, runCommand, runInWorkspaces, stop, clear }}>{children}</NodejsRunContext.Provider>;
}

export function useNodejsRun() {
  const ctx = useContext(NodejsRunContext);
  if (!ctx) {
    throw new Error('useNodejsRun must be used within NodejsRunProvider');
  }
  return ctx;
}
