import { ExternalLink } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { Button } from '@/components/ui/button';
import { BrowserOpenURL } from '@/wailsjs/runtime/runtime';

interface Props {
  url: string;
  label?: string;
}

export function OpenExternalAction({ url, label }: Props) {
  const { t } = useTranslation();
  return (
    <Button variant="outline" size="sm" onClick={() => BrowserOpenURL(url)}>
      {label ?? t('common.open')}
      <ExternalLink className="size-3.5" />
    </Button>
  );
}
