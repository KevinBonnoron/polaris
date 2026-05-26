import type { LucideIcon } from 'lucide-react';
import { Bot, ExternalLink, FolderOpen, GitBranch, RefreshCw, Sparkles, TriangleAlert } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import { cn } from '@/lib/utils';
import { type AgentCliInfo, useAgentClis } from '@/state/agent-clis';
import { BrowserOpenURL } from '@/wailsjs/runtime/runtime';
import { AddProjectModal } from './add-project-modal';

interface Feature {
  icon: LucideIcon;
  titleKey: 'parallelTitle' | 'toolsTitle' | 'reviewTitle';
  descKey: 'parallelDesc' | 'toolsDesc' | 'reviewDesc';
}

const FEATURES: Feature[] = [
  { icon: Bot, titleKey: 'parallelTitle', descKey: 'parallelDesc' },
  { icon: GitBranch, titleKey: 'toolsTitle', descKey: 'toolsDesc' },
  { icon: Sparkles, titleKey: 'reviewTitle', descKey: 'reviewDesc' },
];

function FeatureRow({ feature }: { feature: Feature }) {
  const { t } = useTranslation();
  const Icon = feature.icon;
  return (
    <div className="flex items-start gap-3">
      <div className="flex size-9 shrink-0 items-center justify-center rounded-md bg-accent">
        <Icon className="size-4 text-foreground" />
      </div>
      <div>
        <div className="text-sm font-medium">{t(`projects.welcome.${feature.titleKey}` as const)}</div>
        <div className="text-xs text-muted-foreground">{t(`projects.welcome.${feature.descKey}` as const)}</div>
      </div>
    </div>
  );
}

function ToolInstallRow({ kind }: { kind: AgentCliInfo }) {
  const Icon = kind.icon;
  return (
    <div className="flex items-center gap-3 rounded-md border border-border bg-card/40 px-3 py-2">
      <div className={cn('flex size-8 shrink-0 items-center justify-center rounded-md', kind.iconClass)}>
        <Icon className="size-4" />
      </div>
      <div className="min-w-0 flex-1">
        <div className="text-sm font-medium">{kind.label}</div>
        <code className="block truncate font-mono text-[11px] text-muted-foreground" title={kind.installCmd}>
          {kind.installCmd}
        </code>
      </div>
      <Button type="button" variant="ghost" size="sm" onClick={() => BrowserOpenURL(kind.docsUrl)} aria-label={`${kind.label} docs`}>
        <ExternalLink />
      </Button>
    </div>
  );
}

export function WelcomeWizard() {
  const { t } = useTranslation();
  const { kinds, loading, refresh } = useAgentClis();
  const anyInstalled = kinds.some((k) => k.installed);
  const showInstallGate = !loading && !anyInstalled;

  return (
    <div className="flex h-full flex-col items-center justify-center p-8">
      <Card className="w-full max-w-xl">
        <CardContent className="flex flex-col gap-6 py-8">
          <div className="flex flex-col items-center gap-2 text-center">
            <div className="flex size-12 items-center justify-center rounded-xl bg-primary text-primary-foreground">
              <Sparkles className="size-6" />
            </div>
            <h1 className="text-xl font-semibold tracking-tight">{t('projects.welcome.title')}</h1>
            <p className="max-w-sm text-sm text-muted-foreground">{t('projects.welcome.description')}</p>
          </div>

          {showInstallGate ? (
            <div className="flex flex-col gap-3">
              <div className="flex items-start gap-3 rounded-md border border-amber-500/30 bg-amber-500/10 px-3 py-2">
                <TriangleAlert className="mt-0.5 size-4 shrink-0 text-amber-400" />
                <div className="flex flex-col gap-0.5">
                  <div className="text-sm font-medium">{t('projects.welcome.noToolsTitle')}</div>
                  <div className="text-xs text-muted-foreground">{t('projects.welcome.noToolsDesc')}</div>
                </div>
              </div>

              <div className="flex flex-col gap-2">
                {kinds.map((k) => (
                  <ToolInstallRow key={k.id} kind={k} />
                ))}
              </div>

              <Button type="button" variant="outline" size="lg" onClick={() => void refresh()} disabled={loading}>
                <RefreshCw className={cn(loading && 'animate-spin')} />
                {t('projects.welcome.recheck')}
              </Button>
            </div>
          ) : (
            <>
              <div className="flex flex-col gap-3">
                {FEATURES.map((f) => (
                  <FeatureRow key={f.titleKey} feature={f} />
                ))}
              </div>

              <AddProjectModal>
                <Button size="lg" disabled={loading}>
                  <FolderOpen />
                  {t('projects.welcome.openFirst')}
                </Button>
              </AddProjectModal>
            </>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
