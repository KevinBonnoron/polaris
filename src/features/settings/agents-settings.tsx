import { CheckCircle2, ChevronDown, Copy, ExternalLink, XCircle } from 'lucide-react';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import { AgentModelSelect } from '@/features/agents/agent-model-select';
import { ClaudeUsageBar } from '@/features/agents/claude-usage-bar';
import { cn } from '@/lib/utils';
import { type AgentCliInfo, useAgentClis } from '@/state/agent-clis';
import { useAgentDefaults } from '@/state/agent-defaults';
import { BrowserOpenURL } from '@/wailsjs/runtime/runtime';
import { CustomProvidersSection } from './custom-providers-section';

interface AgentKindCardProps {
  kind: AgentCliInfo;
  detecting: boolean;
  defaultModel: string;
  onDefaultModelChange: (model: string) => void;
  note?: string;
}

function AgentKindCard({ kind, detecting, defaultModel, onDefaultModelChange, note }: AgentKindCardProps) {
  const { t } = useTranslation();
  const installed = kind.installed;
  const Icon = kind.icon;
  const showInstallActions = !installed && !detecting;
  const binaryHint = installed ? (kind.path ?? kind.binary) : kind.installCmd;
  const isClaude = kind.id === 'claude-code';
  const hasModels = kind.models.length > 0;
  const [copied, setCopied] = useState(false);

  const copyInstall = () => {
    navigator.clipboard.writeText(kind.installCmd).then(() => {
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    });
  };

  return (
    <Card className={cn(showInstallActions && 'opacity-60')}>
      <CardContent className="py-2.5">
        <div className="flex items-center gap-3">
          <div className={cn('flex size-8 shrink-0 items-center justify-center rounded-md', kind.iconClass)}>
            <Icon className="size-4" />
          </div>
          <div className="min-w-0 flex-1">
            <div className="flex items-center gap-2">
              <div className="text-sm font-medium">{kind.label}</div>
              <div className={cn('flex items-center gap-1 text-[10px]', detecting ? 'text-muted-foreground' : installed ? 'text-emerald-400' : 'text-muted-foreground')}>
                {detecting ? t('settings.agents.detecting') : installed ? <CheckCircle2 className="size-3" /> : <XCircle className="size-3" />}
                <span>{detecting ? '' : installed ? t('settings.agents.installed') : t('settings.agents.notFound')}</span>
              </div>
            </div>
            <div className="truncate font-mono text-[10px] text-muted-foreground" title={note ?? binaryHint}>
              {note ?? binaryHint}
            </div>
          </div>
          {showInstallActions ? (
            <div className="flex shrink-0 items-center gap-0.5">
              <Button type="button" variant="ghost" size="icon" className="size-7" onClick={copyInstall} title={t('settings.agents.copyInstall')}>
                <Copy className={cn('size-3.5 transition-colors', copied && 'text-emerald-400')} />
              </Button>
              <Button type="button" variant="ghost" size="icon" className="size-7" onClick={() => BrowserOpenURL(kind.docsUrl)} title={t('settings.agents.openDocs')}>
                <ExternalLink className="size-3.5" />
              </Button>
            </div>
          ) : (
            hasModels && <AgentModelSelect models={kind.models} value={defaultModel} onChange={onDefaultModelChange} disabled={!installed} />
          )}
        </div>
        {isClaude && installed && <ClaudeUsageBar model={defaultModel} />}
      </CardContent>
    </Card>
  );
}

export function AgentsSettings() {
  const { t } = useTranslation();
  const { kinds, opencode, loading: detecting } = useAgentClis();
  const defaults = useAgentDefaults();
  const [showNotInstalled, setShowNotInstalled] = useState(false);

  const installed = detecting ? kinds : kinds.filter((k) => k.installed);
  const notInstalled = detecting ? [] : kinds.filter((k) => !k.installed);

  return (
    <section className="flex flex-col gap-6">
      <div className="flex flex-col gap-1">
        <h3 className="text-base font-semibold">{t('settings.agents.title')}</h3>
        <p className="text-xs text-muted-foreground">{t('settings.agents.subtitle')}</p>
      </div>

      <div className="flex flex-col gap-2">
        {installed.map((kind) => (
          <AgentKindCard key={kind.id} kind={kind} detecting={detecting} defaultModel={defaults.get(kind.id) ?? kind.models[0]?.value ?? ''} onDefaultModelChange={(model) => defaults.set(kind.id, model)} />
        ))}
        <AgentKindCard kind={opencode} detecting={detecting} defaultModel={defaults.get(opencode.id) ?? opencode.models[0]?.value ?? ''} onDefaultModelChange={(model) => defaults.set(opencode.id, model)} note={t('settings.agents.opencodeNote')} />

        {notInstalled.length > 0 && (
          <>
            <Button type="button" variant="ghost" size="sm" className="h-7 w-fit gap-1.5 px-2 text-xs text-muted-foreground" onClick={() => setShowNotInstalled((v) => !v)}>
              <ChevronDown className={cn('size-3 transition-transform', showNotInstalled && 'rotate-180')} />
              {t('settings.agents.notInstalledSection', { count: notInstalled.length })}
            </Button>
            {showNotInstalled &&
              notInstalled.map((kind) => (
                <AgentKindCard key={kind.id} kind={kind} detecting={false} defaultModel={defaults.get(kind.id) ?? kind.models[0]?.value ?? ''} onDefaultModelChange={(model) => defaults.set(kind.id, model)} />
              ))}
          </>
        )}
      </div>

      <CustomProvidersSection />
    </section>
  );
}
