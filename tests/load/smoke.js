import { sleep } from 'k6';
import { setup, postEvent, thresholds } from './common.js';

export const options = {
  scenarios: {
    smoke: { executor: 'constant-arrival-rate', rate: 10, timeUnit: '1s', duration: '1m', preAllocatedVUs: 20, maxVUs: 100 },
  },
  thresholds: thresholds(),
};

export { setup };
export default function () { postEvent('smoke'); sleep(0.01); }
