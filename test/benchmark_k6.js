import http from 'k6/http';
import exec from 'k6/execution';
import { check, fail, sleep } from 'k6';
import { Counter, Rate, Trend } from 'k6/metrics';
import { SharedArray } from 'k6/data';

function tryOpen(path) {
  try {
    return open(path);
  } catch (err) {
    return null;
  }
}

function stripWrappingQuotes(value) {
  if (!value) {
    return value;
  }

  if (
    (value.startsWith('"') && value.endsWith('"')) ||
    (value.startsWith("'") && value.endsWith("'"))
  ) {
    return value.slice(1, -1);
  }

  return value;
}

function loadDotEnv() {
  const candidates = ['../.env', '.env'];

  for (let i = 0; i < candidates.length; i += 1) {
    const content = tryOpen(candidates[i]);
    if (content === null) {
      continue;
    }

    const loaded = {};
    const lines = content.split(/\r?\n/);
    for (let j = 0; j < lines.length; j += 1) {
      const line = lines[j].trim();
      if (!line || line.startsWith('#')) {
        continue;
      }

      const idx = line.indexOf('=');
      if (idx <= 0) {
        continue;
      }

      const key = line.slice(0, idx).trim();
      const value = stripWrappingQuotes(line.slice(idx + 1).trim());
      if (!key) {
        continue;
      }

      loaded[key] = value;
    }

    return loaded;
  }

  return {};
}

const DOTENV = loadDotEnv();

function readEnv(key, fallback) {
  const envValue = __ENV[key];
  if (envValue !== undefined && String(envValue).trim() !== '') {
    return envValue;
  }

  const dotEnvValue = DOTENV[key];
  if (dotEnvValue !== undefined && String(dotEnvValue).trim() !== '') {
    return dotEnvValue;
  }

  return fallback;
}

const BASE_URL = readEnv('BASE_URL', 'http://localhost:18080').replace(/\/+$/, '');
const ADMIN_USERNAME = String(readEnv('ADMIN_USERNAME', '')).trim();
const ADMIN_PASSWORD = String(readEnv('ADMIN_PASSWORD', '')).trim();
const ACTIVITY_ID = Number(readEnv('ACTIVITY_ID', 1));
const GOODS_ID = Number(readEnv('GOODS_ID', 1));
const QUANTITY = Number(readEnv('QUANTITY', 1));
const EXECUTOR_MODE = String(readEnv('EXECUTOR_MODE', 'closed-loop')).trim().toLowerCase();
const TARGET_QPS = Number(readEnv('TARGET_QPS', 20000));
const DURATION = Number(readEnv('DURATION', 30));
const PRE_ALLOCATED_VUS = Number(readEnv('PREALLOCATED_VUS', 500));
const MAX_VUS = Number(readEnv('MAX_VUS', Math.max(PRE_ALLOCATED_VUS, TARGET_QPS > 0 ? TARGET_QPS : PRE_ALLOCATED_VUS)));
const CONSTANT_VUS = Number(readEnv('VUS', PRE_ALLOCATED_VUS));
const MAX_USERS = Number(readEnv('MAX_USERS', 1000000));
const TOKEN_FILE = String(readEnv('TOKEN_FILE', 'tokens.txt'));
const HTTP_TIMEOUT = String(readEnv('HTTP_TIMEOUT', '30s'));
const REPORT_PREFIX = String(readEnv('REPORT_PREFIX', 'benchmark_k6')).trim() || 'benchmark_k6';
const CLOSED_LOOP_THROTTLE = String(readEnv('CLOSED_LOOP_THROTTLE', 'false')).trim().toLowerCase() === 'true';
const CLOSED_LOOP_PACE_MS = Number(
  readEnv(
    'PACE_MS',
    TARGET_QPS > 0 && CONSTANT_VUS > 0 ? (CONSTANT_VUS * 1000) / TARGET_QPS : 0,
  ),
);

const successCount = new Counter('success_count');
const http200Count = new Counter('http_200_count');
const soldOutCount = new Counter('sold_out_count');
const limitExceedCount = new Counter('limit_exceed_count');
const rateLimitedCount = new Counter('rate_limited_count');
const businessFailedCount = new Counter('business_failed_count');
const decodeFailedCount = new Counter('decode_failed_count');
const non200StatusCount = new Counter('non_200_status_count');
const transportFailedCount = new Counter('transport_failed_count');
const requestDurationMs = new Trend('request_duration_ms');
const responseDecodedRate = new Rate('response_decoded_rate');
const httpSuccessRate = new Rate('http_success_rate');
const businessSuccessRate = new Rate('business_success_rate');

const tokens = new SharedArray('tokens', function () {
  const lines = open(TOKEN_FILE).split(/\r?\n/);
  const loaded = [];

  for (let i = 0; i < lines.length; i += 1) {
    const token = lines[i].trim();
    if (!token) {
      continue;
    }
    loaded.push(token);
    if (MAX_USERS > 0 && loaded.length >= MAX_USERS) {
      break;
    }
  }

  if (loaded.length === 0) {
    throw new Error(`token 文件为空: ${TOKEN_FILE}`);
  }

  return loaded;
});

function buildScenario() {
  if (EXECUTOR_MODE === 'arrival-rate' && TARGET_QPS > 0) {
    return {
      executor: 'constant-arrival-rate',
      rate: TARGET_QPS,
      timeUnit: '1s',
      duration: `${DURATION}s`,
      preAllocatedVUs: PRE_ALLOCATED_VUS,
      maxVUs: MAX_VUS,
    };
  }

  return {
    executor: 'constant-vus',
    vus: CONSTANT_VUS,
    duration: `${DURATION}s`,
  };
}

export const options = {
  summaryTrendStats: ['avg', 'min', 'med', 'max', 'p(95)', 'p(99)'],
  thresholds: {
    http_req_failed: ['rate<0.01'],
    response_decoded_rate: ['rate>0.99'],
  },
  scenarios: {
    seckill_buy: buildScenario(),
  },
};

function buildJSONHeaders(extraHeaders) {
  const headers = {
    'Content-Type': 'application/json',
  };

  if (extraHeaders) {
    const keys = Object.keys(extraHeaders);
    for (let i = 0; i < keys.length; i += 1) {
      headers[keys[i]] = extraHeaders[keys[i]];
    }
  }

  return headers;
}

function parseJSONResponse(res) {
  try {
    return res.json();
  } catch (err) {
    return null;
  }
}

function warmUp() {
  if (!ADMIN_USERNAME || !ADMIN_PASSWORD) {
    fail('缺少 ADMIN_USERNAME 或 ADMIN_PASSWORD 环境变量');
  }

  const loginRes = http.post(`${BASE_URL}/api/admin/login`, JSON.stringify({
    username: ADMIN_USERNAME,
    password: ADMIN_PASSWORD,
  }), {
    headers: buildJSONHeaders(),
    timeout: HTTP_TIMEOUT,
    tags: { name: 'admin_login' },
  });

  const loginBody = parseJSONResponse(loginRes);
  const loginOK = check(loginRes, {
    'admin login status is 200': (r) => r.status === 200,
    'admin login body is json': () => loginBody !== null,
    'admin login code is 0': () => loginBody !== null && loginBody.code === 0,
    'admin login token exists': () => Boolean(loginBody?.data?.token),
  });
  if (!loginOK) {
    fail(`管理员登录失败: status=${loginRes.status}, body=${loginRes.body}`);
  }

  const res = http.post(`${BASE_URL}/api/admin/warmup`, '', {
    headers: {
      Authorization: `Bearer ${loginBody.data.token}`,
    },
    timeout: HTTP_TIMEOUT,
    tags: { name: 'warmup' },
  });

  const body = parseJSONResponse(res);
  const ok = check(res, {
    'warmup status is 200': (r) => r.status === 200,
    'warmup body is json': () => body !== null,
    'warmup code is 0': () => body !== null && body.code === 0,
  });

  if (!ok) {
    fail(`预热失败: status=${res.status}, body=${res.body}`);
  }
}

function resolveActivityID() {
  if (ACTIVITY_ID > 0) {
    return ACTIVITY_ID;
  }

  const res = http.get(`${BASE_URL}/api/activities`, {
    timeout: HTTP_TIMEOUT,
    tags: { name: 'activities' },
  });
  const body = parseJSONResponse(res);
  if (res.status !== 200 || body === null || body.code !== 0 || !Array.isArray(body.data) || body.data.length === 0) {
    return 0;
  }
  return Number(body.data[0].id || 0);
}

function getStock(activityID) {
  const url = activityID > 0
    ? `${BASE_URL}/api/seckill/activity/${activityID}/stock`
    : `${BASE_URL}/api/seckill/stock/${GOODS_ID}`;
  const res = http.get(url, {
    timeout: HTTP_TIMEOUT,
    tags: { name: 'stock' },
  });
  const body = parseJSONResponse(res);
  if (res.status !== 200 || body === null || !body.data) {
    return null;
  }
  return body.data.stock;
}

function pickToken() {
  const iteration = exec.scenario.iterationInTest;
  return tokens[iteration % tokens.length];
}

export function setup() {
  warmUp();
  const activityID = resolveActivityID();
  const initialStock = getStock(activityID);
  console.log(
    `[setup] token数=${tokens.length}, activity_id=${activityID || 'fallback-goods'}, goods_id=${GOODS_ID}, quantity=${QUANTITY}, 初始库存=${initialStock === null ? 'unknown' : initialStock}`,
  );
  return {
    activityID: activityID,
    initialStock: initialStock,
  };
}

export default function (data) {
  const startedAt = Date.now();
  const token = pickToken();
  const activityID = Number(data?.activityID || 0);
  const payload = activityID > 0
    ? JSON.stringify({ activity_id: activityID, quantity: QUANTITY })
    : JSON.stringify({ goods_id: GOODS_ID, quantity: QUANTITY });

  const res = http.post(`${BASE_URL}/api/seckill/buy`, payload, {
    headers: buildJSONHeaders({
      Authorization: `Bearer ${token}`,
    }),
    timeout: HTTP_TIMEOUT,
    tags: { name: 'seckill_buy' },
  });

  requestDurationMs.add(res.timings.duration);

  if (res.error) {
    transportFailedCount.add(1);
    httpSuccessRate.add(false);
    businessSuccessRate.add(false);
    if (EXECUTOR_MODE !== 'arrival-rate' && CLOSED_LOOP_THROTTLE && CLOSED_LOOP_PACE_MS > 0) {
      const elapsedMs = Date.now() - startedAt;
      const remainingMs = CLOSED_LOOP_PACE_MS - elapsedMs;
      if (remainingMs > 0) {
        sleep(remainingMs / 1000);
      }
    }
    return;
  }

  const httpOK = res.status === 200;
  httpSuccessRate.add(httpOK);
  if (httpOK) {
    http200Count.add(1);
  } else {
    non200StatusCount.add(1);
  }

  const body = parseJSONResponse(res);
  const decoded = body !== null;
  responseDecodedRate.add(decoded);

  if (!decoded) {
    decodeFailedCount.add(1);
    businessSuccessRate.add(false);
    check(res, {
      'buy response is json': () => false,
    });
    if (EXECUTOR_MODE !== 'arrival-rate' && CLOSED_LOOP_THROTTLE && CLOSED_LOOP_PACE_MS > 0) {
      const elapsedMs = Date.now() - startedAt;
      const remainingMs = CLOSED_LOOP_PACE_MS - elapsedMs;
      if (remainingMs > 0) {
        sleep(remainingMs / 1000);
      }
    }
    return;
  }

  check(res, {
    'buy status is expected': (r) => r.status === 200 || r.status === 429,
    'buy code exists': () => typeof body.code === 'number',
  });

  switch (body.code) {
    case 0:
      successCount.add(1);
      businessSuccessRate.add(true);
      break;
    case 1001:
      soldOutCount.add(1);
      businessSuccessRate.add(false);
      break;
    case 1002:
      limitExceedCount.add(1);
      businessSuccessRate.add(false);
      break;
    case 429:
      rateLimitedCount.add(1);
      businessSuccessRate.add(false);
      break;
    default:
      businessFailedCount.add(1);
      businessSuccessRate.add(false);
      break;
  }

  if (EXECUTOR_MODE !== 'arrival-rate' && CLOSED_LOOP_THROTTLE && CLOSED_LOOP_PACE_MS > 0) {
    const elapsedMs = Date.now() - startedAt;
    const remainingMs = CLOSED_LOOP_PACE_MS - elapsedMs;
    if (remainingMs > 0) {
      sleep(remainingMs / 1000);
    }
  }
}

export function teardown(data) {
  const finalStock = getStock(Number(data?.activityID || 0));
  console.log(
    `[teardown] 初始库存=${data && data.initialStock !== undefined ? data.initialStock : 'unknown'}, 剩余库存=${finalStock === null ? 'unknown' : finalStock}`,
  );
}

function metricCount(data, name) {
  const metric = data.metrics[name];
  if (!metric || !metric.values || metric.values.count === undefined) {
    return 0;
  }
  return metric.values.count;
}

function metricRate(data, name) {
  const metric = data.metrics[name];
  if (!metric || !metric.values || metric.values.rate === undefined) {
    return 0;
  }
  return metric.values.rate;
}

function metricTrend(data, name, stat) {
  const metric = data.metrics[name];
  if (!metric || !metric.values || metric.values[stat] === undefined) {
    return 0;
  }
  return metric.values[stat];
}

function round(value) {
  return Number(value || 0).toFixed(2);
}

function percent(value) {
  return `${round((value || 0) * 100)}%`;
}

function buildTextSummary(data) {
  const totalRequests = metricCount(data, 'http_reqs');
  const http200 = metricCount(data, 'http_200_count');
  const orderSuccess = metricCount(data, 'success_count');
  const soldOut = metricCount(data, 'sold_out_count');
  const limitExceed = metricCount(data, 'limit_exceed_count');
  const rateLimited = metricCount(data, 'rate_limited_count');
  const businessFailed = metricCount(data, 'business_failed_count');
  const decodeFailed = metricCount(data, 'decode_failed_count');
  const non200 = metricCount(data, 'non_200_status_count');
  const transportFailed = metricCount(data, 'transport_failed_count');
  const avgLatency = metricTrend(data, 'request_duration_ms', 'avg');
  const p50Latency = metricTrend(data, 'request_duration_ms', 'med');
  const p95Latency = metricTrend(data, 'request_duration_ms', 'p(95)');
  const p99Latency = metricTrend(data, 'request_duration_ms', 'p(99)');
  const httpRate = metricRate(data, 'http_success_rate');
  const orderSuccessRate = metricRate(data, 'business_success_rate');
  const decodeRate = metricRate(data, 'response_decoded_rate');
  const systemQPS = DURATION > 0 ? totalRequests / DURATION : 0;
  const http200QPS = DURATION > 0 ? http200 / DURATION : 0;
  const orderSuccessQPS = DURATION > 0 ? orderSuccess / DURATION : 0;

  return [
    '',
    '=== k6 秒杀压测结果 ===',
    `BASE_URL:        ${BASE_URL}`,
    `TOKEN_FILE:      ${TOKEN_FILE}`,
    `tokens:          ${tokens.length}`,
    `duration:        ${DURATION}s`,
    `executor:        ${EXECUTOR_MODE === 'arrival-rate' && TARGET_QPS > 0 ? 'constant-arrival-rate' : 'closed-loop constant-vus'}`,
    `targetQPS:       ${TARGET_QPS > 0 ? TARGET_QPS : 'unlimited'}`,
    `vus:             ${CONSTANT_VUS}`,
    `closedLoopThrottle: ${CLOSED_LOOP_THROTTLE}`,
    `paceMs:          ${CLOSED_LOOP_THROTTLE && CLOSED_LOOP_PACE_MS > 0 ? round(CLOSED_LOOP_PACE_MS) : 'disabled'}`,
    `preAllocatedVUs: ${EXECUTOR_MODE === 'arrival-rate' && TARGET_QPS > 0 ? PRE_ALLOCATED_VUS : CONSTANT_VUS}`,
    `maxVUs:          ${EXECUTOR_MODE === 'arrival-rate' && TARGET_QPS > 0 ? MAX_VUS : CONSTANT_VUS}`,
    '',
    `总请求数:        ${totalRequests}`,
    `HTTP 200响应:    ${http200}`,
    `下单成功:        ${orderSuccess}`,
    `已售罄:          ${soldOut}`,
    `超过限购:        ${limitExceed}`,
    `触发限流:        ${rateLimited}`,
    `业务失败:        ${businessFailed}`,
    `非200响应:       ${non200}`,
    `请求未到系统:    ${transportFailed}`,
    `响应解码失败:    ${decodeFailed}`,
    '',
    `系统QPS:         ${round(systemQPS)} req/s`,
    `HTTP 200 QPS:    ${round(http200QPS)} req/s`,
    `下单成功QPS:     ${round(orderSuccessQPS)} req/s`,
    `HTTP成功率:      ${percent(httpRate)}`,
    `下单成功率:      ${percent(orderSuccessRate)}`,
    `解码成功率:      ${percent(decodeRate)}`,
    '',
    `平均延迟:        ${round(avgLatency)} ms`,
    `P50延迟:         ${round(p50Latency)} ms`,
    `P95延迟:         ${round(p95Latency)} ms`,
    `P99延迟:         ${round(p99Latency)} ms`,
    '',
  ].join('\n');
}

function buildHTMLSummary(data) {
  const totalRequests = metricCount(data, 'http_reqs');
  const http200 = metricCount(data, 'http_200_count');
  const orderSuccess = metricCount(data, 'success_count');
  const soldOut = metricCount(data, 'sold_out_count');
  const limitExceed = metricCount(data, 'limit_exceed_count');
  const rateLimited = metricCount(data, 'rate_limited_count');
  const businessFailed = metricCount(data, 'business_failed_count');
  const decodeFailed = metricCount(data, 'decode_failed_count');
  const non200 = metricCount(data, 'non_200_status_count');
  const transportFailed = metricCount(data, 'transport_failed_count');
  const avgLatency = metricTrend(data, 'request_duration_ms', 'avg');
  const p50Latency = metricTrend(data, 'request_duration_ms', 'med');
  const p95Latency = metricTrend(data, 'request_duration_ms', 'p(95)');
  const p99Latency = metricTrend(data, 'request_duration_ms', 'p(99)');
  const httpRate = metricRate(data, 'http_success_rate');
  const orderSuccessRate = metricRate(data, 'business_success_rate');
  const decodeRate = metricRate(data, 'response_decoded_rate');
  const systemQPS = DURATION > 0 ? totalRequests / DURATION : 0;
  const http200QPS = DURATION > 0 ? http200 / DURATION : 0;
  const orderSuccessQPS = DURATION > 0 ? orderSuccess / DURATION : 0;
  const generatedAt = new Date().toISOString();

  return `<!DOCTYPE html>
<html lang="zh-CN">
<head>
  <meta charset="UTF-8">
  <title>k6 秒杀压测报告</title>
  <style>
    body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; margin: 24px; background: #f5f7fb; color: #17202a; }
    .container { max-width: 980px; margin: 0 auto; background: #fff; border-radius: 12px; padding: 28px; box-shadow: 0 12px 36px rgba(23, 32, 42, 0.08); }
    h1 { margin: 0 0 8px; font-size: 28px; }
    .meta { color: #5b6574; margin-bottom: 24px; }
    .grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(220px, 1fr)); gap: 16px; margin-bottom: 24px; }
    .card { padding: 18px; border-radius: 10px; background: linear-gradient(135deg, #f9fbff, #edf3ff); border: 1px solid #d8e4ff; }
    .label { font-size: 13px; color: #5b6574; margin-bottom: 6px; }
    .value { font-size: 30px; font-weight: 700; color: #183153; }
    table { width: 100%; border-collapse: collapse; margin-top: 8px; }
    th, td { padding: 12px; border-bottom: 1px solid #e8edf5; text-align: left; }
    th { color: #5b6574; font-weight: 600; width: 220px; }
  </style>
</head>
<body>
  <div class="container">
    <h1>k6 秒杀压测报告</h1>
    <div class="meta">生成时间: ${generatedAt}</div>
    <div class="grid">
      <div class="card"><div class="label">系统QPS</div><div class="value">${round(systemQPS)}</div></div>
      <div class="card"><div class="label">HTTP 200 QPS</div><div class="value">${round(http200QPS)}</div></div>
      <div class="card"><div class="label">平均延迟(ms)</div><div class="value">${round(avgLatency)}</div></div>
      <div class="card"><div class="label">P99延迟(ms)</div><div class="value">${round(p99Latency)}</div></div>
    </div>
    <table>
      <tr><th>BASE_URL</th><td>${BASE_URL}</td></tr>
      <tr><th>TOKEN_FILE</th><td>${TOKEN_FILE}</td></tr>
      <tr><th>token数</th><td>${tokens.length}</td></tr>
      <tr><th>执行器</th><td>${EXECUTOR_MODE === 'arrival-rate' && TARGET_QPS > 0 ? 'constant-arrival-rate' : 'closed-loop constant-vus'}</td></tr>
      <tr><th>duration</th><td>${DURATION}s</td></tr>
      <tr><th>targetQPS</th><td>${TARGET_QPS > 0 ? TARGET_QPS : 'unlimited'}</td></tr>
      <tr><th>vus</th><td>${CONSTANT_VUS}</td></tr>
      <tr><th>closedLoopThrottle</th><td>${CLOSED_LOOP_THROTTLE}</td></tr>
      <tr><th>paceMs</th><td>${CLOSED_LOOP_THROTTLE && CLOSED_LOOP_PACE_MS > 0 ? round(CLOSED_LOOP_PACE_MS) : 'disabled'}</td></tr>
      <tr><th>总请求数</th><td>${totalRequests}</td></tr>
      <tr><th>HTTP 200响应</th><td>${http200}</td></tr>
      <tr><th>下单成功</th><td>${orderSuccess}</td></tr>
      <tr><th>已售罄</th><td>${soldOut}</td></tr>
      <tr><th>超过限购</th><td>${limitExceed}</td></tr>
      <tr><th>触发限流</th><td>${rateLimited}</td></tr>
      <tr><th>业务失败</th><td>${businessFailed}</td></tr>
      <tr><th>非200响应</th><td>${non200}</td></tr>
      <tr><th>请求未到系统</th><td>${transportFailed}</td></tr>
      <tr><th>响应解码失败</th><td>${decodeFailed}</td></tr>
      <tr><th>HTTP 200 QPS</th><td>${round(http200QPS)} req/s</td></tr>
      <tr><th>下单成功QPS</th><td>${round(orderSuccessQPS)} req/s</td></tr>
      <tr><th>HTTP成功率</th><td>${percent(httpRate)}</td></tr>
      <tr><th>下单成功率</th><td>${percent(orderSuccessRate)}</td></tr>
      <tr><th>解码成功率</th><td>${percent(decodeRate)}</td></tr>
      <tr><th>平均延迟</th><td>${round(avgLatency)} ms</td></tr>
      <tr><th>P50延迟</th><td>${round(p50Latency)} ms</td></tr>
      <tr><th>P95延迟</th><td>${round(p95Latency)} ms</td></tr>
      <tr><th>P99延迟</th><td>${round(p99Latency)} ms</td></tr>
    </table>
  </div>
</body>
</html>`;
}

export function handleSummary(data) {
  const timestamp = new Date().toISOString().replace(/[:.]/g, '-');
  const baseName = `${REPORT_PREFIX}_${timestamp}`;
  const summary = {
    config: {
      baseURL: BASE_URL,
      activityID: ACTIVITY_ID,
      goodsID: GOODS_ID,
      quantity: QUANTITY,
      durationSec: DURATION,
      executorMode: EXECUTOR_MODE,
      targetQPS: TARGET_QPS,
      closedLoopThrottle: CLOSED_LOOP_THROTTLE,
      paceMs: CLOSED_LOOP_THROTTLE ? CLOSED_LOOP_PACE_MS : 0,
      vus: CONSTANT_VUS,
      preAllocatedVUs: PRE_ALLOCATED_VUS,
      maxVUs: MAX_VUS,
      tokenFile: TOKEN_FILE,
      tokenCount: tokens.length,
    },
    metrics: {
      totalRequests: metricCount(data, 'http_reqs'),
      successRequests: metricCount(data, 'http_200_count'),
      orderSuccessRequests: metricCount(data, 'success_count'),
      soldOut: metricCount(data, 'sold_out_count'),
      limitExceed: metricCount(data, 'limit_exceed_count'),
      rateLimited: metricCount(data, 'rate_limited_count'),
      businessFailed: metricCount(data, 'business_failed_count'),
      decodeFailed: metricCount(data, 'decode_failed_count'),
      non200Status: metricCount(data, 'non_200_status_count'),
      transportFailed: metricCount(data, 'transport_failed_count'),
      httpSuccessRate: metricRate(data, 'http_success_rate'),
      businessSuccessRate: metricRate(data, 'business_success_rate'),
      responseDecodedRate: metricRate(data, 'response_decoded_rate'),
      avgLatencyMs: metricTrend(data, 'request_duration_ms', 'avg'),
      p50LatencyMs: metricTrend(data, 'request_duration_ms', 'med'),
      p95LatencyMs: metricTrend(data, 'request_duration_ms', 'p(95)'),
      p99LatencyMs: metricTrend(data, 'request_duration_ms', 'p(99)'),
    },
  };

  return {
    stdout: buildTextSummary(data),
    [`${baseName}.json`]: JSON.stringify(summary, null, 2),
    [`${baseName}.html`]: buildHTMLSummary(data),
  };
}
