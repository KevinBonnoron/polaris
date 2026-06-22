import { RunPythonCommand, StartPythonScript, StopPythonScript } from '@/wailsjs/go/main/App';
import { createRunContext } from '../create-run-context';
import type { PythonConfig } from './types';

const { Provider, useRun } = createRunContext<PythonConfig>({
  kind: 'python',
  eventPrefix: 'python',
  defaultPm: 'pip',
  i18nPrefix: 'integrations.python',
  startKey: 'startScript',
  fns: {
    start: StartPythonScript,
    run: RunPythonCommand,
    stop: StopPythonScript,
  },
});

export { Provider as PythonRunProvider };
export const usePythonRun = useRun;
