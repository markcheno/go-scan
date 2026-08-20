import { chromium } from 'playwright';

const BASE = process.env.BASE || 'http://127.0.0.1:8899';
const TOKEN = process.env.TIINGO_API_TOKEN || '';
const shots = process.env.SHOTS || '.';

const problems = [];
const note = (m) => console.log('  ' + m);
const fail = (m) => { problems.push(m); console.log('  ✗ ' + m); };
const ok = (m) => console.log('  ✓ ' + m);

const browser = await chromium.launch();
const page = await browser.newPage({ viewport: { width: 1500, height: 950 } });

const consoleErrors = [];
page.on('console', (msg) => {
  if (msg.type() === 'error') consoleErrors.push(msg.text());
});
page.on('pageerror', (err) => consoleErrors.push('pageerror: ' + err.message));
const failedRequests = [];
page.on('requestfailed', (r) => failedRequests.push(`${r.method()} ${r.url()} ${r.failure()?.errorText}`));

console.log('\n== load ==');
await page.goto(BASE, { waitUntil: 'networkidle' });
await page.waitForTimeout(800);

const title = await page.title();
title === 'go-scan' ? ok(`title "${title}"`) : fail(`title is "${title}"`);

// The selects are populated from /api/meta, which derives them from go-quote's
// registries. Assert against that rather than a count that changes whenever a
// provider or market is added upstream.
const meta = await page.evaluate(async () => {
  const res = await fetch('/api/meta', { headers: { 'X-Scan-Token': window.SCAN_TOKEN } });
  return res.json();
});
const sources = await page.$$eval('#f-source option', (o) => o.map((x) => x.value));
JSON.stringify(sources) === JSON.stringify(meta.sources)
  ? ok(`sources match the registry: ${sources.join(', ')}`)
  : fail(`sources = ${JSON.stringify(sources)}, meta = ${JSON.stringify(meta.sources)}`);
sources.includes('binance') ? ok('binance is selectable') : fail('binance missing from sources');

const markets = await page.$$eval('#f-market option', (o) => o.map((x) => x.value));
markets[0] === '' && JSON.stringify(markets.slice(1)) === JSON.stringify(meta.markets)
  ? ok(`markets: ${meta.markets.length} plus an empty "(none)"`)
  : fail(`market options do not match meta (${markets.length} vs ${meta.markets.length + 1})`);
markets.includes('etf') && markets.some((m) => m.startsWith('binance-'))
  ? ok('etf and the binance-* markets are offered')
  : fail('etf or binance-* markets missing');

console.log('\n== period picker follows the source ==');
await page.selectOption('#f-source', 'binance');
await page.waitForTimeout(200);
const binancePeriods = await page.$$eval('#f-period option', (o) => o.map((x) => x.value));
binancePeriods.length === 15 && binancePeriods.includes('3d')
  ? ok(`binance offers ${binancePeriods.length} periods incl. 3d`)
  : fail(`binance periods = ${JSON.stringify(binancePeriods)}`);

// Pick a period only Binance serves, then switch to a source that does not.
await page.selectOption('#f-period', '3d');
await page.selectOption('#f-source', 'tiingo');
await page.waitForTimeout(200);
const tiingoPeriods = await page.$$eval('#f-period option', (o) => o.map((x) => x.value));
JSON.stringify(tiingoPeriods) === JSON.stringify(['d', 'w', 'm'])
  ? ok('tiingo collapses to d/w/m')
  : fail(`tiingo periods = ${JSON.stringify(tiingoPeriods)}`);
(await page.locator('#f-period').inputValue()) === 'd'
  ? ok('an unsupported period falls back to daily')
  : fail(`period stayed ${await page.locator('#f-period').inputValue()}`);

// A period the source keeps is preserved across a source change.
await page.selectOption('#f-source', 'binance');
await page.selectOption('#f-period', '1h');
await page.selectOption('#f-source', 'coinbase');
await page.waitForTimeout(200);
(await page.locator('#f-period').inputValue()) === '1h'
  ? ok('a still-valid period survives a source change')
  : fail(`period became ${await page.locator('#f-period').inputValue()}`);

console.log('\n== conditional fields ==');
await page.selectOption('#f-source', 'coinbase');
await page.waitForTimeout(150);
(await page.locator('#token-field').isVisible())
  ? fail('token field is visible for coinbase')
  : ok('token field hidden for coinbase');
await page.selectOption('#f-source', 'tiingo');
await page.waitForTimeout(150);
(await page.locator('#token-field').isVisible())
  ? ok('token field shown for tiingo')
  : fail('token field hidden for tiingo');

(await page.locator('#parquet-options').isVisible())
  ? fail('parquet options visible before parquet is ticked')
  : ok('parquet options hidden by default');
await page.getByText('Output', { exact: true }).click();
await page.locator('input[name=format_parquet]').check();
await page.waitForTimeout(150);
(await page.locator('#parquet-options').isVisible())
  ? ok('parquet options revealed')
  : fail('parquet options still hidden after ticking parquet');
await page.locator('input[name=format_parquet]').uncheck();

console.log('\n== fill in a config ==');
await page.selectOption('#f-source', 'coinbase');
await page.fill('input[name=start_date]', '2024-01-01');
await page.fill('input[name=end_date]', '2024-06-30');
await page.fill('textarea[name=tickers]', 'BTC-USD, ETH-USD');

// First column row.
const nameInputs = page.locator('.column-row input').nth(0);
await nameInputs.fill('sma20');
await page.locator('.column-row input').nth(1).fill('sma(c,20)');

console.log('\n== autocomplete ==');
await page.click('#btn-add-column');
await page.waitForTimeout(100);
await page.locator('.column-row').nth(1).locator('input').nth(0).fill('above');
const expr2 = page.locator('.column-row').nth(1).locator('input').nth(1);
await expr2.click();
await expr2.type('gt', { delay: 40 });
await page.waitForTimeout(300);
const acVisible = await page.locator('#autocomplete').isVisible();
acVisible ? ok('autocomplete popup opened') : fail('autocomplete did not open');
if (acVisible) {
  const first = await page.locator('#autocomplete .ac-item').first().innerText();
  note(`first suggestion: ${first.replace(/\n/g, ' — ')}`);
  await expr2.press('Enter');
  await page.waitForTimeout(150);
  const value = await expr2.inputValue();
  value === 'gt(' ? ok('accepting a suggestion inserted "gt("') : fail(`inserted "${value}"`);
}
await expr2.fill('gt(c,sma20)');

console.log('\n== derived state keeps up ==');
await page.waitForTimeout(400);
const count = await page.locator('#column-count').innerText();
count === '2' ? ok('column count reads 2') : fail(`column count reads "${count}", want 2`);
// A column name typed just now must already be offered by autocomplete.
const probe = page.locator('.column-row').nth(1).locator('input').nth(1);
await probe.fill('');
await probe.type('sma2', { delay: 30 });
await page.waitForTimeout(250);
const suggestions = await page.$$eval('#autocomplete .ac-item code', (n) => n.map((x) => x.textContent));
suggestions.includes('sma20')
  ? ok('a user column is offered by autocomplete')
  : fail(`autocomplete offered ${JSON.stringify(suggestions)}`);
await probe.press('Escape');
await probe.fill('gt(c,sma20)');

console.log('\n== focus is kept while typing ==');
const filterInput = page.locator('input[name=filter]');
await filterInput.click();
await filterInput.type('close > ', { delay: 30 });
await page.waitForTimeout(900); // let validation land
const focused = await page.evaluate(() => document.activeElement?.getAttribute('name'));
focused === 'filter' ? ok('focus survives a validation pass') : fail(`focus moved to ${focused}`);
await filterInput.fill('close > 1000');

console.log('\n== preview ==');
await page.click('#btn-preview');
await page.waitForFunction(
  () => !document.getElementById('preview-meta').textContent.includes('Loading'),
  null,
  { timeout: 30000 },
);
await page.waitForTimeout(400);
const previewMeta = await page.locator('#preview-meta').innerText();
note(previewMeta);
const previewRows = await page.locator('#preview-table table.grid tbody tr').count();
previewRows > 2 ? ok(`preview rendered ${previewRows} row elements`) : fail('preview table is empty');
const headers = await page.$$eval('#preview-table th', (t) => t.map((x) => x.textContent.trim()));
note('headers: ' + headers.join(' '));
headers.includes('sma20') && headers.includes('above')
  ? ok('computed columns present')
  : fail('computed columns missing from the preview');

console.log('\n== sorting ==');
const closeIdx = headers.findIndex((h) => h.startsWith('close'));
const beforeSort = await page.$$eval('#preview-table tbody tr td:nth-child(6)', (t) => t.slice(0, 3).map((x) => x.textContent));
await page.locator('#preview-table th').nth(closeIdx).click();
await page.waitForTimeout(200);
const afterSort = await page.$$eval('#preview-table tbody tr td:nth-child(6)', (t) => t.slice(0, 3).map((x) => x.textContent));
JSON.stringify(beforeSort) !== JSON.stringify(afterSort)
  ? ok(`sorting reordered rows (${beforeSort[0]} -> ${afterSort[0]})`)
  : fail('clicking a header did not reorder anything');

console.log('\n== validation surfaces errors ==');
// A bad expression only fails when it runs, so it must come back from the
// preview and land on the column that caused it.
const badExpr = page.locator('.column-row').nth(0).locator('input').nth(1);
await badExpr.fill('nosuchfn(c,20)');
await page.waitForFunction(
  () => document.querySelector('.column-row .err')?.textContent.length > 0,
  null,
  { timeout: 30000 },
).then(() => ok('a bad expression is reported on its own column row'))
 .catch(() => fail('a bad expression produced no inline error'));
note('column error: ' + (await page.locator('.column-row .err').first().innerText()));
await badExpr.fill('sma(c,20)');

// A structural problem is caught by validation without running anything.
await page.evaluate(() => document.querySelectorAll('details').forEach((d) => (d.open = true)));
await page.waitForTimeout(150);
await page.fill('input[name=split_pct]', '5');
await page.waitForTimeout(900);
const splitInvalid = await page.locator('input[name=split_pct]').evaluate((n) => n.classList.contains('invalid'));
splitInvalid ? ok('an out-of-range split marks its own field invalid') : fail('bad split_pct did not mark the field');
const badgeVisible = await page.locator('#problem-badge').isVisible();
badgeVisible ? ok('error badge shown: ' + (await page.locator('#problem-badge').innerText())) : fail('no error badge');
note('status: ' + (await page.locator('#status').innerText()));
await page.fill('input[name=split_pct]', '0');
await page.waitForTimeout(900);
(await page.locator('#problem-badge').isVisible())
  ? fail('badge stayed after the problem was fixed')
  : ok('badge clears once the config is valid again');

console.log('\n== yaml tab ==');
await page.click('.tab[data-tab=yaml]');
await page.waitForTimeout(600);
const yaml = await page.locator('#yaml-view').innerText();
yaml.includes('columns:') && yaml.includes('sma20=sma(c,20)')
  ? ok('YAML shows the configured columns')
  : fail('YAML looks wrong:\n' + yaml.slice(0, 300));
note(yaml.split('\n').slice(0, 6).join(' | '));

console.log('\n== scan tab ==');
await page.click('.tab[data-tab=scan]');
await page.click('#btn-scan');
await page.waitForFunction(
  () => !document.getElementById('scan-meta').textContent.includes('Scanning'),
  null,
  { timeout: 60000 },
);
await page.waitForTimeout(400);
note(await page.locator('#scan-meta').innerText());
const scanRows = await page.locator('#scan-table tbody tr').count();
scanRows > 1 ? ok(`scan listed ${scanRows} row elements`) : fail('scan table empty');
const pills = await page.$$eval('#scan-table .pill', (p) => p.map((x) => x.textContent));
pills.length ? ok(`verdict pills: ${pills.join(', ')}`) : fail('no pass/fail pills rendered');

console.log('\n== function reference ==');
await page.click('.tab[data-tab=data]');
await page.click('#btn-functions');
await page.waitForTimeout(300);
(await page.locator('#function-dialog').isVisible()) ? ok('dialog opened') : fail('dialog did not open');
await page.fill('#function-search', 'bollinger');
await page.waitForTimeout(200);
const found = await page.locator('#function-list .func').count();
found === 1 ? ok('search for "bollinger" found bbands') : fail(`search returned ${found} results`);
await page.click('#btn-close-functions');

console.log('\n== run ==');
await page.click('.tab[data-tab=run]');
await page.click('#btn-run-2');
await page.waitForFunction(
  () => document.getElementById('run-files').children.length > 0
     || /error|cancelled/i.test(document.getElementById('run-meta').textContent),
  null,
  { timeout: 60000 },
);
await page.waitForTimeout(500);
note(await page.locator('#run-meta').innerText());
const files = await page.$$eval('#run-files a', (a) => a.map((x) => x.textContent));
files.length ? ok(`download links: ${files.join(', ')}`) : fail('no output files linked');
const logLines = (await page.locator('#run-log').innerText()).trim().split('\n');
note(`run log has ${logLines.length} lines, last: ${logLines[logLines.length - 1]}`);
const width = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
width <= 0 ? ok('no horizontal page overflow') : fail(`page overflows horizontally by ${width}px`);

console.log('\n== screenshots ==');
await page.click('.tab[data-tab=data]');
await page.waitForTimeout(300);
await page.screenshot({ path: `${shots}/ui-light.png` });
await page.emulateMedia({ colorScheme: 'dark' });
await page.waitForTimeout(200);
await page.screenshot({ path: `${shots}/ui-dark.png` });
await page.click('.tab[data-tab=scan]');
await page.waitForTimeout(200);
await page.screenshot({ path: `${shots}/ui-scan.png` });
ok('captured ui-light.png, ui-dark.png, ui-scan.png');

console.log('\n== console ==');
consoleErrors.length ? consoleErrors.forEach((e) => fail('console: ' + e)) : ok('no console errors');
failedRequests.length ? failedRequests.forEach((r) => fail('request failed: ' + r)) : ok('no failed requests');

await browser.close();

console.log(`\n${problems.length ? `FAILURES (${problems.length}):` : 'ALL CHECKS PASSED'}`);
problems.forEach((p) => console.log(' - ' + p));
process.exit(problems.length ? 1 : 0);
