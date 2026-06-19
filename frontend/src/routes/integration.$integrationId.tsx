import { createFileRoute } from '@tanstack/react-router';
import { IntegrationPage } from '@/features/integrations/integration-page';

export const Route = createFileRoute('/integration/$integrationId')({
  component: IntegrationPage,
});
