import { Settings } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { Button } from '@/components/ui/button';

interface Props {
  onClick: () => void;
  label?: string;
}

export function ConfigureAction({ onClick, label }: Props) {
  const { t } = useTranslation();
  return (
    <Button variant="outline" size="sm" onClick={onClick}>
      <Settings className="size-3.5" />
      {label ?? t('common.configure')}
    </Button>
  );
}
