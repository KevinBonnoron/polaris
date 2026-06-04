import { FlaskConical, Hammer, Package, Play, Square } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { Button } from '@/components/ui/button';
import { cn } from '@/lib/utils';
import { CheckOutdatedPackages, CheckPackagesInstalled, CheckUnusedPackages, CheckVulnerabilities, ListNodePackages, ListNodeWorkspaces, SetNodeDependencyVersion } from '@/wailsjs/go/main/App';
import type { PackageManagerAdapter, RuntimeRunContext } from '../package-manager-page';
import { PackageManagerPage } from '../package-manager-page';
import { useNodejsRun } from './nodejs-run-context';
import type { NodejsConfig } from './types';

function NodejsScripts({ runCtx }: { runCtx: RuntimeRunContext }) {
  const { t } = useTranslation();
  const { run, isRunning, config, startScript, stop } = runCtx;
  const typedConfig = config as NodejsConfig | null;
  const extraScripts = (typedConfig?.extraScripts ?? '')
    .split(',')
    .map((s) => s.trim())
    .filter(Boolean);

  return (
    <section className="flex flex-col gap-2">
      <h2 className="text-sm font-medium text-muted-foreground">{t('integrations.nodejs.tabScripts')}</h2>
      <div className="grid grid-cols-3 gap-2">
        {(
          [
            { key: 'startScript', Icon: Play, label: t('integrations.nodejs.startAction') },
            { key: 'testScript', Icon: FlaskConical, label: t('integrations.nodejs.testAction') },
            { key: 'buildScript', Icon: Hammer, label: t('integrations.nodejs.buildAction') },
          ] as const
        ).map(({ key, Icon, label }) => {
          const scriptName = typedConfig?.[key];
          const active = isRunning && !!scriptName && run?.scriptName === scriptName;
          if (!scriptName) {
            return (
              <div key={key} className="flex flex-col gap-1 rounded-md border border-dashed px-3 py-2.5 opacity-40">
                <div className="flex items-center gap-2 text-muted-foreground">
                  <Icon className="size-3 shrink-0" />
                  <span className="text-xs">{label}</span>
                </div>
                <span className="font-mono text-xs text-muted-foreground">{t('integrations.nodejs.notConfigured')}</span>
              </div>
            );
          }
          return (
            <button
              key={key}
              type="button"
              disabled={isRunning && !active}
              onClick={() => (active ? stop() : startScript(scriptName))}
              className={cn('group flex flex-col gap-1 rounded-md border px-3 py-2.5 text-left transition-colors disabled:pointer-events-none disabled:opacity-50', active ? 'border-primary/40 bg-primary/5' : 'hover:bg-muted/50')}
            >
              <div className="flex items-center gap-2">
                {active ? <Square className="size-3 shrink-0 animate-pulse text-primary" /> : <Icon className="size-3 shrink-0 text-muted-foreground transition-colors group-hover:text-foreground" />}
                <span className="text-xs text-muted-foreground">{label}</span>
              </div>
              <span className="font-mono text-sm">{scriptName}</span>
            </button>
          );
        })}
      </div>
      {extraScripts.length > 0 && (
        <div className="flex flex-wrap gap-1.5">
          {extraScripts.map((name) => {
            const active = isRunning && run?.scriptName === name;
            return (
              <Button key={name} variant={active ? 'default' : 'outline'} size="sm" className={cn('gap-1.5 font-mono text-xs', active && 'animate-pulse')} disabled={isRunning && !active} onClick={() => (active ? stop() : startScript(name))}>
                {active ? <Square className="size-3" /> : <Play className="size-3" />}
                {name}
              </Button>
            );
          })}
        </div>
      )}
    </section>
  );
}

const nodejsAdapter: PackageManagerAdapter = {
  integrationId: 'nodejs',
  runtime: 'Node.js',
  i18nKey: 'integrations.nodejs',
  icon: <Package className="size-5 text-muted-foreground" />,
  skipDepType: 'peerDependency',
  pkgTypeLabels: { dependency: 'Dependencies', devDependency: 'Dev dependencies', peerDependency: 'Peer dependencies' },
  pkgTypeColors: { dependency: 'bg-emerald-500/10 text-emerald-400', devDependency: 'bg-blue-500/10 text-blue-400', peerDependency: 'bg-amber-500/10 text-amber-400' },

  useRun: useNodejsRun as () => RuntimeRunContext,

  loadPackages: (_, mp) => ListNodePackages(mp),
  loadWorkspaces: (_, mp) => ListNodeWorkspaces(mp),
  loadScripts: () => Promise.resolve(null),

  checkOutdated: CheckOutdatedPackages,
  checkUnused: (_, mp, pm, env) => CheckUnusedPackages(mp, pm, env),
  checkVulnerabilities: CheckVulnerabilities,
  checkInstalled: (_, mp) => CheckPackagesInstalled(mp),
  setDependencyVersion: SetNodeDependencyVersion,

  addArgs: (pm, name, dev) => {
    const sub = pm === 'npm' ? 'install' : 'add';
    return dev ? [sub, '-D', name] : [sub, name];
  },
  removeArgs: (pm, name) => [pm === 'npm' ? 'uninstall' : 'remove', name],
  installArgs: () => ['install'],
  formatSpec: (name, version) => `${name}@${version || 'latest'}`,
  updateAllArgs: (pm, specs) => [pm === 'npm' ? 'install' : 'add', ...specs],

  renderScripts: ({ runCtx }) => <NodejsScripts runCtx={runCtx} />,
};

export function NodejsPage() {
  return <PackageManagerPage adapter={nodejsAdapter} />;
}
