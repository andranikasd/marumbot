// A small load profile for the local stack. It is not a benchmark: it exists
// so the dashboards have something to show and so the burst behaviour the
// design cares about can be exercised on a laptop.
import http from 'k6/http';
import { check, sleep } from 'k6';

export const options = {
  scenarios: {
    steady: { executor: 'constant-vus', vus: 5, duration: '30s' },
    burst: {
      executor: 'ramping-vus', startTime: '30s', startVUs: 0,
      stages: [{ duration: '10s', target: 40 }, { duration: '20s', target: 40 }, { duration: '5s', target: 0 }],
    },
  },
  thresholds: {
    http_req_failed: ['rate<0.01'],
    http_req_duration: ['p(95)<2000'],
  },
};

const base = __ENV.TARGET || 'http://marum:8080';

export default function () {
  check(http.get(`${base}/healthz`), { 'healthz 200': (r) => r.status === 200 });
  check(http.get(`${base}/readyz`), { 'readyz 200': (r) => r.status === 200 });
  check(http.get(`${base}/status`), { 'status 200': (r) => r.status === 200 });
  sleep(1);
}
