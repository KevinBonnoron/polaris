import { createFileRoute } from '@tanstack/react-router';
import { NodejsPage } from '@/features/integrations/nodejs/nodejs-page';

export const Route = createFileRoute('/nodejs')({
  component: NodejsPage,
});
