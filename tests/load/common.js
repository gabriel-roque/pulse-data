import http from 'k6/http';
import { check, fail } from 'k6';

export const apiURL = (__ENV.PULSE_API_URL || __ENV.API_URL || '').replace(/\/$/, '');
export const apiKeys = (__ENV.PULSE_API_KEYS || __ENV.PULSE_API_KEY || __ENV.API_KEY || '')
  .split(',').map((value) => value.trim()).filter(Boolean);

export function numberEnv(name, fallback) {
  const value = Number(__ENV[name] || fallback);
  if (!Number.isFinite(value) || value <= 0) {
    throw new Error(`${name} must be a positive number`);
  }
  return value;
}

export function durationEnv(name, fallback) {
  return __ENV[name] || fallback;
}

export function thresholds() {
  const maxErrorRate = numberEnv('LOAD_MAX_ERROR_RATE_PERCENT', 1) / 100;
  const p95 = numberEnv('LOAD_P95_MS', 250);
  const p99 = numberEnv('LOAD_P99_MS', 1000);
  return {
    http_req_failed: [`rate<${maxErrorRate}`],
    http_req_duration: [`p(95)<${p95}`, `p(99)<${p99}`],
    checks: ['rate>0.99'],
  };
}

export function setup() {
  if (!apiURL) {
    throw new Error('PULSE_API_URL (or API_URL) is required; load tests never default to an unknown service');
  }
  if (apiKeys.length === 0) {
    throw new Error('PULSE_API_KEY (or API_KEY) is required; create a test tenant explicitly before loading');
  }
  const response = http.get(`${apiURL}/health/ready`, { tags: { endpoint: 'ready' } });
  if (response.status !== 200) {
    fail(`API readiness check failed with HTTP ${response.status}; external dependency is unavailable`);
  }
  return { startedAt: new Date().toISOString() };
}

export function postEvent(profile) {
  const id = `evt-load-${profile}-${__VU}-${__ITER}`;
  const body = JSON.stringify({
    eventId: id,
    type: `load.${profile}`,
    timestamp: new Date().toISOString(),
    payload: { profile, vu: __VU, iteration: __ITER },
  });
  const key = apiKeys[(__VU - 1) % apiKeys.length];
  const response = http.post(`${apiURL}/v1/events`, body, {
    headers: { Authorization: `Bearer ${key}`, 'Content-Type': 'application/json' },
    tags: { endpoint: 'events', profile },
  });
  const checks = {
    'event accepted after durability point': (r) => r.status === 202 && r.headers['X-Pulse-Durability'],
  };
  if (__ENV.LOAD_CHECK_RESPONSE_BODY !== 'false') {
    checks['response contains event id'] = (r) => r.body && r.body.includes(id);
  }
  check(response, checks);
}
