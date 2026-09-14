import http from 'k6/http';
import { check, fail } from 'k6';
import { Counter, Rate } from 'k6/metrics';

const acceptedEvents = new Counter('accepted_events');
const requestedEvents = new Counter('requested_events');
const eventAcceptance = new Rate('event_acceptance');

const apiURL = (__ENV.PULSE_API_URL || '').replace(/\/$/, '');
const apiKeys = open(__ENV.PULSE_API_KEYS_FILE || '/dev/null')
  .split('\n').map((value) => value.trim()).filter(Boolean);
const targetEventsPerSecond = 100000;
const batchSize = 500;
const requestRate = 200;

export const options = {
  discardResponseBodies: false,
  scenarios: {
    kafkaIngress: {
      executor: 'constant-arrival-rate',
      rate: requestRate,
      timeUnit: '1s',
      duration: '5m',
      preAllocatedVUs: 2000,
      maxVUs: 10000,
    },
  },
  thresholds: {
    accepted_events: ['rate>=99500'],
    checks: ['rate==1'],
    dropped_iterations: ['count==0'],
    event_acceptance: ['rate==1'],
    http_req_duration: [
      'p(95)<1000',
      'p(99)<2000',
    ],
    http_req_failed: ['rate==0'],
  },
};

export function setup() {
  if (!apiURL) {
    throw new Error('PULSE_API_URL is required');
  }
  if (apiKeys.length === 0) {
    throw new Error('PULSE_API_KEYS_FILE must contain at least one API key');
  }
  const response = http.get(`${apiURL}/health/ready`, { tags: { endpoint: 'ready' } });
  if (response.status !== 200) {
    fail(`API readiness check failed with HTTP ${response.status}`);
  }
}

export default function () {
  const events = [];
  for (let index = 0; index < batchSize; index += 1) {
    events.push({
      eventId: `evt-capacity-${__VU}-${__ITER}-${index}`,
      type: 'load.capacity',
      timestamp: new Date().toISOString(),
      payload: { vu: __VU, iteration: __ITER, index },
    });
  }
  const response = http.post(`${apiURL}/v1/events/batch`, JSON.stringify(events), {
    headers: {
      Authorization: `Bearer ${apiKeys[(__VU - 1) % apiKeys.length]}`,
      'Content-Type': 'application/json',
    },
    tags: { endpoint: 'events-batch' },
  });
  requestedEvents.add(batchSize);

  let accepted = response.status === 202 && response.headers['X-Pulse-Durability'] === 'kafka-ack';
  if (accepted) {
    try {
      accepted = JSON.parse(response.body).count === batchSize;
    } catch (_) {
      accepted = false;
    }
  }
  eventAcceptance.add(accepted);
  acceptedEvents.add(accepted ? batchSize : 0);
  check(response, { 'batch accepted by Kafka': () => accepted });
}
