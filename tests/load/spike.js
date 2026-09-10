import { setup, postEvent, thresholds } from './common.js';

export const options = {
  scenarios: {
    spike: {
      executor: 'ramping-arrival-rate',
      startRate: 100,
      timeUnit: '1s',
      preAllocatedVUs: 500,
      maxVUs: 5000,
      stages: [
        { target: 100, duration: '30s' },
        { target: 10000, duration: '15s' },
        { target: 10000, duration: '1m' },
        { target: 100, duration: '15s' },
        { target: 100, duration: '1m' },
      ],
    },
  },
  thresholds: thresholds(),
};

export { setup };
export default function () { postEvent('spike'); }
