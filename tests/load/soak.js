import { setup, postEvent, thresholds, durationEnv, numberEnv } from './common.js';

const rate = numberEnv('SOAK_RATE', 1000);

export const options = {
  scenarios: {
    soak: { executor: 'constant-arrival-rate', rate, timeUnit: '1s', duration: durationEnv('SOAK_DURATION', '1h'), preAllocatedVUs: 1000, maxVUs: 5000 },
  },
  thresholds: thresholds(),
};

export { setup };
export default function () { postEvent('soak'); }
