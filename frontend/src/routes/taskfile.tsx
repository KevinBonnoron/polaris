import { createFileRoute } from '@tanstack/react-router';
import { TaskfilePage } from '@/features/integrations/taskfile/taskfile-page';

export const Route = createFileRoute('/taskfile')({
  component: TaskfilePage,
});
