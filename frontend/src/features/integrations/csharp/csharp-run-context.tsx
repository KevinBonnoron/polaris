import { RunCSharpCommand, StartCSharpScript, StopCSharpScript } from '@/wailsjs/go/main/App';
import { createRunContext } from '../create-run-context';
import type { CSharpConfig } from './types';

const { Provider, useRun } = createRunContext<CSharpConfig>({
  kind: 'csharp',
  eventPrefix: 'csharp',
  defaultPm: 'dotnet',
  i18nPrefix: 'integrations.csharp',
  startKey: 'startScript',
  fns: {
    start: StartCSharpScript,
    run: RunCSharpCommand,
    stop: StopCSharpScript,
  },
});

export { Provider as CSharpRunProvider };
export const useCSharpRun = useRun;
