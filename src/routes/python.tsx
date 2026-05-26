import { createFileRoute } from '@tanstack/react-router';
import { PythonPage } from '@/features/integrations/python/python-page';

export const Route = createFileRoute('/python')({
  component: PythonPage,
});
