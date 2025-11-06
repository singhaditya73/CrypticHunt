import http from 'k6/http';
import { check, sleep } from 'k6';
import { Rate } from 'k6/metrics';

// Custom metrics
const errorRate = new Rate('errors');

// Test configuration
export const options = {
  stages: [
    { duration: '30s', target: 50 },   // Ramp up to 50 users
    { duration: '1m', target: 100 },    // Ramp up to 100 users
    { duration: '2m', target: 500 },    // Ramp up to 500 users
    { duration: '2m', target: 1000 },   // Ramp up to 1000 users
    { duration: '3m', target: 1000 },   // Stay at 1000 users
    { duration: '1m', target: 0 },      // Ramp down to 0 users
  ],
  thresholds: {
    http_req_duration: ['p(95)<500', 'p(99)<1000'], // 95% of requests under 500ms, 99% under 1s
    http_req_failed: ['rate<0.05'], // Error rate under 5%
    errors: ['rate<0.1'],
  },
};

// Base URL - Update this to your server
const BASE_URL = __ENV.BASE_URL || 'http://localhost:8080';

// Sample credentials - you'll need to create these users beforehand
const USERS = [
  { username: 'testuser1', password: 'password123' },
  { username: 'testuser2', password: 'password123' },
  { username: 'testuser3', password: 'password123' },
  // Add more test users as needed
];

// Session store for each VU
let sessionCookie = '';

export function setup() {
  console.log('Starting load test against:', BASE_URL);
  console.log('Test will simulate up to 1000 concurrent users');
  
  // Health check before starting
  const healthRes = http.get(`${BASE_URL}/api/health`);
  console.log('Initial health check:', healthRes.status);
  
  return { startTime: Date.now() };
}

export default function (data) {
  // Select a random user for this virtual user
  const user = USERS[Math.floor(Math.random() * USERS.length)];
  
  // Login if not already logged in
  if (!sessionCookie) {
    login(user);
  }
  
  // Simulate user behavior
  const scenario = Math.random();
  
  if (scenario < 0.4) {
    // 40% - View hunt page
    viewHuntPage();
  } else if (scenario < 0.7) {
    // 30% - Poll locked questions (simulating the old behavior)
    pollLockedQuestions();
  } else if (scenario < 0.85) {
    // 15% - View question detail
    viewQuestionDetail();
  } else if (scenario < 0.95) {
    // 10% - View leaderboard
    viewLeaderboard();
  } else {
    // 5% - SSE connection simulation
    sseConnection();
  }
  
  // Random sleep between 2-5 seconds to simulate real user behavior
  sleep(Math.random() * 3 + 2);
}

function login(user) {
  const loginPayload = {
    username: user.username,
    password: user.password,
  };
  
  const params = {
    headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
  };
  
  const res = http.post(
    `${BASE_URL}/login`,
    `username=${loginPayload.username}&password=${loginPayload.password}`,
    params
  );
  
  const success = check(res, {
    'login successful': (r) => r.status === 200 || r.status === 302,
  });
  
  if (!success) {
    errorRate.add(1);
  }
  
  // Extract session cookie
  const cookies = res.cookies;
  for (let name in cookies) {
    sessionCookie = `${name}=${cookies[name][0].value}`;
  }
}

function viewHuntPage() {
  const params = {
    headers: { 'Cookie': sessionCookie },
  };
  
  const res = http.get(`${BASE_URL}/hunt`, params);
  
  const success = check(res, {
    'hunt page loaded': (r) => r.status === 200,
    'response time OK': (r) => r.timings.duration < 1000,
  });
  
  if (!success) {
    errorRate.add(1);
  }
}

function pollLockedQuestions() {
  const params = {
    headers: { 
      'Cookie': sessionCookie,
      'If-None-Match': '', // Will be populated with actual ETag in real scenario
    },
  };
  
  const res = http.get(`${BASE_URL}/api/locked-questions`, params);
  
  const success = check(res, {
    'locked questions fetched': (r) => r.status === 200 || r.status === 304,
    'response time fast': (r) => r.timings.duration < 200,
  });
  
  if (!success) {
    errorRate.add(1);
  }
}

function viewQuestionDetail() {
  // Random question ID between 1-10
  const questionId = Math.floor(Math.random() * 10) + 1;
  
  const params = {
    headers: { 'Cookie': sessionCookie },
  };
  
  const res = http.get(`${BASE_URL}/hunt/question/${questionId}`, params);
  
  const success = check(res, {
    'question page loaded': (r) => r.status === 200 || r.status === 403 || r.status === 404,
  });
  
  if (!success) {
    errorRate.add(1);
  }
}

function viewLeaderboard() {
  const params = {
    headers: { 'Cookie': sessionCookie },
  };
  
  const res = http.get(`${BASE_URL}/hunt/leaderboard`, params);
  
  const success = check(res, {
    'leaderboard loaded': (r) => r.status === 200,
    'response time OK': (r) => r.timings.duration < 500,
  });
  
  if (!success) {
    errorRate.add(1);
  }
}

function sseConnection() {
  // SSE connections are long-lived, so we'll just test the initial connection
  const params = {
    headers: { 
      'Cookie': sessionCookie,
      'Accept': 'text/event-stream',
    },
    timeout: '5s', // Short timeout for load testing
  };
  
  const res = http.get(`${BASE_URL}/api/events`, params);
  
  const success = check(res, {
    'SSE connection established': (r) => r.status === 200,
  });
  
  if (!success) {
    errorRate.add(1);
  }
}

export function teardown(data) {
  const duration = (Date.now() - data.startTime) / 1000;
  console.log(`Load test completed in ${duration} seconds`);
  
  // Final health check
  const healthRes = http.get(`${BASE_URL}/api/health`);
  console.log('Final health check:', healthRes.status);
  
  if (healthRes.status === 200) {
    const health = JSON.parse(healthRes.body);
    console.log('Server health:', JSON.stringify(health, null, 2));
  }
}
