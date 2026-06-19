import type { ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { PageHeader } from '@/components/atoms/page-header';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';

interface Props {
  title: string;
  description?: string;
  icon?: ReactNode;
  children?: ReactNode;
}

export function PagePlaceholder({ title, description, icon, children }: Props) {
  const { t } = useTranslation();
  return (
    <div className="flex h-full flex-col gap-6 p-4">
      <PageHeader icon={icon} title={title} subtitle={description} />
      <Card>
        <CardHeader>
          <CardTitle>{t('shell.placeholder.title')}</CardTitle>
          <CardDescription>{t('shell.placeholder.description')}</CardDescription>
        </CardHeader>
        {children && <CardContent>{children}</CardContent>}
      </Card>
    </div>
  );
}
