import { Bot } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import type { AgentKind } from '@/types';
import { NewAgentButton } from './new-agent-button';

interface Props {
  onSelect?: (kindId: AgentKind) => void;
}

export function AgentsEmpty({ onSelect }: Props) {
  const { t } = useTranslation();
  return (
    <Card className="border-dashed">
      <CardHeader className="items-center text-center">
        <div className="mb-3 flex size-12 items-center justify-center rounded-full bg-muted">
          <Bot className="size-5 text-muted-foreground" />
        </div>
        <CardTitle className="text-base">{t('agents.empty.title')}</CardTitle>
        <CardDescription>{t('agents.empty.description')}</CardDescription>
      </CardHeader>
      <CardContent className="flex justify-center">
        <NewAgentButton size="default" onSelect={onSelect} />
      </CardContent>
    </Card>
  );
}
