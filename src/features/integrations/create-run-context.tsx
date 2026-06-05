import { createContext, type ReactNode, useCallback, useContext, useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { getInstances } from '@/features/integrations/project-integrations';
import { toastError } from '@/lib/toast-error';
import { useCurrentProject } from '@/state/projects';
import { EventsOff, EventsOn } from '@/wailsjs/runtime/runtime';
import type { RunExitEvent, RunLineEvent, RunState, WorkspaceCommand } from './runtime-types';

interface BaseConfig {
  manifestPath?: string;
  packageManager?: string;
  runEnv?: string;
}

interface RunContextOptions {
  kind: string;
  eventPrefix: string;
  defaultPm: string;
  i18nPrefix: string;
  fns: {
    start: (mp: string, pm: string, name: string, env: string) => Promise<string>;
    run: (mp: string, pm: string, env: string, args: string[]) => Promise<string>;
    stop: (runId: string) => Promise<void>;
  };
}

interface RunContextValue<TConfig extends BaseConfig> {
  run: RunState | null;
  isRunning: boolean;
  config: TConfig | null;
  instances: TConfig[];
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

export function createRunContext<TConfig extends BaseConfig>(opts: RunContextOptions) {
  const { kind, eventPrefix, defaultPm, i18nPrefix, fns } = opts;

  const Context = createContext<RunContextValue<TConfig> | null>(null);

  function Provider({ children }: { children: ReactNode }) {
    const { t } = useTranslation();
    const { project } = useCurrentProject();
    const [runs, setRuns] = useState<Map<string, RunState>>(new Map());
    const [openTerminals, setOpenTerminals] = useState<Map<string, boolean>>(new Map());
    const exitResolvers = useRef(new Map<string, () => void>());
    const [instanceIndex, setInstanceIndex] = useState(0);

    const projectId = project?.id ?? '';
    const projectIdRef = useRef(projectId);
    projectIdRef.current = projectId;

    const instances = (project ? getInstances(project, kind) : []) as TConfig[];
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
      EventsOn(`${eventPrefix}:run:line`, onLine);
      EventsOn(`${eventPrefix}:run:exit`, onExit);
      return () => {
        EventsOff(`${eventPrefix}:run:line`);
        EventsOff(`${eventPrefix}:run:exit`);
      };
    }, []);

    const isRunning = run !== null && !run.exited;

    const launch = useCallback(
      async (args: string[], label: string, manifestPath?: string) => {
        const mp = manifestPath ?? config?.manifestPath;
        if (!mp) {
          return undefined;
        }
        const runId = await fns.run(mp, config?.packageManager ?? defaultPm, config?.runEnv ?? '', args);
        setProjectRun({ runId, scriptName: label, manifest: mp, lines: [] });
        setTerminalOpen(true);
        return runId;
      },
      [config, setProjectRun, setTerminalOpen],
    );

    const startScript = useCallback(
      async (scriptName: string, manifestOverride?: string) => {
        const mp = manifestOverride ?? config?.manifestPath;
        if (!mp || isRunning) {
          return;
        }
        try {
          const runId = await fns.start(mp, config?.packageManager ?? defaultPm, scriptName, config?.runEnv ?? '');
          setProjectRun({ runId, scriptName, manifest: mp, lines: [] });
          setTerminalOpen(true);
        } catch (err) {
          toastError({ title: t(`${i18nPrefix}.couldNotStart`), err });
        }
      },
      [config, isRunning, t, setProjectRun, setTerminalOpen],
    );

    const runCommand = useCallback(
      async (args: string[], label: string, manifestPath?: string) => {
        if (!(manifestPath ?? config?.manifestPath) || isRunning) {
          return;
        }
        try {
          await launch(args, label, manifestPath);
        } catch (err) {
          toastError({ title: t(`${i18nPrefix}.couldNotRunCommand`), err });
        }
      },
      [config, isRunning, launch, t],
    );

    const runInWorkspaces = useCallback(
      async (commands: WorkspaceCommand[]) => {
        const hasManifest = commands.some((c) => Boolean(c.manifest)) || Boolean(config?.manifestPath);
        if (!hasManifest || isRunning) {
          return;
        }
        for (const command of commands) {
          let runId: string | undefined;
          try {
            runId = await launch(command.args, command.label, command.manifest);
          } catch (err) {
            toastError({ title: t(`${i18nPrefix}.couldNotRunCommand`), err });
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
        await fns.stop(run.runId);
      } catch (err) {
        toastError({ title: t(`${i18nPrefix}.couldNotStop`), err });
      }
    }, [run, t]);

    const restart = useCallback(async () => {
      if (!run) {
        return;
      }
      const { runId, scriptName, manifest } = run;
      const mp = manifest ?? config?.manifestPath;
      if (!mp) {
        return;
      }
      if (!run.exited) {
        const exitPromise = new Promise<void>((resolve) => {
          exitResolvers.current.set(runId, resolve);
        });
        try {
          await fns.stop(runId);
        } catch (err) {
          toastError({ title: t(`${i18nPrefix}.couldNotStop`), err });
          return;
        }
        await exitPromise;
      }
      try {
        const newRunId = await fns.start(mp, config?.packageManager ?? defaultPm, scriptName, config?.runEnv ?? '');
        setProjectRun({ runId: newRunId, scriptName, manifest: mp, lines: [] });
      } catch (err) {
        toastError({ title: t(`${i18nPrefix}.couldNotStart`), err });
      }
    }, [run, config, t, setProjectRun]);

    const clear = useCallback(() => {
      if (!isRunning) {
        setProjectRun(null);
        setTerminalOpen(false);
      }
    }, [isRunning, setProjectRun, setTerminalOpen]);

    return <Context.Provider value={{ run, isRunning, config, instances, instanceIndex, setInstanceIndex, terminalOpen, setTerminalOpen, startScript, runCommand, runInWorkspaces, stop, restart, clear }}>{children}</Context.Provider>;
  }

  function useRun(): RunContextValue<TConfig> {
    const ctx = useContext(Context);
    if (!ctx) {
      throw new Error(`useRun must be used within the ${kind} Provider`);
    }
    return ctx;
  }

  return { Provider, useRun };
}
