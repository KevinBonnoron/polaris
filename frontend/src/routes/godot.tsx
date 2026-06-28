import { createFileRoute } from '@tanstack/react-router';
import { GodotPage } from '@/features/integrations/godot/godot-page';

export const Route = createFileRoute('/godot')({
  component: GodotPage,
});
