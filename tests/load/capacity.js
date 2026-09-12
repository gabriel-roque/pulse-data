import { setup, postEvent, numberEnv, durationEnv, thresholds } from './common.js';

const targetRate = numberEnv('CAPACITY_TARGET_RPS', 100000);
const startRate = numberEnv('CAPACITY_START_RPS', 1000);
const batchSize = numberEnv('CAPACITY_BATCH_SIZE', 1);
const stageDuration = durationEnv('CAPACITY_STAGE_DURATION', '1m');
const warmupDuration = durationEnv('CAPACITY_WARMUP_DURATION', '1m');
const holdDuration = durationEnv('CAPACITY_HOLD_DURATION', '5m');
const direct = __ENV.CAPACITY_DIRECT === 'true';
const requestRate = (eventsPerSecond) => eventsPerSecond / batchSize;

const stages = direct
  ? [
      { target: requestRate(targetRate), duration: warmupDuration },
      { target: requestRate(targetRate), duration: holdDuration },
    ]
  : [
      { target: requestRate(Math.min(1000, targetRate)), duration: stageDuration },
      { target: requestRate(Math.min(10000, targetRate)), duration: stageDuration },
      { target: requestRate(Math.min(25000, targetRate)), duration: stageDuration },
      { target: requestRate(Math.min(50000, targetRate)), duration: stageDuration },
      { target: requestRate(targetRate), duration: stageDuration },
      { target: requestRate(targetRate), duration: holdDuration },
    ];

export const options = {
  discardResponseBodies: false,
  scenarios: {
    capacity: {
      executor: 'ramping-arrival-rate',
      startRate: requestRate(direct ? targetRate : startRate),
      timeUnit: '1s',
      preAllocatedVUs: numberEnv('CAPACITY_PREALLOCATED_VUS', 2000),
      maxVUs: numberEnv('CAPACITY_MAX_VUS', 10000),
      stages,
    },
  },
  thresholds: thresholds(),
};

export { setup };
export default function () { postEvent('capacity', batchSize); }
