import { createFileRoute } from '@tanstack/react-router';
import { TicketsPage } from '@/features/integrations/tickets-page';

export const Route = createFileRoute('/tickets')({
  component: TicketsPage,
});
