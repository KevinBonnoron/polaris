import { useTranslation } from 'react-i18next';
import { Card, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';

interface Props {
  provider?: string;
}

export function ProviderNotice({ provider }: Props) {
  const { t } = useTranslation();
  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">{t('integrations.repository.notSupported')}</CardTitle>
        <CardDescription>{provider === 'gitlab' ? t('integrations.repository.gitlabNotice') : t('integrations.repository.githubOnlyNotice', { provider: provider ?? 'unknown' })}</CardDescription>
      </CardHeader>
    </Card>
  );
}
