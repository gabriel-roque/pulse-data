import { setup, postEvent, numberEnv, durationEnv, thresholds } from './common.js';

const targetRate = numberEnv('CAPACITY_TARGET_RPS', 100000);
const startRate = numberEnv('CAPACITY_START_RPS', 1000);
const stageDuration = durationEnv('CAPACITY_STAGE_DURATION', '1m');
const holdDuration = durationEnv('CAPACITY_HOLD_DURATION', '5m');

export const options = {
  discardResponseBodies: true,
  scenarios: {
    capacity: {
      executor: 'ramping-arrival-rate',
      startRate,
      timeUnit: '1s',
      preAllocatedVUs: numberEnv('CAPACITY_PREALLOCATED_VUS', 2000),
      maxVUs: numberEnv('CAPACITY_MAX_VUS', 10000),
      stages: [
        { target: Math.min(1000, targetRate), duration: stageDuration },
        { target: Math.min(10000, targetRate), duration: stageDuration },
        { target: Math.min(25000, targetRate), duration: stageDuration },
        { target: Math.min(50000, targetRate), duration: stageDuration },
        { target: targetRate, duration: stageDuration },
        { target: targetRate, duration: holdDuration },
      ],
    },
  },
  thresholds: thresholds(),
};

export { setup };
export default function () { postEvent('capacity'); }
