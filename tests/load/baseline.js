import { setup, postEvent, thresholds } from './common.js';

export const options = {
  scenarios: {
    baseline: { executor: 'constant-arrival-rate', rate: 1000, timeUnit: '1s', duration: '5m', preAllocatedVUs: 500, maxVUs: 2000 },
  },
  thresholds: thresholds(),
};

export { setup };
export default function () { postEvent('baseline'); }
