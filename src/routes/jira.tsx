import { createFileRoute } from '@tanstack/react-router';
import { JiraPage } from '@/features/integrations/jira-page';

export const Route = createFileRoute('/jira')({
  component: JiraPage,
});
