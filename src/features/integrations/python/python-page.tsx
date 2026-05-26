import { Boxes, Play, Square } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { Button } from '@/components/ui/button';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import { cn } from '@/lib/utils';
import { CheckPythonOutdatedPackages, CheckPythonPackagesInstalled, CheckPythonUnusedPackages, CheckPythonVulnerabilities, ListPythonPackages, ListPythonScripts, ListPythonWorkspaces, SetPythonDependencyVersion } from '@/wailsjs/go/main/App';
import type { GenericScript, PackageManagerAdapter, RuntimeRunContext } from '../package-manager-page';
import { PackageManagerPage } from '../package-manager-page';
import { usePythonRun } from './python-run-context';
import { addArgs, installArgs, removeArgs } from './types';

function PythonScripts({ scripts, runCtx }: { scripts: GenericScript[]; runCtx: RuntimeRunContext }) {
  const { t } = useTranslation();
  const { run, isRunning, startScript, stop } = runCtx;

  return (
    <section>
      <h2 className="mb-3 text-sm font-medium text-muted-foreground">{t('integrations.python.tabScripts')}</h2>
      {scripts.length === 0 ? (
        <p className="text-sm text-muted-foreground">{t('integrations.python.noScripts')}</p>
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
                <TooltipContent side="bottom" className="font-mono text-xs">{script.command}</TooltipContent>
              </Tooltip>
            );
          })}
        </div>
      )}
    </section>
  );
}

const pythonAdapter: PackageManagerAdapter = {
  integrationId: 'python',
  runtime: 'Python',
  i18nKey: 'integrations.python',
  icon: <Boxes className="size-5 text-muted-foreground" />,
  skipDepType: 'optional',
  pkgTypeLabels: { dependency: 'Dependencies', devDependency: 'Dev dependencies', optional: 'Optional dependencies' },
  pkgTypeColors: { dependency: 'bg-emerald-500/10 text-emerald-400', devDependency: 'bg-blue-500/10 text-blue-400', optional: 'bg-amber-500/10 text-amber-400' },

  useRun: usePythonRun as () => RuntimeRunContext,

  loadPackages: ListPythonPackages,
  loadWorkspaces: ListPythonWorkspaces,
  loadScripts: ListPythonScripts,

  checkOutdated: CheckPythonOutdatedPackages,
  checkUnused: CheckPythonUnusedPackages,
  checkVulnerabilities: CheckPythonVulnerabilities,
  checkInstalled: CheckPythonPackagesInstalled,
  setDependencyVersion: SetPythonDependencyVersion,

  addArgs,
  removeArgs,
  installArgs,
  formatSpec: (name, version) => (version ? `${name}==${version}` : name),
  updateAllArgs: (_, specs) => ['add', ...specs],

  renderScripts: ({ scripts, runCtx }) => <PythonScripts scripts={scripts} runCtx={runCtx} />,
};

export function PythonPage() {
  return <PackageManagerPage adapter={pythonAdapter} />;
}
