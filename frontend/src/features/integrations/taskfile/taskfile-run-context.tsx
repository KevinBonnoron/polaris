import { RunTaskfileCommand, StartTaskfileTask, StopTaskfileTask } from '@/wailsjs/go/main/App';
import { createRunContext } from '../create-run-context';
import type { TaskfileConfig } from './types';

const { Provider, useRun } = createRunContext<TaskfileConfig>({
  kind: 'taskfile',
  eventPrefix: 'taskfile',
  defaultPm: 'task',
  i18nPrefix: 'integrations.taskfile',
  fns: {
    start: StartTaskfileTask,
    run: RunTaskfileCommand,
    stop: StopTaskfileTask,
  },
});

export { Provider as TaskfileRunProvider };
export const useTaskfileRun = useRun;
