// go-scan web UI: builds a config, previews the data it produces, screens a
// universe with the filter and runs the real thing.

import { api } from './api.js';
import { Grid, el } from './grid.js';
import { FunctionHelp } from './funcs.js';

const STORAGE_KEY = 'go-scan.config';
const VALIDATE_DELAY = 300;
const PREVIEW_DELAY = 700;

const $ = (id) => document.getElementById(id);
const form = $('config-form');

let meta = null;
let config = null;
let help = null;
let columns = []; // [{name, expr}] in editor order
let sentColumnIndex = []; // sent column index -> editor row index
let previewGrid = null;
let scanGrid = null;
let scanResult = null;
let currentJob = null;

// ─────────────────────────────── boot ────────────────────────────────────

init().catch((err) => setStatus(err.message, 'error'));

async function init() {
  meta = await api.meta();
  help = new FunctionHelp(meta);
  previewGrid = new Grid($('preview-table'));
  scanGrid = new Grid($('scan-table'));

  fillSelect($('f-source'), meta.sources);
  fillSelect($('f-market'), ['', ...meta.markets], { '': '(none)' });
  fillSelect($('f-compression'), meta.compressions);
  fillSelect($('f-partition-date'), ['', ...meta.partition_date_formats], { '': '(default)' });

  applyConfig(restore() ?? meta.config);
  // Read back from the form so the config we send always matches what is shown.
  config = readConfig();
  wireEvents();
  refreshAll({ preview: true });
}

function restore() {
  try {
    const saved = localStorage.getItem(STORAGE_KEY);
    return saved ? JSON.parse(saved) : null;
  } catch {
    return null;
  }
}

// ────────────────────────── config <-> form ──────────────────────────────

/** Populate the form from a config object. */
function applyConfig(cfg) {
  const set = (name, value) => {
    const field = form.elements[name];
    if (!field) return;
    if (field.type === 'checkbox') field.checked = Boolean(value);
    else field.value = value ?? '';
  };

  set('source', cfg.source);
  // The period list depends on the source, so populate it before setting it.
  refreshPeriods(cfg.source);
  set('period', cfg.period || 'd');
  set('tiingo_token', cfg.tiingo_token);
  set('start_date', cfg.start_date);
  set('end_date', cfg.end_date);
  set('market', cfg.market);
  set('tickers', (cfg.tickers ?? []).join(', '));
  set('filter', cfg.filter);
  set('lookback', cfg.lookback);
  set('truncate', cfg.truncate ?? 0);
  set('split_pct', cfg.split_pct ?? 0);
  set('drop_columns', cfg.drop_columns);
  set('pivot', cfg.pivot);
  set('target_column', cfg.target_column);
  set('outfile', cfg.outfile);
  set('logfile', cfg.logfile);
  set('parquet_compression', cfg.parquet_compression || 'snappy');
  set('parquet_row_group_size', cfg.parquet_row_group_size ?? 100000);
  set('parquet_partition_by', (cfg.parquet_partition_by ?? []).join(', '));
  set('parquet_partition_date_format', cfg.parquet_partition_date_format);
  set('parquet_sort_by', (cfg.parquet_sort_by ?? []).join(', '));

  const formats = (cfg.output_formats ?? []).map((f) => f.toLowerCase());
  form.elements.format_csv.checked = formats.length === 0 || formats.includes('csv');
  form.elements.format_parquet.checked = formats.includes('parquet');

  columns = (cfg.columns ?? []).map((spec) => {
    const at = spec.indexOf('=');
    return at === -1
      ? { name: spec, expr: '' }
      : { name: spec.slice(0, at).trim(), expr: spec.slice(at + 1).trim() };
  });
  if (!columns.length) columns.push({ name: '', expr: '' });
  renderColumns();
  syncConditionalFields();
}

/** Read the form back into a config object. */
function readConfig() {
  const get = (name) => {
    const field = form.elements[name];
    if (!field) return '';
    return field.type === 'checkbox' ? field.checked : field.value;
  };

  const formats = [];
  if (get('format_csv')) formats.push('csv');
  if (get('format_parquet')) formats.push('parquet');

  // Blank rows are not sent, so remember which editor row each sent column came
  // from in order to attach the server's errors to the right one.
  sentColumnIndex = [];
  columns.forEach((c, i) => {
    if (c.name.trim() || c.expr.trim()) sentColumnIndex.push(i);
  });

  return {
    tiingo_token: get('tiingo_token'),
    logfile: get('logfile'),
    outfile: get('outfile'),
    start_date: get('start_date'),
    end_date: get('end_date'),
    filter: get('filter'),
    source: get('source'),
    period: get('period'),
    tickers: splitList(get('tickers')),
    market: get('market'),
    columns: columns
      .filter((c) => c.name.trim() || c.expr.trim())
      .map((c) => `${c.name.trim()}=${c.expr.trim()}`),
    drop_columns: get('drop_columns'),
    target_column: get('target_column'),
    lookback: get('lookback'),
    truncate: Number(get('truncate')) || 0,
    pivot: get('pivot'),
    split_pct: Number(get('split_pct')) || 0,
    output_formats: formats,
    parquet_compression: get('parquet_compression'),
    parquet_partition_by: splitList(get('parquet_partition_by')),
    parquet_partition_date_format: get('parquet_partition_date_format'),
    parquet_sort_by: splitList(get('parquet_sort_by')),
    parquet_row_group_size: Number(get('parquet_row_group_size')) || 0,
  };
}

function splitList(value) {
  return String(value ?? '')
    .split(/[\s,|]+/)
    .map((s) => s.trim())
    .filter(Boolean);
}

// ─────────────────────────── column editor ───────────────────────────────

// columnRows holds the DOM node for each column so validation can update error
// text in place. Rebuilding the rows would steal focus while the user types.
let columnRows = [];

function renderColumns() {
  const host = $('columns');
  const frag = document.createDocumentFragment();
  columnRows = [];

  columns.forEach((column, index) => {
    const row = el('div', { class: 'column-row', draggable: 'true' });

    const name = el('input', { type: 'text', placeholder: 'name' });
    name.value = column.name;
    // The form's own input listener handles the debounce; this only keeps the
    // model in step, and runs first because it is on the target.
    name.addEventListener('input', () => {
      columns[index].name = name.value;
    });

    const expr = el('input', { type: 'text', placeholder: 'sma(c,20)' });
    expr.value = column.expr;
    expr.addEventListener('input', () => {
      columns[index].expr = expr.value;
    });
    help.attach(expr);

    const remove = el('button', { type: 'button', class: 'ghost', title: 'Remove' }, '×');
    remove.addEventListener('click', () => {
      columns.splice(index, 1);
      if (!columns.length) columns.push({ name: '', expr: '' });
      renderColumns();
      onChange();
    });

    const drag = el('button', { type: 'button', class: 'drag', title: 'Drag to reorder' }, '⠿');

    const error = el('div', { class: 'err' });
    row.append(name, el('span', { class: 'eq' }, '='), expr, drag, error);
    columnRows.push({ row, name, expr, error });

    row.addEventListener('dragstart', (event) => {
      event.dataTransfer.setData('text/plain', String(index));
      row.classList.add('dragging');
    });
    row.addEventListener('dragend', () => row.classList.remove('dragging'));
    row.addEventListener('dragover', (event) => event.preventDefault());
    row.addEventListener('drop', (event) => {
      event.preventDefault();
      const from = Number(event.dataTransfer.getData('text/plain'));
      if (Number.isNaN(from) || from === index) return;
      const [moved] = columns.splice(from, 1);
      columns.splice(index, 0, moved);
      renderColumns();
      onChange();
    });

    frag.append(row);
  });

  host.replaceChildren(frag);
  refreshColumnMeta();
}

// The rows are not rebuilt on every keystroke, so anything derived from the
// column names has to be refreshed separately.
function refreshColumnMeta() {
  const names = columns.map((c) => c.name.trim()).filter(Boolean);
  $('column-count').textContent = names.length || '';
  help.setColumnNames(names);
}

// ───────────────────────────── wiring ────────────────────────────────────

function wireEvents() {
  // Nothing is submitted over a form post; Enter in a text field would
  // otherwise reload the page and lose the config.
  form.addEventListener('submit', (event) => event.preventDefault());
  form.addEventListener('input', onChange);
  form.addEventListener('change', () => {
    syncConditionalFields();
    onChange();
  });

  $('btn-add-column').addEventListener('click', () => {
    columns.push({ name: '', expr: '' });
    renderColumns();
    $('columns').lastElementChild?.querySelector('input')?.focus();
  });

  document.querySelectorAll('.tab').forEach((tab) => {
    tab.addEventListener('click', () => selectTab(tab.dataset.tab));
  });

  $('btn-preview').addEventListener('click', () => runPreview());
  $('btn-scan').addEventListener('click', () => runScan());
  $('btn-run').addEventListener('click', () => startRun());
  $('btn-run-2').addEventListener('click', () => startRun());
  $('btn-cancel').addEventListener('click', () => cancelRun());
  $('btn-load').addEventListener('click', () => loadConfigFile());
  $('btn-save').addEventListener('click', () => saveConfigFile());
  $('btn-clear-cache').addEventListener('click', () => clearCache());

  $('btn-copy-yaml').addEventListener('click', () => {
    navigator.clipboard?.writeText($('yaml-view').textContent);
    setStatus('YAML copied', 'ok');
  });
  $('btn-copy-passed').addEventListener('click', () => {
    navigator.clipboard?.writeText(passedTickers().join(', '));
    setStatus(`${passedTickers().length} tickers copied`, 'ok');
  });
  $('btn-use-passed').addEventListener('click', () => {
    const passed = passedTickers();
    if (!passed.length) return;
    form.elements.tickers.value = passed.join(', ');
    form.elements.market.value = '';
    onChange();
    setStatus(`Ticker list replaced with ${passed.length} passing symbols`, 'ok');
  });
  $('scan-passed-only').addEventListener('change', () => renderScan());
}

/** Show and hide fields that only apply in certain modes. */
function syncConditionalFields() {
  const source = form.elements.source.value;
  $('token-field').hidden = !source.startsWith('tiingo');
  refreshPeriods(source);
  $('parquet-options').hidden = !form.elements.format_parquet.checked;
  $('target-field').hidden = !form.elements.pivot.checked;
}

/**
 * Refill the period picker for the selected source. The sets differ a lot —
 * Binance serves 15 periods, Tiingo 3 — so the current choice is kept only when
 * the new source still offers it.
 */
function refreshPeriods(source) {
  const select = $('f-period');
  const periods = meta.providers.find((p) => p.name === source)?.periods ?? [];
  const wanted = select.value;

  fillSelect(select, periods);
  select.value = periods.includes(wanted) ? wanted : (periods.includes('d') ? 'd' : periods[0] ?? '');
}

let validateTimer = null;
let previewTimer = null;
let configValid = true;

function onChange() {
  config = readConfig();
  refreshColumnMeta();
  localStorage.setItem(STORAGE_KEY, JSON.stringify(config));

  clearTimeout(validateTimer);
  validateTimer = setTimeout(() => refreshAll({ preview: false }), VALIDATE_DELAY);

  if ($('auto-preview').checked) {
    clearTimeout(previewTimer);
    // Validation runs first and settles well before this fires, so a config
    // known to be invalid never costs a round trip.
    previewTimer = setTimeout(() => {
      if (configValid) runPreview();
    }, PREVIEW_DELAY);
  }
}

async function refreshAll({ preview }) {
  const ok = await validate();
  await refreshYAML();
  if (preview && ok) runPreview();
}

// ──────────────────────────── validation ─────────────────────────────────

async function validate() {
  let response;
  try {
    response = await api.validate(config);
  } catch (err) {
    setStatus(err.message, 'error');
    configValid = false;
    return false;
  }

  const problems = response.problems;
  const errors = problems.filter((p) => p.severity === 'error');
  const badge = $('problem-badge');

  if (problems.length) {
    badge.hidden = false;
    badge.className = errors.length ? 'badge' : 'badge warn';
    badge.textContent = errors.length
      ? `${errors.length} error${errors.length > 1 ? 's' : ''}`
      : `${problems.length} warning${problems.length > 1 ? 's' : ''}`;
    badge.title = problems.map((p) => `${p.field}: ${p.message}`).join('\n');
  } else {
    badge.hidden = true;
  }

  // Attach problems to their fields. Column errors are written into the
  // existing rows so the field the user is typing in keeps focus.
  form.querySelectorAll('.invalid').forEach((node) => node.classList.remove('invalid'));
  for (const entry of columnRows) {
    entry.error.textContent = '';
    entry.expr.classList.remove('invalid');
  }

  for (const problem of problems) {
    if (problem.field === 'columns') {
      const entry = columnRows[sentColumnIndex[problem.index]];
      if (entry) {
        entry.error.textContent = problem.message;
        if (problem.severity === 'error') entry.expr.classList.add('invalid');
      }
      continue;
    }
    const field = form.elements[problem.field];
    if (field && problem.severity === 'error' && field.classList) {
      field.classList.add('invalid');
    }
  }

  setStatus(errors.length ? `${errors[0].field}: ${errors[0].message}` : '', errors.length ? 'error' : '');
  configValid = errors.length === 0;
  return configValid;
}

async function refreshYAML() {
  try {
    $('yaml-view').textContent = await api.yaml(config);
  } catch (err) {
    $('yaml-view').textContent = `# ${err.message}`;
  }
}

// ───────────────────────────── preview ───────────────────────────────────

let previewSeq = 0;

async function runPreview() {
  const seq = ++previewSeq;
  $('preview-meta').textContent = 'Loading…';

  let result;
  try {
    result = await api.preview(config, meta.preview_limits);
  } catch (err) {
    if (seq !== previewSeq) return;
    $('preview-meta').textContent = '';
    showNotices($('preview-errors'), [{ severity: 'error', message: err.message }]);
    previewGrid.clear();
    return;
  }
  if (seq !== previewSeq) return; // a newer preview already landed

  previewGrid.setData(result.headers, result.rows);
  showNotices(
    $('preview-errors'),
    result.errors.map((e) => ({ severity: 'warn', field: e.ticker, message: e.message })),
  );
  showColumnEvalErrors(result.errors);

  // A market can resolve to thousands of symbols; say so, because the preview
  // only samples the first few and a full run would fetch every one.
  const sampled = result.tickers.join(', ') || 'nothing';
  const universe =
    result.universe > result.tickers.length
      ? `sampled ${sampled} of ${result.universe} symbols`
      : `${sampled}`;

  // The preview keeps only the most recent bars, so a config starting years
  // back still opens mid-history here. Say so plainly: read as "N rows", that
  // looks like missing data rather than a display limit.
  const shown = result.rows.length;
  const rows =
    result.total_rows > shown
      ? `showing the last ${shown} of ${result.total_rows} rows`
      : `${shown} rows`;
  $('preview-meta').textContent =
    `${rows} · ${result.headers.length} columns · ${universe} · ${result.elapsed}`;
}

// ─────────────────────────────── scan ────────────────────────────────────

async function runScan() {
  $('scan-meta').textContent = 'Scanning…';
  $('btn-scan').disabled = true;
  try {
    scanResult = await api.scan(config);
  } catch (err) {
    $('scan-meta').textContent = '';
    showNotices($('scan-errors'), [{ severity: 'error', message: err.message }]);
    return;
  } finally {
    $('btn-scan').disabled = false;
  }
  renderScan();
}

function renderScan() {
  if (!scanResult) return;

  const passedOnly = $('scan-passed-only').checked;
  const verdicts = passedOnly ? scanResult.verdicts.filter((v) => v.passed) : scanResult.verdicts;

  // Show the ticker, its verdict and every value the filter could have used.
  const valueColumns = scanResult.headers.filter((h) => h !== 'symbol');
  const headers = ['ticker', 'filter', 'bars', ...valueColumns];
  const rows = verdicts.map((v) => [
    v.ticker,
    v.passed ? 'pass' : 'fail',
    String(v.bars),
    ...valueColumns.map((h) => v.values?.[h] ?? ''),
  ]);

  scanGrid.renderRow = (row, tr) => {
    if (row[1] === 'fail') tr.classList.add('fail');
    const cell = tr.children[1];
    if (cell) {
      cell.replaceChildren(el('span', { class: `pill ${row[1]}` }, row[1]));
      cell.className = 'text';
    }
  };
  scanGrid.setData(headers, rows);

  showNotices(
    $('scan-errors'),
    scanResult.errors.map((e) => ({ severity: 'warn', field: e.ticker, message: e.message })),
  );
  $('scan-meta').textContent =
    `${scanResult.passed} of ${scanResult.total} tickers passed` +
    (scanResult.errors.length ? ` · ${scanResult.errors.length} failed to load` : '') +
    ` · ${scanResult.elapsed}`;
}

// A bad expression is only discovered when it runs, so the preview's per-ticker
// errors are the only place a column's real problem shows up. The engine
// prefixes them with "column <name>:", which is enough to put the message back
// on the row that caused it.
function showColumnEvalErrors(errors) {
  const byName = new Map();
  for (const err of errors) {
    const match = /^column ([A-Za-z_][A-Za-z0-9_]*): (.*)$/s.exec(err.message);
    if (match && !byName.has(match[1])) byName.set(match[1], match[2]);
  }
  if (!byName.size) return;

  for (const entry of columnRows) {
    const message = byName.get(entry.name.value.trim());
    if (message) {
      entry.error.textContent = message;
      entry.expr.classList.add('invalid');
    }
  }
}

function passedTickers() {
  return (scanResult?.verdicts ?? []).filter((v) => v.passed).map((v) => v.ticker);
}

// ──────────────────────────────── run ────────────────────────────────────

async function startRun() {
  if (currentJob) return;
  selectTab('run');

  $('run-log').textContent = '';
  $('run-files').replaceChildren();
  $('run-meta').textContent = 'Starting…';
  setProgress(0);

  let job;
  try {
    job = await api.run(config);
  } catch (err) {
    const problems = err.payload?.problems;
    $('run-meta').textContent = problems
      ? problems.map((p) => `${p.field}: ${p.message}`).join('\n')
      : err.message;
    return;
  }

  currentJob = job.job_id;
  $('btn-run').disabled = true;
  $('btn-run-2').disabled = true;
  $('btn-cancel').disabled = false;

  const stream = api.events(currentJob);
  stream.onmessage = (event) => handleRunEvent(JSON.parse(event.data), stream);
  stream.onerror = () => {
    stream.close();
    finishRun();
  };
}

function handleRunEvent(event, stream) {
  switch (event.type) {
    case 'progress': {
      const { phase, done, total, ticker } = event.progress;
      setProgress(total ? done / total : 0);
      $('run-meta').textContent = `${phase} ${done}/${total}${ticker ? ` · ${ticker}` : ''}`;
      break;
    }
    case 'log':
      appendLog(event.message);
      break;
    case 'done': {
      const r = event.result;
      setProgress(1);
      $('run-meta').textContent =
        `Wrote ${r.rows} rows from ${r.kept} of ${r.total} tickers in ${r.elapsed}` +
        (r.errors.length ? ` · ${r.errors.length} tickers failed` : '');
      // The server sends Content-Disposition, so these download rather than
      // navigate.
      $('run-files').replaceChildren(
        ...r.files.map((path) => el('a', { href: api.downloadURL(path) }, path)),
      );
      for (const e of r.errors) appendLog(`${e.ticker}: ${e.message}`);
      stream.close();
      finishRun();
      setStatus('Run complete', 'ok');
      break;
    }
    case 'error':
      appendLog(`error: ${event.message}`);
      $('run-meta').textContent = event.message;
      stream.close();
      finishRun();
      break;
    default:
      break;
  }
}

async function cancelRun() {
  if (!currentJob) return;
  try {
    await api.cancel(currentJob);
  } catch (err) {
    setStatus(err.message, 'error');
  }
}

function finishRun() {
  currentJob = null;
  $('btn-run').disabled = false;
  $('btn-run-2').disabled = false;
  $('btn-cancel').disabled = true;
}

function setProgress(fraction) {
  $('progress-bar').style.width = `${Math.round(fraction * 100)}%`;
}

function appendLog(line) {
  const log = $('run-log');
  log.textContent += line + '\n';
  log.scrollTop = log.scrollHeight;
}

// ──────────────────────── config file load/save ──────────────────────────

async function loadConfigFile() {
  const path = $('config-path').value.trim();
  if (!path) return setStatus('Enter a config path first', 'error');
  try {
    const response = await api.loadConfig(path);
    config = response.config;
    applyConfig(config);
    localStorage.setItem(STORAGE_KEY, JSON.stringify(config));
    setStatus(`Loaded ${response.path}`, 'ok');
    refreshAll({ preview: true });
  } catch (err) {
    setStatus(err.message, 'error');
  }
}

async function saveConfigFile() {
  const path = $('config-path').value.trim();
  if (!path) return setStatus('Enter a config path first', 'error');
  try {
    const response = await api.saveConfig(path, config);
    setStatus(`Saved ${response.path}`, 'ok');
  } catch (err) {
    setStatus(err.message, 'error');
  }
}

async function clearCache() {
  try {
    await api.clearCache();
    setStatus('Quote cache cleared', 'ok');
  } catch (err) {
    setStatus(err.message, 'error');
  }
}

// ──────────────────────────── small helpers ──────────────────────────────

function selectTab(name) {
  document.querySelectorAll('.tab').forEach((tab) => {
    tab.setAttribute('aria-selected', String(tab.dataset.tab === name));
  });
  document.querySelectorAll('.tab-panel').forEach((panel) => {
    panel.hidden = panel.dataset.panel !== name;
  });
  // The grids size themselves from a visible container.
  if (name === 'data') previewGrid?.paint();
  if (name === 'scan') scanGrid?.paint();
}

function fillSelect(select, values, labels = {}) {
  select.replaceChildren(
    ...values.map((value) => {
      // Set value as a property: an empty string is a meaningful option value
      // but would be dropped as an attribute.
      const option = document.createElement('option');
      option.value = value;
      option.textContent = labels[value] ?? value;
      return option;
    }),
  );
}

function showNotices(host, notices) {
  host.replaceChildren(
    ...notices.map((n) =>
      el('div', { class: `notice ${n.severity === 'error' ? 'error' : 'warn'}` },
        n.field ? el('b', {}, n.field) : null,
        n.message)),
  );
}

let statusTimer = null;

function setStatus(message, kind = '') {
  const node = $('status');
  node.textContent = message;
  node.className = `status ${kind}`;
  clearTimeout(statusTimer);
  if (message && kind === 'ok') {
    statusTimer = setTimeout(() => {
      node.textContent = '';
    }, 4000);
  }
}
