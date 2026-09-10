import { setup, postEvent, thresholds } from './common.js';

export const options = {
  scenarios: {
    stress: {
      executor: 'ramping-arrival-rate',
      startRate: 1000,
      timeUnit: '1s',
      preAllocatedVUs: 1000,
      maxVUs: 10000,
      stages: [
        { target: 1000, duration: '2m' },
        { target: 10000, duration: '2m' },
        { target: 25000, duration: '2m' },
        { target: 50000, duration: '2m' },
        { target: 75000, duration: '2m' },
        { target: 100000, duration: '5m' },
      ],
    },
  },
  thresholds: thresholds(),
};

export { setup };
export default function () { postEvent('stress'); }
