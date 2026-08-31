import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { Skeleton } from '@/components/ui/skeleton';
import { GetNetworkSettings, GetSystemPACUrl, UpdateNetworkSettings } from '@/wailsjs/go/main/App';
import { polaris } from '@/wailsjs/go/models';
import { SettingsRow } from './settings-row';

type ProxyMode = 'none' | 'manual' | 'pac';

export function NetworkSettings() {
  const { t } = useTranslation();
  const [settings, setSettings] = useState<polaris.NetworkSettings | null>(null);
  const [detecting, setDetecting] = useState(false);

  useEffect(() => {
    GetNetworkSettings()
      .then(setSettings)
      .catch(() => setSettings({}));
  }, []);

  function persist(next: polaris.NetworkSettings) {
    setSettings(next);
    UpdateNetworkSettings(next).catch(() => {});
  }

  function handleModeChange(value: string) {
    if (!settings) return;
    persist({ ...settings, mode: value as polaris.ProxyMode });
  }

  function handleField(field: keyof polaris.NetworkSettings, value: string) {
    if (!settings) return;
    persist({ ...settings, [field]: value });
  }

  async function handleAutoDetect() {
    setDetecting(true);
    try {
      const url = await GetSystemPACUrl();
      if (url) {
        persist({ ...settings!, pacUrl: url });
      } else {
        alert(t('settings.network.pacNotFound'));
      }
    } finally {
      setDetecting(false);
    }
  }

  if (!settings) {
    return (
      <div className="flex flex-col gap-3">
        {[1, 2, 3].map((i) => (
          <Skeleton key={i} className="h-12 w-full rounded" />
        ))}
      </div>
    );
  }

  const mode: ProxyMode = (settings.mode as ProxyMode) || 'manual';

  return (
    <div className="flex flex-col">
      <SettingsRow
        label={t('settings.network.mode')}
        description={t('settings.network.modeDesc')}
        control={
          <Select value={mode} onValueChange={handleModeChange}>
            <SelectTrigger className="w-40">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="none">{t('settings.network.modeNone')}</SelectItem>
              <SelectItem value="manual">{t('settings.network.modeManual')}</SelectItem>
              <SelectItem value="pac">{t('settings.network.modePAC')}</SelectItem>
            </SelectContent>
          </Select>
        }
      />

      {mode === 'pac' && (
        <SettingsRow
          label={t('settings.network.pacUrl')}
          description={t('settings.network.pacUrlDesc')}
          control={
            <div className="flex gap-2">
              <Input className="w-64 font-mono text-xs" placeholder="http://proxy.example.com/proxy.pac" value={settings.pacUrl ?? ''} onChange={(e) => handleField('pacUrl', e.target.value)} />
              <Button variant="outline" size="sm" disabled={detecting} onClick={handleAutoDetect}>
                {t('settings.network.pacAutoDetect')}
              </Button>
            </div>
          }
        />
      )}

      {mode === 'manual' && (
        <>
          <SettingsRow label={t('settings.network.httpProxy')} description={t('settings.network.httpProxyDesc')} control={<Input className="w-72 font-mono text-xs" placeholder="http://proxy:8080" value={settings.httpProxy ?? ''} onChange={(e) => handleField('httpProxy', e.target.value)} />} />
          <SettingsRow label={t('settings.network.httpsProxy')} description={t('settings.network.httpsProxyDesc')} control={<Input className="w-72 font-mono text-xs" placeholder="http://proxy:8080" value={settings.httpsProxy ?? ''} onChange={(e) => handleField('httpsProxy', e.target.value)} />} />
          <SettingsRow label={t('settings.network.noProxy')} description={t('settings.network.noProxyDesc')} control={<Input className="w-72 font-mono text-xs" placeholder="localhost,127.0.0.1" value={settings.noProxy ?? ''} onChange={(e) => handleField('noProxy', e.target.value)} />} />
        </>
      )}
    </div>
  );
}
