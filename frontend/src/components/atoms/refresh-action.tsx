import { RefreshCw } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { Button } from '@/components/ui/button';
import { cn } from '@/lib/utils';

interface Props {
  onRefresh: () => void;
  loading?: boolean;
  disabled?: boolean;
  label?: string;
}

export function RefreshAction({ onRefresh, loading = false, disabled = false, label }: Props) {
  const { t } = useTranslation();
  return (
    <Button variant="outline" size="sm" onClick={onRefresh} disabled={loading || disabled}>
      <RefreshCw className={cn('size-3.5', loading && 'animate-spin')} />
      {label ?? t('common.refresh')}
    </Button>
  );
}
