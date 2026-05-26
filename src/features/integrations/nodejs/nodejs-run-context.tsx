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
  manifest?: string;
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
  startScript: (scriptName: string, manifestOverride?: string) => Promise<void>;
  runCommand: (args: string[], label: string, manifestPath?: string) => Promise<void>;
  runInWorkspaces: (commands: WorkspaceCommand[]) => Promise<void>;
  stop: () => Promise<void>;
  restart: () => Promise<void>;
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
  const [runs, setRuns] = useState<Map<string, RunState>>(new Map());
  const [openTerminals, setOpenTerminals] = useState<Map<string, boolean>>(new Map());
  const exitResolvers = useRef(new Map<string, () => void>());
  const [instanceIndex, setInstanceIndex] = useState(0);

  const projectId = project?.id ?? '';
  const projectIdRef = useRef(projectId);
  projectIdRef.current = projectId;

  const instances = (project ? getInstances(project, 'nodejs') : []) as NodejsConfig[];
  const config = instances[instanceIndex] ?? null;

  const run = runs.get(projectId) ?? null;
  const terminalOpen = openTerminals.get(projectId) ?? false;

  const setProjectRun = useCallback((value: RunState | null) => {
    const pid = projectIdRef.current;
    setRuns((prev) => {
      const next = new Map(prev);
      if (value === null) {
        next.delete(pid);
      } else {
        next.set(pid, value);
      }
      return next;
    });
  }, []);

  const setTerminalOpen = useCallback((open: boolean) => {
    const pid = projectIdRef.current;
    setOpenTerminals((prev) => new Map(prev).set(pid, open));
  }, []);

  useEffect(() => {
    const onLine = (data: RunLineEvent) => {
      setRuns((prev) => {
        for (const [pid, runState] of prev) {
          if (runState.runId === data.runId) {
            const seq = (runState.lines[runState.lines.length - 1]?.seq ?? 0) + 1;
            return new Map(prev).set(pid, { ...runState, lines: [...runState.lines, { ...data, seq }] });
          }
        }
        return prev;
      });
    };
    const onExit = (data: RunExitEvent) => {
      setRuns((prev) => {
        for (const [pid, runState] of prev) {
          if (runState.runId === data.runId) {
            return new Map(prev).set(pid, { ...runState, exited: data });
          }
        }
        return prev;
      });
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
    async (scriptName: string, manifestOverride?: string) => {
      const mp = manifestOverride ?? config?.manifestPath;
      if (!mp || isRunning) {
        return;
      }
      try {
        const runId = await StartNodeScript(mp, config?.packageManager ?? 'npm', scriptName, config?.runEnv ?? '');
        setProjectRun({ runId, scriptName, manifest: mp, lines: [] });
        setTerminalOpen(true);
      } catch (err) {
        toastError({ title: t('integrations.nodejs.couldNotStart'), err });
      }
    },
    [config, isRunning, t, setProjectRun, setTerminalOpen],
  );

  const launch = useCallback(
    async (args: string[], label: string, manifestPath?: string) => {
      const mp = manifestPath ?? config?.manifestPath;
      if (!mp) {
        return undefined;
      }
      const runId = await RunNodeCommand(mp, config?.packageManager ?? 'npm', config?.runEnv ?? '', args);
      setProjectRun({ runId, scriptName: label, manifest: mp, lines: [] });
      setTerminalOpen(true);
      return runId;
    },
    [config, setProjectRun, setTerminalOpen],
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

  const restart = useCallback(async () => {
    if (!run) return;
    const { runId, scriptName, manifest } = run;
    const mp = manifest ?? config?.manifestPath;
    if (!mp) return;
    if (!run.exited) {
      const exitPromise = new Promise<void>((resolve) => {
        exitResolvers.current.set(runId, resolve);
      });
      try {
        await StopNodeScript(runId);
      } catch (err) {
        toastError({ title: t('integrations.nodejs.couldNotStop'), err });
        return;
      }
      await exitPromise;
    }
    try {
      const newRunId = await StartNodeScript(mp, config?.packageManager ?? 'npm', scriptName, config?.runEnv ?? '');
      setProjectRun({ runId: newRunId, scriptName, manifest: mp, lines: [] });
    } catch (err) {
      toastError({ title: t('integrations.nodejs.couldNotStart'), err });
    }
  }, [run, config, t, setProjectRun]);

  const clear = useCallback(() => {
    if (!isRunning) {
      setProjectRun(null);
      setTerminalOpen(false);
    }
  }, [isRunning, setProjectRun, setTerminalOpen]);

  return <NodejsRunContext.Provider value={{ run, isRunning, config, instances, instanceIndex, setInstanceIndex, terminalOpen, setTerminalOpen, startScript, runCommand, runInWorkspaces, stop, restart, clear }}>{children}</NodejsRunContext.Provider>;
}

export function useNodejsRun() {
  const ctx = useContext(NodejsRunContext);
  if (!ctx) {
    throw new Error('useNodejsRun must be used within NodejsRunProvider');
  }
  return ctx;
}
