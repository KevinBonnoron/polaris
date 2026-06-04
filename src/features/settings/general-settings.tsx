import type { LucideIcon } from 'lucide-react';
import { Folder } from 'lucide-react';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import { Dialog, DialogClose, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { Skeleton } from '@/components/ui/skeleton';
import { Switch } from '@/components/ui/switch';
import { SUPPORTED_LANGUAGES } from '@/lib/i18n';
import { cn } from '@/lib/utils';
import { DetectIdes, GetGeneralSettings, ResetAllData, UpdateGeneralSettings } from '@/wailsjs/go/main/App';
import { polaris } from '@/wailsjs/go/models';
import { SettingsRow } from './settings-row';

interface IdeOption {
  id: string;
  name: string;
  cmd: string;
  color: string;
  glyph?: string;
  icon?: LucideIcon;
}

const IDES: IdeOption[] = [
  { id: 'vscode', name: 'VS Code', cmd: 'code --goto "$FILE:$LINE:$COL"', color: '#0078D4', glyph: '</>' },
  { id: 'cursor', name: 'Cursor', cmd: 'cursor --goto "$FILE:$LINE:$COL"', color: '#1e1e1e', glyph: '⌘' },
  { id: 'zed', name: 'Zed', cmd: 'zed "$FILE:$LINE:$COL"', color: '#084CCD', glyph: 'Z' },
  { id: 'windsurf', name: 'Windsurf', cmd: 'windsurf "$FILE"', color: '#06b6d4', glyph: 'W' },
  { id: 'jetbrains', name: 'JetBrains', cmd: 'idea "$FILE" --line $LINE', color: '#FE315D', glyph: 'IJ' },
  { id: 'sublime', name: 'Sublime', cmd: 'subl "$FILE":$LINE', color: '#FF9800', glyph: 'S' },
  { id: 'xcode', name: 'Xcode', cmd: 'xed "$FILE"', color: '#147EFB', glyph: 'X' },
  { id: 'vim', name: 'Vim / Neovim', cmd: 'vim "$FILE" +$LINE', color: '#019733', glyph: 'V' },
  { id: 'finder', name: 'Finder', cmd: 'open -R "$FILE"', color: '#1268D1', icon: Folder },
];

interface IdeCardProps {
  ide: IdeOption;
  selected: boolean;
  detected: boolean;
  onSelect: (id: string) => void;
}

function IdeCard({ ide, selected, detected, onSelect }: IdeCardProps) {
  const { t } = useTranslation();
  const Icon = ide.icon;
  return (
    <button
      type="button"
      onClick={() => onSelect(ide.id)}
      disabled={!detected && !selected}
      className={cn('flex items-center gap-2 rounded-md border p-2 text-left transition-colors', selected ? 'border-primary/60 bg-accent/40' : 'border-border hover:bg-accent/30', !detected && !selected && 'cursor-not-allowed opacity-40')}
    >
      <div className="flex size-7 shrink-0 items-center justify-center rounded text-[10px] font-bold text-white" style={{ background: ide.color }}>
        {Icon ? <Icon className="size-3.5" /> : ide.glyph}
      </div>
      <div className="min-w-0 flex-1">
        <div className="truncate text-xs font-medium">{ide.name}</div>
        <div className="text-[10px] text-muted-foreground">{detected ? t('settings.general.detected') : t('settings.general.notInstalled')}</div>
      </div>
    </button>
  );
}

export function GeneralSettings() {
  const { t, i18n } = useTranslation();
  const [ideId, setIdeId] = useState('vscode');
  const [detected, setDetected] = useState<Record<string, boolean>>({});
  const [idesLoading, setIdesLoading] = useState(true);
  const [settingsLoading, setSettingsLoading] = useState(true);
  const [autoResume, setAutoResume] = useState(false);
  const [agentCloseAction, setAgentCloseAction] = useState('archive');
  const [resetOpen, setResetOpen] = useState(false);
  const [confirmText, setConfirmText] = useState('');
  const [busy, setBusy] = useState(false);
  const [resetError, setResetError] = useState<string | null>(null);

  const confirmWord = t('settings.general.resetConfirmWord');
  const canReset = confirmText.trim().toUpperCase() === confirmWord.toUpperCase();

  const handleReset = async () => {
    if (busy || !canReset) {
      return;
    }

    setBusy(true);
    setResetError(null);
    try {
      await ResetAllData();
      setResetOpen(false);
      setConfirmText('');
    } catch (e) {
      setResetError(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  };

  useEffect(() => {
    let cancelled = false;
    DetectIdes()
      .then((results) => {
        if (cancelled) { return; }
        const map: Record<string, boolean> = {};
        for (const r of results) {
          map[r.id] = r.installed;
        }
        setDetected(map);
        setIdesLoading(false);
      })
      .catch(() => {
        if (!cancelled) { setIdesLoading(false); }
      });
    GetGeneralSettings()
      .then((s) => {
        if (cancelled) { return; }
        setAutoResume(s.autoResumeSessions);
        if (s.ideId) { setIdeId(s.ideId); }
        if (s.agentCloseAction) { setAgentCloseAction(s.agentCloseAction); }
        setSettingsLoading(false);
      })
      .catch(() => {
        if (!cancelled) { setSettingsLoading(false); }
      });
    return () => {
      cancelled = true;
    };
  }, []);

  const persist = (patch: { autoResumeSessions?: boolean; ideId?: string; ideCmd?: string; agentCloseAction?: string }) => {
    const next = polaris.GeneralSettings.createFrom({ autoResumeSessions: autoResume, ideId, agentCloseAction, ...patch });
    UpdateGeneralSettings(next).catch(() => {});
  };

  const handleAutoResumeChange = (next: boolean) => {
    setAutoResume(next);
    persist({ autoResumeSessions: next });
  };

  const handleSelect = (id: string) => {
    setIdeId(id);
    const ide = IDES.find((i) => i.id === id);
    persist({ ideId: id, ideCmd: ide?.cmd });
  };

  const handleAgentCloseActionChange = (value: string) => {
    setAgentCloseAction(value);
    persist({ agentCloseAction: value });
  };

  const currentLang = i18n.resolvedLanguage ?? i18n.language;

  return (
    <section className="flex flex-col gap-6">
      <div className="rounded-md border border-border px-3">
        <SettingsRow
          label={t('settings.general.language')}
          description={t('settings.general.languageDesc')}
          control={
            <Select value={currentLang} onValueChange={(value) => i18n.changeLanguage(value)}>
              <SelectTrigger size="sm">
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
          }
        />
        <SettingsRow label={t('settings.general.autoSave')} description={t('settings.general.autoSaveDesc')} control={settingsLoading ? <Skeleton className="h-5 w-9 rounded-full" /> : <Switch checked={autoResume} onCheckedChange={handleAutoResumeChange} />} />
        <SettingsRow
          label={t('settings.general.agentCloseAction')}
          description={t('settings.general.agentCloseActionDesc')}
          control={
            settingsLoading ? (
              <Skeleton className="h-8 w-28 rounded-md" />
            ) : (
              <Select value={agentCloseAction} onValueChange={handleAgentCloseActionChange}>
                <SelectTrigger size="sm">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="archive">{t('settings.general.agentCloseActionArchive')}</SelectItem>
                  <SelectItem value="delete">{t('settings.general.agentCloseActionDelete')}</SelectItem>
                </SelectContent>
              </Select>
            )
          }
        />
      </div>

      <div className="flex flex-col gap-2">
        <div className="flex items-baseline justify-between">
          <h4 className="text-xs font-medium uppercase tracking-wide text-muted-foreground">{t('settings.general.defaultIde')}</h4>
          <span className="text-[11px] text-muted-foreground">{t('settings.general.defaultIdeHint')}</span>
        </div>
        {idesLoading ? (
          <div className="grid grid-cols-2 gap-2 md:grid-cols-3">
            {IDES.map((ide) => (
              <Skeleton key={ide.id} className="h-[52px] rounded-md" />
            ))}
          </div>
        ) : (
          <div className="grid grid-cols-2 gap-2 md:grid-cols-3">
            {IDES.map((ide) => (
              <IdeCard key={ide.id} ide={ide} selected={ideId === ide.id} detected={detected[ide.id] ?? false} onSelect={handleSelect} />
            ))}
          </div>
        )}
      </div>

      <div className="flex flex-col gap-2">
        <h4 className="text-xs font-medium uppercase tracking-wide text-destructive">{t('settings.general.dangerZone')}</h4>
        <Card className="border-destructive/40">
          <CardContent className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
            <div className="min-w-0">
              <div className="text-sm font-medium">{t('settings.general.resetAllData')}</div>
              <p className="text-xs text-muted-foreground">{t('settings.general.resetAllDataDesc')}</p>
            </div>
            <Button variant="destructive" onClick={() => setResetOpen(true)}>
              {t('settings.general.resetAllData')}
            </Button>
          </CardContent>
        </Card>
      </div>

      <Dialog
        open={resetOpen}
        onOpenChange={(open) => {
          if (!busy) {
            setResetOpen(open);
            if (!open) {
              setConfirmText('');
              setResetError(null);
            }
          }
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('settings.general.resetAllData')}</DialogTitle>
            <DialogDescription>{t('settings.general.resetConfirmDesc', { word: confirmWord })}</DialogDescription>
          </DialogHeader>
          <Input value={confirmText} onChange={(e) => setConfirmText(e.target.value)} placeholder={confirmWord} autoFocus />
          {resetError && <p className="text-xs text-destructive">{resetError}</p>}
          <DialogFooter>
            <DialogClose asChild>
              <Button variant="outline" disabled={busy}>
                {t('common.cancel')}
              </Button>
            </DialogClose>
            <Button variant="destructive" onClick={handleReset} disabled={!canReset || busy}>
              {busy ? t('settings.general.resetting') : t('settings.general.resetAllData')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </section>
  );
}
