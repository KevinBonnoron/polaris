import { Plus, Workflow } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { AutomationEditModal } from './automation-edit-modal';

interface Props {
  projectId: string;
}

export function AutomationsEmpty({ projectId }: Props) {
  const { t } = useTranslation();
  return (
    <Card className="border-dashed">
      <CardHeader className="items-center text-center">
        <div className="mb-3 flex size-12 items-center justify-center rounded-full bg-muted">
          <Workflow className="size-5 text-muted-foreground" />
        </div>
        <CardTitle className="text-base">{t('automations.emptyTitle')}</CardTitle>
        <CardDescription>{t('automations.emptyDesc')}</CardDescription>
      </CardHeader>
      <CardContent className="flex justify-center">
        <AutomationEditModal projectId={projectId}>
          <Button>
            <Plus className="size-3.5" />
            {t('automations.new')}
          </Button>
        </AutomationEditModal>
      </CardContent>
    </Card>
  );
}
