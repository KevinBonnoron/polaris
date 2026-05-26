import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Avatar, AvatarFallback } from '@/components/ui/avatar';
import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import { Dialog, DialogClose, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import { ResetAllData } from '@/wailsjs/go/main/App';

export function AccountSettings() {
  const { t } = useTranslation();
  const [resetOpen, setResetOpen] = useState(false);
  const [confirmText, setConfirmText] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const confirmWord = t('settings.account.resetConfirmWord');
  const canReset = confirmText.trim().toUpperCase() === confirmWord.toUpperCase();

  const handleReset = async () => {
    setBusy(true);
    setError(null);
    try {
      await ResetAllData();
      setResetOpen(false);
      setConfirmText('');
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <section className="flex flex-col gap-6">
      <h3 className="text-base font-semibold">{t('settings.account.title')}</h3>

      <Card>
        <CardContent className="flex items-center gap-3">
          <Avatar className="size-10">
            <AvatarFallback className="bg-primary text-primary-foreground">JD</AvatarFallback>
          </Avatar>
          <div className="min-w-0 flex-1">
            <div className="text-sm font-medium">John Doe</div>
            <div className="truncate text-xs text-muted-foreground">john.doe@example.com</div>
          </div>
        </CardContent>
      </Card>

      <div>
        <Button variant="destructive">{t('settings.account.signOut')}</Button>
      </div>

      <div className="flex flex-col gap-2">
        <h4 className="text-xs font-medium uppercase tracking-wide text-destructive">{t('settings.account.dangerZone')}</h4>
        <Card className="border-destructive/40">
          <CardContent className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
            <div className="min-w-0">
              <div className="text-sm font-medium">{t('settings.account.resetAllData')}</div>
              <p className="text-xs text-muted-foreground">{t('settings.account.resetAllDataDesc')}</p>
            </div>
            <Button variant="destructive" onClick={() => setResetOpen(true)}>
              {t('settings.account.resetAllData')}
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
              setError(null);
            }
          }
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('settings.account.resetAllData')}</DialogTitle>
            <DialogDescription>{t('settings.account.resetConfirmDesc', { word: confirmWord })}</DialogDescription>
          </DialogHeader>
          <Input value={confirmText} onChange={(e) => setConfirmText(e.target.value)} placeholder={confirmWord} autoFocus />
          {error && <p className="text-xs text-destructive">{error}</p>}
          <DialogFooter>
            <DialogClose asChild>
              <Button variant="outline" disabled={busy}>
                {t('common.cancel')}
              </Button>
            </DialogClose>
            <Button variant="destructive" onClick={handleReset} disabled={!canReset || busy}>
              {busy ? t('settings.account.resetting') : t('settings.account.resetAllData')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </section>
  );
}
