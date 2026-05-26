import type { LucideIcon } from 'lucide-react';
import { Folder, Terminal } from 'lucide-react';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Input } from '@/components/ui/input';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { Switch } from '@/components/ui/switch';
import { SUPPORTED_LANGUAGES } from '@/lib/i18n';
import { cn } from '@/lib/utils';
import { DetectIdes, GetGeneralSettings, UpdateGeneralSettings } from '@/wailsjs/go/main/App';
import { polaris } from '@/wailsjs/go/models';
import { SettingsRow } from './settings-row';

interface IdeOption {
  id: string;
  name: string;
  cmd: string;
  color: string;
  alwaysAvailable?: boolean;
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
  { id: 'custom', name: 'Custom...', cmd: '', color: '#1f2937', alwaysAvailable: true, icon: Terminal },
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
  const showDetectionState = !ide.alwaysAvailable;
  return (
    <button type="button" onClick={() => onSelect(ide.id)} className={cn('flex items-center gap-2 rounded-md border p-2 text-left transition-colors', selected ? 'border-primary/60 bg-accent/40' : 'border-border hover:bg-accent/30', showDetectionState && !detected && !selected && 'opacity-60')}>
      <div className="flex size-7 shrink-0 items-center justify-center rounded text-[10px] font-bold text-white" style={{ background: ide.color }}>
        {Icon ? <Icon className="size-3.5" /> : ide.glyph}
      </div>
      <div className="min-w-0 flex-1">
        <div className="truncate text-xs font-medium">{ide.name}</div>
        {showDetectionState && <div className="text-[10px] text-muted-foreground">{detected ? t('settings.general.detected') : t('settings.general.notInstalled')}</div>}
      </div>
    </button>
  );
}

export function GeneralSettings() {
  const { t, i18n } = useTranslation();
  const [ideId, setIdeId] = useState('vscode');
  const [customCmd, setCustomCmd] = useState(IDES.find((i) => i.id === 'vscode')?.cmd ?? '');
  const [detected, setDetected] = useState<Record<string, boolean>>({});
  const [autoResume, setAutoResume] = useState(false);

  useEffect(() => {
    let cancelled = false;
    DetectIdes()
      .then((results) => {
        if (cancelled) {
          return;
        }
        const map: Record<string, boolean> = {};
        for (const r of results) {
          map[r.id] = r.installed;
        }
        setDetected(map);
      })
      .catch(() => {});
    GetGeneralSettings()
      .then((s) => {
        if (!cancelled) {
          setAutoResume(s.autoResumeSessions);
        }
      })
      .catch(() => {});
    return () => {
      cancelled = true;
    };
  }, []);

  const handleAutoResumeChange = (next: boolean) => {
    setAutoResume(next);
    UpdateGeneralSettings(polaris.GeneralSettings.createFrom({ autoResumeSessions: next })).catch(() => {
      setAutoResume(!next);
    });
  };

  const handleSelect = (id: string) => {
    setIdeId(id);
    const next = IDES.find((i) => i.id === id);
    if (next?.cmd) {
      setCustomCmd(next.cmd);
    }
  };

  const currentLang = i18n.resolvedLanguage ?? i18n.language;

  return (
    <section className="flex flex-col gap-6">
      <div className="flex flex-col gap-1">
        <h3 className="text-base font-semibold">{t('settings.general.title')}</h3>
        <p className="text-xs text-muted-foreground">{t('settings.general.subtitle')}</p>
      </div>

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
        <SettingsRow label={t('settings.general.autoSave')} description={t('settings.general.autoSaveDesc')} control={<Switch checked={autoResume} onCheckedChange={handleAutoResumeChange} />} />
      </div>

      <div className="flex flex-col gap-2">
        <div className="flex items-baseline justify-between">
          <h4 className="text-xs font-medium uppercase tracking-wide text-muted-foreground">{t('settings.general.defaultIde')}</h4>
          <span className="text-[11px] text-muted-foreground">{t('settings.general.defaultIdeHint')}</span>
        </div>
        <div className="grid grid-cols-2 gap-2 md:grid-cols-3">
          {IDES.map((ide) => (
            <IdeCard key={ide.id} ide={ide} selected={ideId === ide.id} detected={detected[ide.id] ?? false} onSelect={handleSelect} />
          ))}
        </div>
        <div className="mt-1 flex flex-col gap-1">
          <Input value={customCmd} onChange={(e) => setCustomCmd(e.target.value)} placeholder={'e.g. /usr/local/bin/mate "$FILE" -l $LINE'} className="font-mono text-xs" />
          <p className="text-[11px] text-muted-foreground">
            Tokens: <code>$FILE</code> · <code>$LINE</code> · <code>$COL</code> · <code>$REPO</code> · <code>$BRANCH</code>
          </p>
        </div>
      </div>
    </section>
  );
}
