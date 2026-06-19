import { AlertTriangle } from 'lucide-react';
import { useCallback, useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import { Button } from '@/components/ui/button';
import { BackendStatus } from '@/wailsjs/go/main/App';

const TOAST_ID = 'polaris:backend-health';

interface Status {
  ready: boolean;
  error?: string;
  bindingsAvailable: boolean;
}

async function checkBackend(): Promise<Status> {
  try {
    const status = await BackendStatus();
    return { ready: status.ready, error: status.lastError || undefined, bindingsAvailable: true };
  } catch {
    return { ready: false, bindingsAvailable: false };
  }
}

export function BackendHealthCheck() {
  const { t } = useTranslation();
  const [_status, setStatus] = useState<Status | null>(null);
  const dismissedRef = useRef(false);
  const verifyRef = useRef<() => Promise<void>>(() => Promise.resolve());

  const renderToast = useCallback(
    (s: Status) => {
      let description: string;
      if (s.error) {
        description = t('shell.health.goReports', { error: s.error });
      } else if (!s.bindingsAvailable) {
        description = t('shell.health.unreachableNoBindings');
      } else {
        description = t('shell.health.notBooted');
      }
      toast.error(t('shell.health.title'), {
        id: TOAST_ID,
        description,
        duration: Infinity,
        icon: <AlertTriangle className="size-4" />,
        action: (
          <Button
            size="sm"
            variant="outline"
            onClick={() => {
              dismissedRef.current = false;
              void verifyRef.current();
            }}
          >
            {t('shell.health.retry')}
          </Button>
        ),
        onDismiss: () => {
          dismissedRef.current = true;
        },
      });
    },
    [t],
  );

  const verify = useCallback(async () => {
    const s = await checkBackend();
    setStatus(s);
    if (s.ready) {
      toast.dismiss(TOAST_ID);
      return;
    }
    if (!dismissedRef.current) {
      renderToast(s);
    }
  }, [renderToast]);

  useEffect(() => {
    verifyRef.current = verify;
  }, [verify]);

  useEffect(() => {
    void verify();
    const id = window.setInterval(() => {
      void verify();
    }, 5_000);
    return () => window.clearInterval(id);
  }, [verify]);

  return null;
}
