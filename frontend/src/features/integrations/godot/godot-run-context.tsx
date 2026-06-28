import { RunGodotCommand, StartGodotPlay, StopGodotPlay } from '@/wailsjs/go/main/App';
import { createRunContext } from '../create-run-context';
import type { GodotConfig } from './types';

const { Provider, useRun } = createRunContext<GodotConfig>({
  kind: 'godot',
  eventPrefix: 'godot',
  defaultPm: 'play',
  i18nPrefix: 'integrations.godot',
  startKey: 'playCommand',
  fns: {
    start: StartGodotPlay,
    run: RunGodotCommand,
    stop: StopGodotPlay,
  },
});

export { Provider as GodotRunProvider };
export const useGodotRun = useRun;
