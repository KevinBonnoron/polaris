import { createFileRoute } from '@tanstack/react-router';
import { MessagingPage } from '@/features/integrations/messaging/messaging-page';

export const Route = createFileRoute('/slack')({
  component: () => <MessagingPage integrationId="slack" />,
});
