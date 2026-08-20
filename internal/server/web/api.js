// Thin wrapper around the go-scan HTTP API. Every call carries the session
// token the server injected into the page.

const TOKEN = window.SCAN_TOKEN;

async function request(method, path, body) {
  const opts = {
    method,
    headers: { 'X-Scan-Token': TOKEN },
  };
  if (body !== undefined) {
    opts.headers['Content-Type'] = 'application/json';
    opts.body = JSON.stringify(body);
  }
  const res = await fetch(path, opts);
  const text = await res.text();

  let payload = text;
  if (res.headers.get('content-type')?.includes('application/json')) {
    payload = text ? JSON.parse(text) : null;
  }
  if (!res.ok) {
    const message = (payload && payload.error) || res.statusText;
    const err = new Error(message);
    err.status = res.status;
    err.payload = payload;
    throw err;
  }
  return payload;
}

export const api = {
  meta: () => request('GET', '/api/meta'),
  validate: (config) => request('POST', '/api/validate', { config }),
  preview: (config, limits = {}) =>
    request('POST', '/api/preview', {
      config,
      max_tickers: limits.tickers ?? 0,
      max_bars: limits.bars ?? 0,
    }),
  scan: (config) => request('POST', '/api/scan', { config }),
  run: (config) => request('POST', '/api/run', { config }),
  cancel: (jobId) => request('POST', `/api/jobs/${jobId}/cancel`),
  loadConfig: (path) => request('GET', `/api/config?path=${encodeURIComponent(path)}`),
  saveConfig: (path, config) => request('PUT', '/api/config', { path, config }),
  yaml: (config) => request('POST', '/api/config/yaml', { config }),
  clearCache: () => request('POST', '/api/cache/clear'),
  downloadURL: (path) => `/api/files?path=${encodeURIComponent(path)}&token=${TOKEN}`,
  // EventSource cannot send headers, so the token rides in the query string.
  events: (jobId) => new EventSource(`/api/jobs/${jobId}/events?token=${TOKEN}`),
};
