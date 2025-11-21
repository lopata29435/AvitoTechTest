import http from 'k6/http';
import { check, sleep } from 'k6';

export const options = {
  vus: 10,
  duration: '30s',
};

const base = __ENV.BASE_URL || 'http://localhost:8080';

export default function () {
  const tid = __VU;
  const team = {
    team_name: `team-${tid}`,
    members: [
      { user_id: `u${tid}-1`, username: 'A', is_active: true },
      { user_id: `u${tid}-2`, username: 'B', is_active: true },
      { user_id: `u${tid}-3`, username: 'C', is_active: true },
    ],
  };
  http.post(`${base}/team/add`, JSON.stringify(team), { headers: { 'Content-Type': 'application/json' } });

  const pr = {
    pull_request_id: `pr-${tid}-${__ITER}`,
    pull_request_name: 'Load Test',
    author_id: `u${tid}-1`,
  };
  const res = http.post(`${base}/pullRequest/create`, JSON.stringify(pr), { headers: { 'Content-Type': 'application/json' } });
  check(res, { 'pr created': (r) => r.status === 201 || r.status === 409 });

  sleep(0.2);
}
