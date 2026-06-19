import { createFileRoute } from '@tanstack/react-router';
import { CSharpPage } from '@/features/integrations/csharp/csharp-page';

export const Route = createFileRoute('/csharp')({
  component: CSharpPage,
});
