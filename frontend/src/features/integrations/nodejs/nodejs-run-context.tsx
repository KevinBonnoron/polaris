import { RunNodeCommand, StartNodeScript, StopNodeScript } from '@/wailsjs/go/main/App';
import { createRunContext } from '../create-run-context';
import type { NodejsConfig } from './types';

const { Provider, useRun } = createRunContext<NodejsConfig>({
  kind: 'nodejs',
  eventPrefix: 'nodejs',
  defaultPm: 'npm',
  i18nPrefix: 'integrations.nodejs',
  startKey: 'startScript',
  fns: {
    start: StartNodeScript,
    run: RunNodeCommand,
    stop: StopNodeScript,
  },
});

export { Provider as NodejsRunProvider };
export const useNodejsRun = useRun;
