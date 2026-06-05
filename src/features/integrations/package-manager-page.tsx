import { AlertTriangle, ArrowUpCircle, ChevronDown, ChevronRight, Download, Package, Plug, Settings, ShieldAlert, Stethoscope, Trash2, Unplug } from 'lucide-react';
import { Fragment, type ReactNode, useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import { PageHeader } from '@/components/atoms/page-header';
import { RefreshAction } from '@/components/atoms/refresh-action';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import { ScrollArea } from '@/components/ui/scroll-area';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { Switch } from '@/components/ui/switch';
import { Table, TableBody, TableCell, TableRow } from '@/components/ui/table';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import { buildDiagnostic, isTagSpec } from '@/features/integrations/build-diagnostic';
import { ConfigureIntegrationModal } from '@/features/integrations/configure-integration-modal';
import { InstanceSelector } from '@/features/integrations/instance-selector';
import { findIntegration } from '@/features/integrations/integration-catalog';
import { toastError } from '@/lib/toast-error';
import { cn } from '@/lib/utils';
import { useCurrentProject } from '@/state/projects';
import { BrowserOpenURL } from '@/wailsjs/runtime/runtime';
import type { IntegrationConfig } from '@/types';
import type { DiagnosticInput } from './build-diagnostic';
import type { RunState, WorkspaceCommand } from './runtime-types';

export interface Dep {
  name: string;
  type: string;
  version: string;
  locations?: DepLocation[];
}

export interface DepLocation {
  workspace: string;
  manifest: string;
  type: string;
  version: string;
}

export interface OutdatedPkg {
  name: string;
  workspace?: string;
  latest: string;
  current: string;
}

export interface Vuln {
  name: string;
  url?: string;
  severity: string;
  title: string;
}

interface UnusedPkg {
  workspace: string;
  name: string;
}

export interface PkgWorkspace {
  manifest: string;
  name: string;
  isRoot?: boolean;
  dependencies?: Dep[];
}

export interface GenericScript {
  name: string;
  command?: string;
}

export interface BaseConfig {
  manifestPath?: string;
  packageManager?: string;
  runEnv?: string;
  [key: string]: unknown;
}

export interface RuntimeRunContext {
  run: RunState | null;
  isRunning: boolean;
  config: BaseConfig | null;
  instances: IntegrationConfig[];
  instanceIndex: number;
  setInstanceIndex: (i: number) => void;
  startScript: (name: string, manifestOverride?: string) => Promise<void>;
  runCommand: (args: string[], label: string, manifest?: string) => Promise<void>;
  runInWorkspaces: (cmds: WorkspaceCommand[]) => Promise<void>;
  stop: () => Promise<void>;
}

export interface PackageManagerAdapter {
  integrationId: string;
  runtime: string;
  i18nKey: string;
  icon: ReactNode;
  skipDepType: string;
  pkgTypeLabels: Record<string, string>;
  pkgTypeColors: Record<string, string>;

  useRun: () => RuntimeRunContext;

  loadPackages: (projectPath: string, manifestPath: string) => Promise<Dep[] | null>;
  loadWorkspaces: (projectPath: string, manifestPath: string) => Promise<PkgWorkspace[] | null>;
  loadScripts: (manifestPath: string) => Promise<GenericScript[] | null>;

  checkOutdated: (manifestPath: string, pm: string, env: string) => Promise<OutdatedPkg[] | null>;
  checkUnused: (projectPath: string, manifestPath: string, pm: string, env: string) => Promise<UnusedPkg[] | null>;
  checkVulnerabilities: (manifestPath: string, pm: string, env: string) => Promise<Vuln[] | null>;
  checkInstalled: (projectPath: string, manifestPath: string) => Promise<boolean>;
  setDependencyVersion: (manifests: string[], name: string, version: string) => Promise<void>;

  addArgs: (pm: string, name: string, dev: boolean) => string[];
  removeArgs: (pm: string, name: string) => string[];
  installArgs: (pm: string) => string[];
  formatSpec: (name: string, version: string) => string;
  updateAllArgs: (pm: string, specs: string[]) => string[];

  renderScripts: (props: { scripts: GenericScript[]; runCtx: RuntimeRunContext }) => ReactNode;
}

const VULN_SEVERITY_COLORS: Record<string, string> = {
  critical: 'bg-red-500/10 text-red-400 hover:bg-red-500/20',
  high: 'bg-red-500/10 text-red-400 hover:bg-red-500/20',
  moderate: 'bg-orange-500/10 text-orange-400 hover:bg-orange-500/20',
  low: 'bg-yellow-500/10 text-yellow-400 hover:bg-yellow-500/20',
  info: 'bg-muted text-muted-foreground hover:bg-muted/80',
};

export function PackageManagerPage({ adapter }: { adapter: PackageManagerAdapter }) {
  const { t } = useTranslation();
  const { project } = useCurrentProject();
  const runCtx = adapter.useRun();
  const { run, isRunning, config, instances, instanceIndex, setInstanceIndex, startScript, runCommand, runInWorkspaces, stop } = runCtx;
  const integration = findIntegration(adapter.integrationId)!;

  const manifestPath = config?.manifestPath ?? '';
  const packageManager = config?.packageManager ?? '';
  const projectPath = project?.path ?? '';

  const [scripts, setScripts] = useState<GenericScript[]>([]);
  const [packages, setPackages] = useState<Dep[]>([]);
  const [workspaces, setWorkspaces] = useState<PkgWorkspace[]>([]);
  const [tab, setTab] = useState('all');
  const [outdated, setOutdated] = useState<OutdatedPkg[]>([]);
  const [unused, setUnused] = useState<Set<string>>(new Set());
  const [vulns, setVulns] = useState<Vuln[]>([]);
  const [installed, setInstalled] = useState(true);
  const [loading, setLoading] = useState(false);
  const [collapsedGroups, setCollapsedGroups] = useState<Set<string>>(new Set());
  const [addDialogOpen, setAddDialogOpen] = useState(false);
  const [addInput, setAddInput] = useState('');
  const [addAsDev, setAddAsDev] = useState(false);
  const [addManifest, setAddManifest] = useState<string | undefined>(undefined);
  const [resolveDep, setResolveDep] = useState<Dep | null>(null);
  const [resolveTarget, setResolveTarget] = useState('');

  const refreshAll = useCallback(async () => {
    if (!manifestPath) {
      setScripts([]);
      setPackages([]);
      setWorkspaces([]);
      return;
    }
    setLoading(true);
    try {
      const [s, p, w] = await Promise.all([adapter.loadScripts(manifestPath), adapter.loadPackages(projectPath, manifestPath), adapter.loadWorkspaces(projectPath, manifestPath)]);
      setScripts([...(s ?? [])].sort((a, b) => a.name.localeCompare(b.name)));
      setPackages(p ?? []);
      setWorkspaces(w ?? []);
    } catch (err) {
      toastError({ title: t(`${adapter.i18nKey}.couldNotList`), err });
    } finally {
      setLoading(false);
    }
  }, [manifestPath, projectPath, t]);

  const checkForUpdates = useCallback(async () => {
    if (!manifestPath) {
      return;
    }
    try {
      const result = await adapter.checkOutdated(manifestPath, packageManager, config?.runEnv ?? '');
      setOutdated(result ?? []);
    } catch (err) {
      toastError({ title: t(`${adapter.i18nKey}.couldNotCheckUpdates`), err });
    }
  }, [manifestPath, packageManager, config?.runEnv, t]);

  const checkForUnused = useCallback(async () => {
    if (!manifestPath) {
      return;
    }
    try {
      const result = await adapter.checkUnused(projectPath, manifestPath, packageManager, config?.runEnv ?? '');
      setUnused(new Set((result ?? []).map((u) => `${u.workspace}::${u.name}`)));
    } catch (err) {
      toastError({ title: t(`${adapter.i18nKey}.couldNotCheckUnused`), err });
    }
  }, [projectPath, manifestPath, packageManager, config?.runEnv, t]);

  const checkForVulns = useCallback(async () => {
    if (!manifestPath) {
      return;
    }
    try {
      const result = await adapter.checkVulnerabilities(manifestPath, packageManager, config?.runEnv ?? '');
      setVulns(result ?? []);
    } catch (err) {
      toastError({ title: t(`${adapter.i18nKey}.couldNotCheckVulns`), err });
    }
  }, [manifestPath, packageManager, config?.runEnv, t]);

  const checkInstalled = useCallback(async () => {
    if (!manifestPath) {
      return;
    }
    try {
      setInstalled(await adapter.checkInstalled(projectPath, manifestPath));
    } catch {
      setInstalled(true);
    }
  }, [projectPath, manifestPath]);

  const refreshEverything = useCallback(async () => {
    await Promise.all([refreshAll(), checkForUpdates(), checkForUnused(), checkForVulns(), checkInstalled()]);
  }, [refreshAll, checkForUpdates, checkForUnused, checkForVulns, checkInstalled]);

  useEffect(() => {
    void refreshEverything();
  }, [refreshEverything]);

  const wasRunningRef = useRef(false);
  useEffect(() => {
    if (wasRunningRef.current && !isRunning) {
      void refreshEverything();
    }
    wasRunningRef.current = isRunning;
  }, [isRunning, refreshEverything]);

  const isMonorepo = workspaces.length > 1;
  const activeWorkspace = useMemo(() => (tab === 'all' ? undefined : workspaces.find((w) => w.manifest === tab)), [tab, workspaces]);

  const groupByType = (deps: Dep[]) => {
    const groups: Record<string, Dep[]> = {};
    for (const pkg of deps) {
      if (!groups[pkg.type]) {
        groups[pkg.type] = [];
      }
      groups[pkg.type].push(pkg);
    }
    return Object.entries(groups).sort(([a], [b]) => {
      const order = ['dependency', 'devDependency', adapter.skipDepType];
      return order.indexOf(a) - order.indexOf(b);
    });
  };

  const outdatedByName = useMemo(() => {
    const map = new Map<string, OutdatedPkg>();
    for (const o of outdated) {
      map.set(o.name, o);
    }
    return map;
  }, [outdated]);

  const outdatedByWorkspace = useMemo(() => {
    const map = new Map<string, OutdatedPkg>();
    for (const o of outdated) {
      if (o.workspace) {
        map.set(`${o.workspace}::${o.name}`, o);
      }
    }
    return map;
  }, [outdated]);

  const vulnsByName = useMemo(() => {
    const map = new Map<string, Vuln[]>();
    for (const v of vulns) {
      const list = map.get(v.name) ?? [];
      list.push(v);
      map.set(v.name, list);
    }
    return map;
  }, [vulns]);

  const depUnused = useCallback(
    (dep: Dep, workspace?: string): boolean => {
      if (workspace) {
        return unused.has(`${workspace}::${dep.name}`);
      }
      const locations = (dep.locations ?? []).filter((l) => l.type !== adapter.skipDepType);
      return locations.length > 0 && locations.every((l) => unused.has(`${l.workspace}::${dep.name}`));
    },
    [unused],
  );

  const updateInfo = useCallback(
    (name: string, workspace?: string): OutdatedPkg | undefined => {
      if (isMonorepo) {
        return workspace ? outdatedByWorkspace.get(`${workspace}::${name}`) : undefined;
      }
      return outdatedByName.get(name);
    },
    [isMonorepo, outdatedByWorkspace, outdatedByName],
  );

  const outdatedCount = useMemo(() => {
    if (activeWorkspace) {
      return (activeWorkspace.dependencies ?? []).filter((d) => d.type !== adapter.skipDepType && !depUnused(d, activeWorkspace.name) && updateInfo(d.name, activeWorkspace.name)).length;
    }
    return packages.filter((d) => !depUnused(d) && (d.locations ?? []).some((l) => l.type !== adapter.skipDepType && updateInfo(d.name, l.workspace))).length;
  }, [activeWorkspace, packages, depUnused, updateInfo]);

  const toggleGroup = (type: string) => {
    setCollapsedGroups((prev) => {
      const next = new Set(prev);
      if (next.has(type)) {
        next.delete(type);
      } else {
        next.add(type);
      }
      return next;
    });
  };

  const actionManifests = (dep: Dep, fallback: string): string[] => {
    const manifests = [...new Set((dep.locations ?? []).filter((l) => l.type !== adapter.skipDepType).map((l) => l.manifest))];
    return manifests.length > 0 ? manifests : [fallback];
  };

  const conflictVersions = (dep: Dep): string[] | null => {
    const locs = dep.locations ?? [];
    if (locs.length < 2) {
      return null;
    }
    const versions = [...new Set(locs.map((l) => l.version))];
    return versions.length > 1 ? versions : null;
  };

  const installAll = () => runCommand(adapter.installArgs(packageManager), t(`${adapter.i18nKey}.installAll`));

  const copyDiagnostic = async () => {
    try {
      const report = buildDiagnostic({ runtime: adapter.runtime, projectName: project?.name ?? '', manifestPath, packageManager, installed, packages, outdated, unusedKeys: [...unused], vulns, workspaces, typeLabels: adapter.pkgTypeLabels } as unknown as DiagnosticInput);
      await navigator.clipboard.writeText(report);
      toast.success(t(`${adapter.i18nKey}.diagnosticCopied`));
    } catch (err) {
      toastError({ title: t(`${adapter.i18nKey}.couldNotCopyDiagnostic`), err });
    }
  };

  const openAdd = (manifest?: string) => {
    setAddManifest(manifest);
    setAddInput('');
    setAddAsDev(false);
    setAddDialogOpen(true);
  };

  const addPackage = () => {
    const name = addInput.trim();
    if (!name) {
      return;
    }
    void runCommand(adapter.addArgs(packageManager, name, addAsDev), `add ${name}`, addManifest);
    setAddInput('');
    setAddAsDev(false);
    setAddDialogOpen(false);
  };

  const removeDep = (name: string, manifests: string[]) => {
    void runInWorkspaces(manifests.map((manifest) => ({ args: adapter.removeArgs(packageManager, name), label: `remove ${name}`, manifest })));
  };

  const updateDep = (name: string, latest: string, manifests: string[]) => {
    const spec = adapter.formatSpec(name, latest);
    void runInWorkspaces(manifests.map((manifest) => ({ args: adapter.addArgs(packageManager, spec, false), label: `update ${name}`, manifest })));
  };

  const updateAll = () => {
    const byManifest = new Map<string, string[]>();
    const queue = (name: string, latest: string, manifest: string) => {
      const specs = byManifest.get(manifest) ?? [];
      specs.push(adapter.formatSpec(name, latest));
      byManifest.set(manifest, specs);
    };
    if (activeWorkspace) {
      for (const dep of activeWorkspace.dependencies ?? []) {
        if (dep.type === adapter.skipDepType || depUnused(dep, activeWorkspace.name)) {
          continue;
        }
        const hit = updateInfo(dep.name, activeWorkspace.name);
        if (hit) {
          queue(dep.name, hit.latest, activeWorkspace.manifest);
        }
      }
    } else {
      for (const dep of packages) {
        for (const loc of dep.locations ?? []) {
          if (loc.type === adapter.skipDepType || unused.has(`${loc.workspace}::${dep.name}`)) {
            continue;
          }
          const hit = updateInfo(dep.name, loc.workspace);
          if (hit) {
            queue(dep.name, hit.latest, loc.manifest);
          }
        }
      }
    }
    const commands = [...byManifest].map(([manifest, specs]) => ({ args: adapter.updateAllArgs(packageManager, specs), label: t(`${adapter.i18nKey}.updateAll`), manifest }));
    if (commands.length > 0) {
      void runInWorkspaces(commands);
    }
  };

  const openResolve = (dep: Dep) => {
    const versions = conflictVersions(dep) ?? [];
    setResolveTarget(outdatedByName.get(dep.name)?.latest ?? versions[0] ?? '');
    setResolveDep(dep);
  };

  const applyResolve = async () => {
    if (!resolveDep || !resolveTarget) {
      return;
    }
    const manifests = [...new Set((resolveDep.locations ?? []).map((l) => l.manifest))];
    const name = resolveDep.name;
    setResolveDep(null);
    try {
      await adapter.setDependencyVersion(manifests, name, resolveTarget);
    } catch (err) {
      toastError({ title: t(`${adapter.i18nKey}.couldNotRunCommand`), err });
      return;
    }
    void runCommand(adapter.installArgs(packageManager), `align ${name}`);
  };

  const resolveOptions = useMemo(() => {
    if (!resolveDep) {
      return [];
    }
    const declared = [...new Set((resolveDep.locations ?? []).map((l) => l.version))];
    const latest = outdatedByName.get(resolveDep.name)?.latest;
    return latest && !declared.includes(latest) ? [latest, ...declared] : declared;
  }, [resolveDep, outdatedByName]);

  const renderRow = (dep: Dep, opts: { update?: OutdatedPkg; manifests: string[]; rowKey: string; conflict?: string[] | null; isUnused: boolean; vulns?: Vuln[] }) => {
    const { update, manifests, rowKey, conflict, isUnused, vulns: rowVulns } = opts;
    const worstVuln = rowVulns && rowVulns.length > 0 ? rowVulns[0] : undefined;
    return (
      <TableRow key={rowKey}>
        <TableCell className="w-full py-1 pl-8 font-mono text-xs">{dep.name}</TableCell>
        <TableCell className="whitespace-nowrap py-1 font-mono text-xs text-muted-foreground">
          {conflict ? (
            <Tooltip>
              <TooltipTrigger asChild>
                <button type="button" disabled={isRunning} onClick={() => openResolve(dep)} className="inline-flex cursor-pointer items-center gap-1 text-amber-400 transition-colors hover:text-amber-300 disabled:pointer-events-none disabled:opacity-50">
                  <AlertTriangle className="size-3" />
                  {conflict.length}
                </button>
              </TooltipTrigger>
              <TooltipContent side="top" className="font-mono text-xs">
                {t(`${adapter.i18nKey}.versionMismatch`, { versions: conflict.join(', ') })}
              </TooltipContent>
            </Tooltip>
          ) : update && isTagSpec(dep.version) ? (
            <Tooltip>
              <TooltipTrigger asChild>
                <span className="cursor-help underline decoration-dotted underline-offset-2">{update.current}</span>
              </TooltipTrigger>
              <TooltipContent side="top" className="font-mono text-xs">
                {t(`${adapter.i18nKey}.declaredSpec`, { spec: dep.version })}
              </TooltipContent>
            </Tooltip>
          ) : (
            dep.version
          )}
        </TableCell>
        <TableCell className="whitespace-nowrap py-1">
          <span className="flex items-center justify-start gap-1.5">
            {worstVuln && (
              <Tooltip>
                <TooltipTrigger asChild>
                  <button type="button" onClick={() => worstVuln.url && BrowserOpenURL(worstVuln.url)} className={cn('inline-flex cursor-pointer items-center gap-1 rounded px-1.5 py-0.5 font-mono text-[10px] font-medium transition-colors', VULN_SEVERITY_COLORS[worstVuln.severity] ?? VULN_SEVERITY_COLORS.info)}>
                    <ShieldAlert className="size-2.5" />
                    {rowVulns?.length}
                  </button>
                </TooltipTrigger>
                <TooltipContent side="top" className="max-w-xs">
                  <span className="flex flex-col gap-0.5">
                    {(rowVulns ?? []).slice(0, 8).map((v) => (
                      <span key={`${v.url}|${v.title}`}>
                        {v.severity ? `${v.severity}: ` : ''}
                        {v.title}
                      </span>
                    ))}
                    {(rowVulns?.length ?? 0) > 8 && <span>+{(rowVulns?.length ?? 0) - 8}…</span>}
                  </span>
                </TooltipContent>
              </Tooltip>
            )}
            {isUnused ? (
              <Tooltip>
                <TooltipTrigger asChild>
                  <button
                    type="button"
                    disabled={isRunning}
                    onClick={() => removeDep(dep.name, manifests)}
                    className="inline-flex cursor-pointer items-center gap-1 rounded bg-red-500/10 px-1.5 py-0.5 font-mono text-[10px] font-medium text-red-400 transition-colors hover:bg-red-500/20 disabled:pointer-events-none disabled:opacity-50"
                  >
                    <Unplug className="size-2.5" />
                    {t(`${adapter.i18nKey}.unused`)}
                  </button>
                </TooltipTrigger>
                <TooltipContent side="top">{t(`${adapter.i18nKey}.unusedTooltip`)}</TooltipContent>
              </Tooltip>
            ) : update ? (
              <Tooltip>
                <TooltipTrigger asChild>
                  <button
                    type="button"
                    disabled={isRunning}
                    onClick={() => updateDep(dep.name, update.latest, manifests)}
                    className="inline-flex cursor-pointer items-center gap-1 rounded bg-amber-500/10 px-1.5 py-0.5 font-mono text-[10px] font-medium text-amber-400 transition-colors hover:bg-amber-500/20 disabled:pointer-events-none disabled:opacity-50"
                  >
                    <ArrowUpCircle className="size-2.5" />
                    {update.latest}
                  </button>
                </TooltipTrigger>
                <TooltipContent side="top">{t(`${adapter.i18nKey}.updateTo`, { version: update.latest })}</TooltipContent>
              </Tooltip>
            ) : null}
          </span>
        </TableCell>
        <TableCell className="w-px py-1 pr-2 text-right">
          <Button size="icon" variant="ghost" className="size-5 text-muted-foreground hover:text-destructive" disabled={isRunning} onClick={() => removeDep(dep.name, manifests)}>
            <Trash2 className="size-2.5" />
          </Button>
        </TableCell>
      </TableRow>
    );
  };

  if (!project) {
    return (
      <div className="flex h-full items-center justify-center p-4">
        <p className="text-sm text-muted-foreground">{t(`${adapter.i18nKey}.selectProject`)}</p>
      </div>
    );
  }

  if (!manifestPath) {
    return (
      <div className="flex h-full flex-col gap-6 p-4">
        <header className="flex flex-col gap-1">
          <h1 className="text-2xl font-semibold tracking-tight">{t(`${adapter.i18nKey}.title`)}</h1>
          <p className="text-sm text-muted-foreground">{t(`${adapter.i18nKey}.noManifest`, { project: project.name })}</p>
        </header>
        <Card className="border-dashed">
          <CardHeader className="items-center text-center">
            <div className="mb-3 flex size-12 items-center justify-center rounded-full bg-muted">
              <Plug className="size-5 text-muted-foreground" />
            </div>
            <CardTitle className="text-base">{t(`${adapter.i18nKey}.connectTitle`)}</CardTitle>
            <CardDescription>{t(`${adapter.i18nKey}.connectDesc`)}</CardDescription>
          </CardHeader>
          <CardContent className="flex justify-center">
            <ConfigureIntegrationModal projectId={project.id} integrationId={adapter.integrationId}>
              <Button>{t(`${adapter.i18nKey}.configureCta`)}</Button>
            </ConfigureIntegrationModal>
          </CardContent>
        </Card>
      </div>
    );
  }

  return (
    <ScrollArea className="h-full">
      <div className="flex flex-col gap-6 p-4">
        <PageHeader
          icon={adapter.icon}
          title={t(`${adapter.i18nKey}.title`)}
          badges={<Badge variant="secondary">{packageManager}</Badge>}
          subtitle={<span title={manifestPath}>{manifestPath}</span>}
          actions={
            <div className="flex items-center gap-2">
              <InstanceSelector integration={integration} instances={instances} selectedIndex={instanceIndex} onSelect={setInstanceIndex} projectPath={project.path ?? ''} />
              {isMonorepo && (
                <Select value={tab} onValueChange={setTab}>
                  <SelectTrigger className="h-8 w-auto min-w-[160px]">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="all">{t(`${adapter.i18nKey}.viewAll`)}</SelectItem>
                    {workspaces.map((ws) => (
                      <SelectItem key={ws.manifest} value={ws.manifest}>
                        {ws.isRoot ? t(`${adapter.i18nKey}.rootWorkspace`) : ws.name}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              )}
              <Button variant="outline" size="sm" disabled={isRunning || installed} onClick={installAll}>
                <Download className="size-3.5" />
                {t(`${adapter.i18nKey}.installAll`)}
              </Button>
              <RefreshAction onRefresh={() => refreshEverything()} loading={loading} />
              <ConfigureIntegrationModal projectId={project.id} integrationId={adapter.integrationId} instanceIndex={instanceIndex}>
                <Button variant="outline" size="sm">
                  <Settings className="size-3.5" />
                  {t('common.configure')}
                </Button>
              </ConfigureIntegrationModal>
            </div>
          }
        />

        {adapter.renderScripts({ scripts, runCtx })}

        <section>
          <div className="mb-3 flex flex-col gap-2">
            <h2 className="text-sm font-medium text-muted-foreground">
              {t(`${adapter.i18nKey}.tabPackages`)} ({packages.length})
            </h2>
            <div className="flex items-center gap-3">
              <div className="flex-1" />
              <Button size="sm" variant="outline" className="h-7 gap-1.5 text-xs" onClick={() => void copyDiagnostic()}>
                <Stethoscope className="size-3" />
                {t(`${adapter.i18nKey}.copyDiagnostic`)}
              </Button>
              {outdatedCount > 0 && (
                <Button size="sm" variant="outline" disabled={isRunning} onClick={updateAll} className="h-7 gap-1.5 text-xs text-amber-400 hover:text-amber-300">
                  <ArrowUpCircle className="size-3" />
                  {t(`${adapter.i18nKey}.updateAll`)} ({outdatedCount})
                </Button>
              )}
              <Button size="sm" variant="outline" disabled={isRunning} className="h-7 gap-1.5 text-xs" onClick={() => openAdd(activeWorkspace?.manifest)}>
                <Package className="size-3" />
                {t(`${adapter.i18nKey}.addPackage`)}
              </Button>
            </div>
          </div>
          {packages.length === 0 ? (
            <p className="text-sm text-muted-foreground">{t(`${adapter.i18nKey}.noPackages`)}</p>
          ) : (
            <Table>
              <TableBody>
                {(activeWorkspace ? groupByType(activeWorkspace.dependencies ?? []) : groupByType(packages)).map(([type, pkgs]) => {
                  const groupKey = `${tab}:${type}`;
                  const collapsed = collapsedGroups.has(groupKey);
                  return (
                    <Fragment key={groupKey}>
                      <TableRow className="hover:bg-transparent">
                        <TableCell colSpan={4} className="py-1">
                          <button type="button" onClick={() => toggleGroup(groupKey)} className="flex items-center gap-1.5 rounded text-left text-xs font-medium text-muted-foreground hover:text-foreground">
                            {collapsed ? <ChevronRight className="size-3.5" /> : <ChevronDown className="size-3.5" />}
                            {adapter.pkgTypeLabels[type] ?? type}
                            <Badge variant="secondary" className={cn('ml-1 h-4 px-1.5 text-[10px]', adapter.pkgTypeColors[type])}>
                              {pkgs.length}
                            </Badge>
                          </button>
                        </TableCell>
                      </TableRow>
                      {!collapsed &&
                        pkgs.map((pkg) =>
                          activeWorkspace
                            ? renderRow(pkg, { update: updateInfo(pkg.name, activeWorkspace.name), manifests: [activeWorkspace.manifest], rowKey: `${tab}:${pkg.name}`, isUnused: depUnused(pkg, activeWorkspace.name), vulns: vulnsByName.get(pkg.name) })
                            : renderRow(pkg, { update: outdatedByName.get(pkg.name), manifests: actionManifests(pkg, manifestPath), rowKey: `all:${pkg.name}`, conflict: conflictVersions(pkg), isUnused: depUnused(pkg), vulns: vulnsByName.get(pkg.name) }),
                        )}
                    </Fragment>
                  );
                })}
              </TableBody>
            </Table>
          )}
        </section>

        <Dialog open={addDialogOpen} onOpenChange={setAddDialogOpen}>
          <DialogContent className="sm:max-w-md">
            <DialogHeader>
              <DialogTitle>{t(`${adapter.i18nKey}.addPackage`)}</DialogTitle>
              <DialogDescription>{t(`${adapter.i18nKey}.addPackageDesc`)}</DialogDescription>
            </DialogHeader>
            <div className="flex flex-col gap-4">
              <Input value={addInput} onChange={(e) => setAddInput(e.target.value)} onKeyDown={(e) => e.key === 'Enter' && addPackage()} placeholder={t(`${adapter.i18nKey}.addPackagePlaceholder`)} autoFocus />
              <div className="flex items-center gap-2 text-sm">
                <Switch id="add-as-dev" checked={addAsDev} onCheckedChange={setAddAsDev} />
                <label htmlFor="add-as-dev">{t(`${adapter.i18nKey}.addAsDev`)}</label>
              </div>
            </div>
            <DialogFooter>
              <Button disabled={!addInput.trim()} onClick={addPackage}>
                {t(`${adapter.i18nKey}.installAll`)}
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>

        <Dialog open={resolveDep !== null} onOpenChange={(open) => !open && setResolveDep(null)}>
          <DialogContent className="sm:max-w-md">
            <DialogHeader>
              <DialogTitle>{t(`${adapter.i18nKey}.resolveTitle`, { name: resolveDep?.name })}</DialogTitle>
              <DialogDescription>{t(`${adapter.i18nKey}.resolveDesc`)}</DialogDescription>
            </DialogHeader>
            {resolveDep && (
              <div className="flex flex-col gap-4">
                <div className="grid grid-cols-[1fr_auto] gap-x-4 gap-y-1 text-xs">
                  {(resolveDep.locations ?? []).map((loc) => (
                    <Fragment key={`${loc.manifest}:${loc.type}`}>
                      <span className="truncate font-mono text-muted-foreground">{loc.workspace}</span>
                      <span className="text-right font-mono">{loc.version}</span>
                    </Fragment>
                  ))}
                </div>
                <div className="flex items-center gap-2">
                  <span className="whitespace-nowrap text-sm text-muted-foreground">{t(`${adapter.i18nKey}.resolveTarget`)}</span>
                  <Select value={resolveTarget} onValueChange={setResolveTarget}>
                    <SelectTrigger className="h-8 flex-1 font-mono text-xs">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {resolveOptions.map((v) => (
                        <SelectItem key={v} value={v} className="font-mono text-xs">
                          {v}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>
              </div>
            )}
            <DialogFooter>
              <Button disabled={!resolveTarget || isRunning} onClick={applyResolve}>
                {t(`${adapter.i18nKey}.resolveApply`)}
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      </div>
    </ScrollArea>
  );
}
