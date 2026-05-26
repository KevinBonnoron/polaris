import { useTranslation } from 'react-i18next';
import { PagePlaceholder } from '@/features/shell/page-placeholder';

export function DashboardPage() {
  const { t } = useTranslation();
  return <PagePlaceholder title={t('dashboard.title')} description={t('dashboard.description')} />;
}
