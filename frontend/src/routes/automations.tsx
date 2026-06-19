import { createFileRoute } from '@tanstack/react-router';
import { AutomationsPage } from '@/features/automations/automations-page';

export const Route = createFileRoute('/automations')({
  component: AutomationsPage,
});
