import { AlertTriangle, ArrowUpCircle, ChevronDown, ChevronRight, Download, Package, Play, Plug, Settings, ShieldAlert, Square, Stethoscope, Trash2, Unplug } from 'lucide-react';
import { Fragment, useCallback, useEffect, useMemo, useRef, useState } from 'react';
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
import { CheckOutdatedPackages, CheckPackagesInstalled, CheckUnusedPackages, CheckVulnerabilities, ListNodePackages, ListNodeScripts, ListNodeWorkspaces, SetNodeDependencyVersion } from '@/wailsjs/go/main/App';
import type { nodejs } from '@/wailsjs/go/models';
import { BrowserOpenURL } from '@/wailsjs/runtime/runtime';
import { useNodejsRun } from './nodejs-run-context';

interface Script {
  name: string;
  command: string;
}

const PKG_TYPE_LABELS: Record<string, string> = {
  dependency: 'Dependencies',
  devDependency: 'Dev dependencies',
  peerDependency: 'Peer dependencies',
};

const PKG_TYPE_COLORS: Record<string, string> = {
  dependency: 'bg-emerald-500/10 text-emerald-400',
  devDependency: 'bg-blue-500/10 text-blue-400',
  peerDependency: 'bg-amber-500/10 text-amber-400',
};

const VULN_SEVERITY_COLORS: Record<string, string> = {
  critical: 'bg-red-500/10 text-red-400 hover:bg-red-500/20',
  high: 'bg-red-500/10 text-red-400 hover:bg-red-500/20',
  moderate: 'bg-orange-500/10 text-orange-400 hover:bg-orange-500/20',
  low: 'bg-yellow-500/10 text-yellow-400 hover:bg-yellow-500/20',
  info: 'bg-muted text-muted-foreground hover:bg-muted/80',
};

export function NodejsPage() {
  const { t } = useTranslation();
  const { project } = useCurrentProject();
  const { run, isRunning, config, instances, instanceIndex, setInstanceIndex, startScript, runCommand, runInWorkspaces, stop } = useNodejsRun();
  const integration = findIntegration('nodejs')!;

  const manifestPath = config?.manifestPath ?? '';
  const packageManager = config?.packageManager ?? 'npm';

  const [scripts, setScripts] = useState<Script[]>([]);
  const [packages, setPackages] = useState<nodejs.Dependency[]>([]);
  const [workspaces, setWorkspaces] = useState<nodejs.Workspace[]>([]);
  const [tab, setTab] = useState('all');
  const [outdated, setOutdated] = useState<nodejs.OutdatedPackage[]>([]);
  const [unused, setUnused] = useState<Set<string>>(new Set());
  const [vulns, setVulns] = useState<nodejs.Vulnerability[]>([]);
  const [installed, setInstalled] = useState(true);
  const [loading, setLoading] = useState(false);
  const [collapsedGroups, setCollapsedGroups] = useState<Set<string>>(new Set());
  const [addDialogOpen, setAddDialogOpen] = useState(false);
  const [addInput, setAddInput] = useState('');
  const [addAsDev, setAddAsDev] = useState(false);
  const [addManifest, setAddManifest] = useState<string | undefined>(undefined);
  const [resolveDep, setResolveDep] = useState<nodejs.Dependency | null>(null);
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
      const [s, p, w] = await Promise.all([ListNodeScripts(manifestPath), ListNodePackages(manifestPath), ListNodeWorkspaces(manifestPath)]);
      setScripts([...(s ?? [])].sort((a, b) => a.name.localeCompare(b.name)));
      setPackages(p ?? []);
      setWorkspaces(w ?? []);
    } catch (err) {
      toastError({ title: t('integrations.nodejs.couldNotList'), err });
    } finally {
      setLoading(false);
    }
  }, [manifestPath, t]);

  const checkForUpdates = useCallback(async () => {
    if (!manifestPath) {
      return;
    }
    try {
      const result = await CheckOutdatedPackages(manifestPath, packageManager, config?.runEnv ?? '');
      setOutdated(result ?? []);
    } catch (err) {
      toastError({ title: t('integrations.nodejs.couldNotCheckUpdates'), err });
    }
  }, [manifestPath, packageManager, config?.runEnv, t]);

  const checkForUnused = useCallback(async () => {
    if (!manifestPath) {
      return;
    }
    try {
      const result = await CheckUnusedPackages(manifestPath, packageManager, config?.runEnv ?? '');
      setUnused(new Set((result ?? []).map((u) => `${u.workspace}::${u.name}`)));
    } catch (err) {
      toastError({ title: t('integrations.nodejs.couldNotCheckUnused'), err });
    }
  }, [manifestPath, packageManager, config?.runEnv, t]);

  const checkForVulns = useCallback(async () => {
    if (!manifestPath) {
      return;
    }
    try {
      const result = await CheckVulnerabilities(manifestPath, packageManager, config?.runEnv ?? '');
      setVulns(result ?? []);
    } catch (err) {
      toastError({ title: t('integrations.nodejs.couldNotCheckVulns'), err });
    }
  }, [manifestPath, packageManager, config?.runEnv, t]);

  const checkInstalled = useCallback(async () => {
    if (!manifestPath) {
      return;
    }
    try {
      setInstalled(await CheckPackagesInstalled(manifestPath));
    } catch {
      setInstalled(true);
    }
  }, [manifestPath]);

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
  const addSub = packageManager === 'npm' ? 'install' : 'add';
  const removeSub = packageManager === 'npm' ? 'uninstall' : 'remove';

  const activeWorkspace = useMemo(() => (tab === 'all' ? undefined : workspaces.find((w) => w.manifest === tab)), [tab, workspaces]);

  const groupByType = (deps: nodejs.Dependency[]) => {
    const groups: Record<string, nodejs.Dependency[]> = {};
    for (const pkg of deps) {
      if (!groups[pkg.type]) {
        groups[pkg.type] = [];
      }
      groups[pkg.type].push(pkg);
    }
    return Object.entries(groups).sort(([a], [b]) => {
      const order = ['dependency', 'devDependency', 'peerDependency'];
      return order.indexOf(a) - order.indexOf(b);
    });
  };

  const outdatedByName = useMemo(() => {
    const map = new Map<string, nodejs.OutdatedPackage>();
    for (const o of outdated) {
      map.set(o.name, o);
    }
    return map;
  }, [outdated]);

  const outdatedByWorkspace = useMemo(() => {
    const map = new Map<string, nodejs.OutdatedPackage>();
    for (const o of outdated) {
      if (o.workspace) {
        map.set(`${o.workspace}::${o.name}`, o);
      }
    }
    return map;
  }, [outdated]);

  const vulnsByName = useMemo(() => {
    const map = new Map<string, nodejs.Vulnerability[]>();
    for (const v of vulns) {
      const list = map.get(v.name) ?? [];
      list.push(v);
      map.set(v.name, list);
    }
    return map;
  }, [vulns]);

  // A dependency is "unused" for a given workspace tab when depcheck flagged it
  // there; in the aggregated "All" view, only when it's unused in every (non-peer)
  // workspace that declares it.
  const depUnused = useCallback(
    (dep: nodejs.Dependency, workspace?: string): boolean => {
      if (workspace) {
        return unused.has(`${workspace}::${dep.name}`);
      }
      const locations = (dep.locations ?? []).filter((l) => l.type !== 'peerDependency');
      return locations.length > 0 && locations.every((l) => unused.has(`${l.workspace}::${dep.name}`));
    },
    [unused],
  );

  const updateInfo = useCallback(
    (name: string, workspace?: string): nodejs.OutdatedPackage | undefined => {
      // In a monorepo, outdated data is per-workspace: a package that is current
      // in this workspace must not borrow another workspace's pending update. The
      // name fallback is only for single-repo data, which carries no workspace.
      if (isMonorepo) {
        return workspace ? outdatedByWorkspace.get(`${workspace}::${name}`) : undefined;
      }
      return outdatedByName.get(name);
    },
    [isMonorepo, outdatedByWorkspace, outdatedByName],
  );

  // Outdated count for the active scope: the current workspace tab, or every
  // updatable package across the repo on the "All" tab. Mirrors what updateAll
  // would touch (skips peers and unused packages).
  const outdatedCount = useMemo(() => {
    if (activeWorkspace) {
      return (activeWorkspace.dependencies ?? []).filter((d) => d.type !== 'peerDependency' && !depUnused(d, activeWorkspace.name) && updateInfo(d.name, activeWorkspace.name)).length;
    }
    return packages.filter((d) => !depUnused(d) && (d.locations ?? []).some((l) => l.type !== 'peerDependency' && updateInfo(d.name, l.workspace))).length;
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

  // The workspaces a mutation should touch: every non-peer location, or the given
  // fallback manifest for a single-package repo / a per-workspace row.
  const actionManifests = (dep: nodejs.Dependency, fallback: string): string[] => {
    const manifests = [...new Set((dep.locations ?? []).filter((l) => l.type !== 'peerDependency').map((l) => l.manifest))];
    return manifests.length > 0 ? manifests : [fallback];
  };

  const conflictVersions = (dep: nodejs.Dependency): string[] | null => {
    const locations = dep.locations ?? [];
    if (locations.length < 2) {
      return null;
    }
    const versions = [...new Set(locations.map((l) => l.version))];
    return versions.length > 1 ? versions : null;
  };

  const installAll = () => runCommand(['install'], t('integrations.nodejs.installAll'));

  const copyDiagnostic = async () => {
    try {
      const report = buildDiagnostic({
        runtime: 'Node.js',
        projectName: project?.name ?? '',
        manifestPath,
        packageManager,
        installed,
        packages,
        outdated,
        unusedKeys: [...unused],
        vulns,
        workspaces,
        typeLabels: PKG_TYPE_LABELS,
      });
      await navigator.clipboard.writeText(report);
      toast.success(t('integrations.nodejs.diagnosticCopied'));
    } catch (err) {
      toastError({ title: t('integrations.nodejs.couldNotCopyDiagnostic'), err });
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
    const args = addAsDev ? [addSub, '-D', name] : [addSub, name];
    void runCommand(args, `${addSub} ${name}`, addManifest);
    setAddInput('');
    setAddAsDev(false);
    setAddDialogOpen(false);
  };

  const removeDep = (name: string, manifests: string[]) => {
    void runInWorkspaces(manifests.map((manifest) => ({ args: [removeSub, name], label: `${removeSub} ${name}`, manifest })));
  };

  const updateDep = (name: string, latest: string, manifests: string[]) => {
    const spec = `${name}@${latest || 'latest'}`;
    void runInWorkspaces(manifests.map((manifest) => ({ args: [addSub, spec], label: `update ${name}`, manifest })));
  };

  // Update every outdated package in the active scope: just the current
  // workspace tab, or the whole repo on the "All" tab.
  const updateAll = () => {
    const byManifest = new Map<string, string[]>();
    const queue = (name: string, latest: string, manifest: string) => {
      const specs = byManifest.get(manifest) ?? [];
      specs.push(`${name}@${latest}`);
      byManifest.set(manifest, specs);
    };
    if (activeWorkspace) {
      for (const dep of activeWorkspace.dependencies ?? []) {
        if (dep.type === 'peerDependency' || depUnused(dep, activeWorkspace.name)) {
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
          if (loc.type === 'peerDependency' || unused.has(`${loc.workspace}::${dep.name}`)) {
            continue;
          }
          const hit = updateInfo(dep.name, loc.workspace);
          if (hit) {
            queue(dep.name, hit.latest, loc.manifest);
          }
        }
      }
    }
    const commands = [...byManifest].map(([manifest, specs]) => ({ args: [addSub, ...specs], label: t('integrations.nodejs.updateAll'), manifest }));
    if (commands.length > 0) {
      void runInWorkspaces(commands);
    }
  };

  const openResolve = (dep: nodejs.Dependency) => {
    const versions = conflictVersions(dep) ?? [];
    setResolveTarget(outdatedByName.get(dep.name)?.latest ?? versions[0] ?? '');
    setResolveDep(dep);
  };

  const applyResolve = async () => {
    if (!resolveDep || !resolveTarget) {
      return;
    }
    // Edit every declaring package.json (peerDependencies included, which `add`
    // can't target), then install once to sync the lockfile.
    const manifests = [...new Set((resolveDep.locations ?? []).map((l) => l.manifest))];
    const name = resolveDep.name;
    setResolveDep(null);
    try {
      await SetNodeDependencyVersion(manifests, name, resolveTarget);
    } catch (err) {
      toastError({ title: t('integrations.nodejs.couldNotRunCommand'), err });
      return;
    }
    void runCommand(['install'], `align ${name}@${resolveTarget}`);
  };

  const resolveOptions = useMemo(() => {
    if (!resolveDep) {
      return [];
    }
    const declared = [...new Set((resolveDep.locations ?? []).map((l) => l.version))];
    const latest = outdatedByName.get(resolveDep.name)?.latest;
    return latest && !declared.includes(latest) ? [latest, ...declared] : declared;
  }, [resolveDep, outdatedByName]);

  const renderRow = (dep: nodejs.Dependency, opts: { update?: nodejs.OutdatedPackage; manifests: string[]; rowKey: string; conflict?: string[] | null; isUnused: boolean; vulns?: nodejs.Vulnerability[] }) => {
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
                {t('integrations.nodejs.versionMismatch', { versions: conflict.join(', ') })}
              </TooltipContent>
            </Tooltip>
          ) : update && isTagSpec(dep.version) ? (
            <Tooltip>
              <TooltipTrigger asChild>
                <span className="cursor-help underline decoration-dotted underline-offset-2">{update.current}</span>
              </TooltipTrigger>
              <TooltipContent side="top" className="font-mono text-xs">
                {t('integrations.nodejs.declaredSpec', { spec: dep.version })}
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
                        {v.severity}: {v.title}
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
                    {t('integrations.nodejs.unused')}
                  </button>
                </TooltipTrigger>
                <TooltipContent side="top">{t('integrations.nodejs.unusedTooltip')}</TooltipContent>
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
                <TooltipContent side="top">{t('integrations.nodejs.updateTo', { version: update.latest })}</TooltipContent>
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
        <p className="text-sm text-muted-foreground">{t('integrations.nodejs.selectProject')}</p>
      </div>
    );
  }

  if (!manifestPath) {
    return (
      <div className="flex h-full flex-col gap-6 p-4">
        <header className="flex flex-col gap-1">
          <h1 className="text-2xl font-semibold tracking-tight">{t('integrations.nodejs.title')}</h1>
          <p className="text-sm text-muted-foreground">{t('integrations.nodejs.noManifest', { project: project.name })}</p>
        </header>
        <Card className="border-dashed">
          <CardHeader className="items-center text-center">
            <div className="mb-3 flex size-12 items-center justify-center rounded-full bg-muted">
              <Plug className="size-5 text-muted-foreground" />
            </div>
            <CardTitle className="text-base">{t('integrations.nodejs.connectTitle')}</CardTitle>
            <CardDescription>{t('integrations.nodejs.connectDesc')}</CardDescription>
          </CardHeader>
          <CardContent className="flex justify-center">
            <ConfigureIntegrationModal projectId={project.id} integrationId="nodejs">
              <Button>{t('integrations.nodejs.configureCta')}</Button>
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
          icon={<Package className="size-5 text-muted-foreground" />}
          title={t('integrations.nodejs.title')}
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
                    <SelectItem value="all">{t('integrations.nodejs.viewAll')}</SelectItem>
                    {workspaces.map((ws) => (
                      <SelectItem key={ws.manifest} value={ws.manifest}>
                        {ws.isRoot ? t('integrations.nodejs.rootWorkspace') : ws.name}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              )}
              <Button variant="outline" size="sm" disabled={isRunning || installed} onClick={installAll}>
                <Download className="size-3.5" />
                {t('integrations.nodejs.installAll')}
              </Button>
              <RefreshAction onRefresh={() => refreshEverything()} loading={loading} />
              <ConfigureIntegrationModal projectId={project.id} integrationId="nodejs" instanceIndex={instanceIndex}>
                <Button variant="outline" size="sm">
                  <Settings className="size-3.5" />
                  {t('common.configure')}
                </Button>
              </ConfigureIntegrationModal>
            </div>
          }
        />

        {/* Scripts */}
        <section>
          <h2 className="mb-3 text-sm font-medium text-muted-foreground">{t('integrations.nodejs.tabScripts')}</h2>
          {scripts.length === 0 ? (
            <p className="text-sm text-muted-foreground">{t('integrations.nodejs.noScripts')}</p>
          ) : (
            <div className="flex flex-wrap gap-2">
              {scripts.map((script) => {
                const active = isRunning && run?.scriptName === script.name;
                return (
                  <Tooltip key={script.name}>
                    <TooltipTrigger asChild>
                      <Button variant={active ? 'default' : 'outline'} size="sm" className={cn('gap-1.5 font-mono text-xs', active && 'animate-pulse')} disabled={isRunning && !active} onClick={() => (active ? stop() : startScript(script.name))}>
                        {active ? <Square className="size-3" /> : <Play className="size-3" />}
                        {script.name}
                      </Button>
                    </TooltipTrigger>
                    <TooltipContent side="bottom" className="font-mono text-xs">
                      {script.command}
                    </TooltipContent>
                  </Tooltip>
                );
              })}
            </div>
          )}
        </section>

        {/* Packages */}
        <section>
          <div className="mb-3 flex flex-col gap-2">
            <h2 className="text-sm font-medium text-muted-foreground">
              {t('integrations.nodejs.tabPackages')} ({packages.length})
            </h2>

            <div className="flex items-center gap-3">
              <div className="flex-1" />

              <Button size="sm" variant="outline" className="h-7 gap-1.5 text-xs" onClick={() => void copyDiagnostic()}>
                <Stethoscope className="size-3" />
                {t('integrations.nodejs.copyDiagnostic')}
              </Button>

              {outdatedCount > 0 && (
                <Button size="sm" variant="outline" disabled={isRunning} onClick={updateAll} className="h-7 gap-1.5 text-xs text-amber-400 hover:text-amber-300">
                  <ArrowUpCircle className="size-3" />
                  {t('integrations.nodejs.updateAll')} ({outdatedCount})
                </Button>
              )}

              <Button size="sm" variant="outline" disabled={isRunning} className="h-7 gap-1.5 text-xs" onClick={() => openAdd(activeWorkspace?.manifest)}>
                <Package className="size-3" />
                {t('integrations.nodejs.addPackage')}
              </Button>
            </div>
          </div>

          {packages.length === 0 ? (
            <p className="text-sm text-muted-foreground">{t('integrations.nodejs.noPackages')}</p>
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
                            {PKG_TYPE_LABELS[type] ?? type}
                            <Badge variant="secondary" className={cn('ml-1 h-4 px-1.5 text-[10px]', PKG_TYPE_COLORS[type])}>
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
              <DialogTitle>{t('integrations.nodejs.addPackage')}</DialogTitle>
              <DialogDescription>{t('integrations.nodejs.addPackageDesc')}</DialogDescription>
            </DialogHeader>
            <div className="flex flex-col gap-4">
              <Input value={addInput} onChange={(e) => setAddInput(e.target.value)} onKeyDown={(e) => e.key === 'Enter' && addPackage()} placeholder={t('integrations.nodejs.addPackagePlaceholder')} autoFocus />
              <div className="flex items-center gap-2 text-sm">
                <Switch id="add-as-dev" checked={addAsDev} onCheckedChange={setAddAsDev} />
                <label htmlFor="add-as-dev">{t('integrations.nodejs.addAsDev')}</label>
              </div>
            </div>
            <DialogFooter>
              <Button disabled={!addInput.trim()} onClick={addPackage}>
                {t('integrations.nodejs.installAll')}
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>

        <Dialog open={resolveDep !== null} onOpenChange={(open) => !open && setResolveDep(null)}>
          <DialogContent className="sm:max-w-md">
            <DialogHeader>
              <DialogTitle>{t('integrations.nodejs.resolveTitle', { name: resolveDep?.name })}</DialogTitle>
              <DialogDescription>{t('integrations.nodejs.resolveDesc')}</DialogDescription>
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
                  <span className="whitespace-nowrap text-sm text-muted-foreground">{t('integrations.nodejs.resolveTarget')}</span>
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
                {t('integrations.nodejs.resolveApply')}
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      </div>
    </ScrollArea>
  );
}
