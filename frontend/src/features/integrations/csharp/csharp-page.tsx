import { Hash, Play, Square } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { Button } from '@/components/ui/button';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import { cn } from '@/lib/utils';
import { CheckCSharpOutdatedPackages, CheckCSharpPackagesInstalled, CheckCSharpUnusedPackages, CheckCSharpVulnerabilities, ListCSharpPackages, ListCSharpScripts, ListCSharpWorkspaces, SetCSharpDependencyVersion } from '@/wailsjs/go/main/App';
import type { GenericScript, PackageManagerAdapter, RuntimeRunContext } from '../package-manager-page';
import { PackageManagerPage } from '../package-manager-page';
import { useCSharpRun } from './csharp-run-context';
import { addArgs, installArgs, removeArgs } from './types';

function CSharpScripts({ scripts, runCtx }: { scripts: GenericScript[]; runCtx: RuntimeRunContext }) {
  const { t } = useTranslation();
  const { run, isRunning, startScript, stop } = runCtx;

  return (
    <section>
      <h2 className="mb-3 text-sm font-medium text-muted-foreground">{t('integrations.csharp.tabScripts')}</h2>
      {scripts.length === 0 ? (
        <p className="text-sm text-muted-foreground">{t('integrations.csharp.noScripts')}</p>
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
  );
}

const csharpAdapter: PackageManagerAdapter = {
  integrationId: 'csharp',
  runtime: '.NET',
  i18nKey: 'integrations.csharp',
  icon: <Hash className="size-5 text-muted-foreground" />,
  skipDepType: '',
  pkgTypeLabels: { dependency: 'Dependencies' },
  pkgTypeColors: { dependency: 'bg-emerald-500/10 text-emerald-400' },

  useRun: useCSharpRun as () => RuntimeRunContext,

  loadPackages: ListCSharpPackages,
  loadWorkspaces: ListCSharpWorkspaces,
  loadScripts: ListCSharpScripts,

  checkOutdated: CheckCSharpOutdatedPackages,
  checkUnused: CheckCSharpUnusedPackages,
  checkVulnerabilities: CheckCSharpVulnerabilities,
  checkInstalled: CheckCSharpPackagesInstalled,
  setDependencyVersion: SetCSharpDependencyVersion,

  addArgs,
  removeArgs,
  installArgs,
  formatSpec: (name, version) => (version ? `${name}@${version}` : name),
  updateAllArgs: (_, specs) => ['update-packages', ...specs],

  renderScripts: ({ scripts, runCtx }) => <CSharpScripts scripts={scripts} runCtx={runCtx} />,
};

export function CSharpPage() {
  return <PackageManagerPage adapter={csharpAdapter} />;
}
