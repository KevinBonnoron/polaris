import { CheckCircle2, Download, Loader2, RefreshCw } from 'lucide-react';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import { AppVersion, CheckForUpdate, OpenExternalURL } from '@/wailsjs/go/main/App';
import type { polaris } from '@/wailsjs/go/models';

export function AboutSettings() {
  const { t } = useTranslation();
  const [version, setVersion] = useState<string>('');
  const [info, setInfo] = useState<polaris.UpdateInfo | null>(null);
  const [checking, setChecking] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const runCheck = (force: boolean) => {
    setChecking(true);
    setError(null);
    CheckForUpdate(force)
      .then((result) => setInfo(result))
      .catch((e) => setError(e instanceof Error ? e.message : String(e)))
      .finally(() => setChecking(false));
  };

  useEffect(() => {
    AppVersion()
      .then(setVersion)
      .catch(() => setVersion(''));
    runCheck(false);
  }, []);

  const hasUpdate = info?.hasUpdate ?? false;
  const checkedAt = info?.checkedAt ? new Date(info.checkedAt).toLocaleString() : null;

  return (
    <section className="flex flex-col gap-6">
      <Card>
        <CardContent className="flex flex-col gap-4">
          <div className="flex items-center justify-between gap-3">
            <div className="min-w-0">
              <div className="text-sm font-medium">Polaris</div>
              <div className="text-xs text-muted-foreground">
                {t('settings.about.currentVersion')}: <span className="font-mono">{version || '—'}</span>
              </div>
            </div>
            {info &&
              !checking &&
              (hasUpdate ? (
                <Badge variant="default">{t('settings.about.updateAvailable')}</Badge>
              ) : (
                <Badge variant="secondary">
                  <CheckCircle2 className="mr-1 size-3" />
                  {t('settings.about.upToDate')}
                </Badge>
              ))}
          </div>

          {hasUpdate && info && (
            <div className="flex flex-col gap-2 rounded-md border border-primary/30 bg-primary/5 p-3">
              <div className="text-sm font-medium">{t('settings.about.newVersion', { version: info.latest })}</div>
              {info.releaseNotes && <pre className="max-h-48 overflow-auto whitespace-pre-wrap text-xs text-muted-foreground">{info.releaseNotes}</pre>}
              <div>
                <Button size="sm" onClick={() => OpenExternalURL(info.htmlUrl)}>
                  <Download className="mr-2 size-4" />
                  {t('settings.about.download')}
                </Button>
              </div>
            </div>
          )}

          {error && <p className="text-xs text-destructive">{error}</p>}

          <div className="flex items-center justify-between gap-3">
            <span className="text-xs text-muted-foreground">{checking ? t('settings.about.checking') : checkedAt ? t('settings.about.lastCheckedAt', { when: checkedAt }) : t('settings.about.neverChecked')}</span>
            <Button variant="outline" size="sm" onClick={() => runCheck(true)} disabled={checking}>
              {checking ? <Loader2 className="mr-2 size-4 animate-spin" /> : <RefreshCw className="mr-2 size-4" />}
              {t('settings.about.checkNow')}
            </Button>
          </div>
        </CardContent>
      </Card>
    </section>
  );
}
