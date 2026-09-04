import type { LucideIcon } from 'lucide-react';
import { Bot, Check, Copy, ExternalLink, FolderOpen, GitBranch, Globe, RefreshCw, Sparkles, TriangleAlert } from 'lucide-react';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { SUPPORTED_LANGUAGES } from '@/lib/i18n';
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
  const { t } = useTranslation();
  const Icon = kind.icon;
  const [copied, setCopied] = useState(false);

  async function copyCmd() {
    try {
      await navigator.clipboard.writeText(kind.installCmd);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch {
      // clipboard access denied or unavailable
    }
  }

  return (
    <div className="flex items-center gap-3 rounded-md border border-border bg-card/40 px-3 py-2">
      <div className={cn('flex size-8 shrink-0 items-center justify-center rounded-md', kind.iconClass)}>
        <Icon className="size-4" aria-hidden />
      </div>
      <div className="min-w-0 flex-1">
        <div className="text-sm font-medium">{kind.label}</div>
        <div className="flex items-center gap-1">
          <code className="min-w-0 flex-1 truncate font-mono text-[11px] text-muted-foreground" title={kind.installCmd}>
            {kind.installCmd}
          </code>
          <button type="button" onClick={() => void copyCmd()} aria-label={t('projects.welcome.copyInstall', { label: kind.label })} className="shrink-0 text-muted-foreground hover:text-foreground">
            {copied ? <Check className="size-3 text-green-500" aria-hidden /> : <Copy className="size-3" aria-hidden />}
          </button>
          <span role="status" aria-live="polite" className="sr-only">
            {copied ? t('projects.welcome.copied') : ''}
          </span>
        </div>
      </div>
      <Button type="button" variant="ghost" size="sm" onClick={() => BrowserOpenURL(kind.docsUrl)} aria-label={`${kind.label} docs`}>
        <ExternalLink />
      </Button>
    </div>
  );
}

export function WelcomeWizard() {
  const { t, i18n } = useTranslation();
  const { kinds, opencode, loading, refresh } = useAgentClis();
  const allKinds = [...kinds, opencode];
  const anyInstalled = allKinds.some((k) => k.installed);
  const [skipped, setSkipped] = useState(false);
  const showInstallGate = !loading && !anyInstalled && !skipped;

  return (
    <div className="flex min-h-full flex-col items-center justify-center p-4">
      <Card className="relative w-full max-w-2xl py-0">
        <div className="absolute right-4 top-4">
          <Select value={i18n.language} onValueChange={(value) => i18n.changeLanguage(value)}>
            <SelectTrigger size="sm" className="w-auto gap-1.5">
              <Globe className="size-3.5 text-muted-foreground" />
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {SUPPORTED_LANGUAGES.map((lang) => (
                <SelectItem key={lang.code} value={lang.code}>
                  {lang.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>

        <CardContent className="flex flex-col gap-6 py-6">
          <div className="flex flex-col items-center gap-2 text-center">
            <div className="flex size-12 items-center justify-center rounded-xl bg-primary text-primary-foreground">
              <Sparkles className="size-6" />
            </div>
            <h1 className="text-xl font-semibold tracking-tight">{t('projects.welcome.title')}</h1>
            <p className="max-w-sm text-sm text-muted-foreground">{t('projects.welcome.description')}</p>
          </div>

          {loading ? (
            <div role="status" className="flex justify-center py-4">
              <RefreshCw className="size-5 animate-spin text-muted-foreground" aria-hidden />
              <span className="sr-only">{t('projects.welcome.checking')}</span>
            </div>
          ) : showInstallGate ? (
            <div className="flex flex-col gap-3">
              <div className="flex items-start gap-3 rounded-md border border-amber-500/30 bg-amber-500/10 px-3 py-2">
                <TriangleAlert className="mt-0.5 size-4 shrink-0 text-amber-400" />
                <div className="flex flex-col gap-0.5">
                  <div className="text-sm font-medium">{t('projects.welcome.noToolsTitle')}</div>
                  <div className="text-xs text-muted-foreground">{t('projects.welcome.noToolsDesc')}</div>
                </div>
              </div>
              <div className="grid grid-cols-1 gap-2 sm:grid-cols-2">
                {allKinds.map((k) => (
                  <ToolInstallRow key={k.id} kind={k} />
                ))}
              </div>
              <Button type="button" variant="outline" size="lg" onClick={() => void refresh()} disabled={loading}>
                <RefreshCw className={cn(loading && 'animate-spin')} />
                {t('projects.welcome.recheck')}
              </Button>
              <Button type="button" variant="ghost" size="sm" onClick={() => setSkipped(true)} className="text-muted-foreground">
                {t('projects.welcome.skipNoTools')}
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
                <Button size="lg" disabled={loading} className="w-full">
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
