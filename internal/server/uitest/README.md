# Web UI smoke test

An end-to-end browser test for the `-serve` UI. It drives a real Chromium against a running
server and checks that the page loads without console errors, that the form is populated from
`/api/meta`, that autocomplete, validation, preview, sorting, the screener, the function
reference and a full run all work, and that the page does not overflow horizontally. It also
writes light and dark screenshots.

Playwright is not a dependency of go-scan; install it only when you want to run this.

```sh
npm init -y && npm install playwright && npx playwright install chromium

go build -o scan ./cmd/scan
./scan -serve -addr 127.0.0.1:8899 &

BASE=http://127.0.0.1:8899 SHOTS=/tmp node internal/server/uitest/uitest.mjs
```

The test uses the `coinbase` source, which needs no API token but does need network access.
It exits non-zero and lists every failure if anything breaks.
