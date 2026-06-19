import { useParams } from '@tanstack/react-router';
import { useTranslation } from 'react-i18next';
import { PagePlaceholder } from '@/features/shell/page-placeholder';

export function IntegrationPage() {
  const { t } = useTranslation();
  const { integrationId } = useParams({ from: '/integration/$integrationId' });
  return <PagePlaceholder title={integrationId} description={t('integrations.generic.description')} />;
}
