// A sortable table that only renders the rows currently in view. Preview sets
// run to a few thousand rows, which is more than enough to make naive DOM
// rendering feel slow.

const ROW_HEIGHT = 22;
const OVERSCAN = 12;

export class Grid {
  /**
   * @param {HTMLElement} host scrolling container
   */
  constructor(host) {
    this.host = host;
    this.headers = [];
    this.rows = [];
    this.order = null; // indices into rows, or null for natural order
    this.sortCol = -1;
    this.sortDir = 1;
    this.renderRow = null; // optional (row, tr) => void decorator

    this.host.addEventListener('scroll', () => this.paint());
    new ResizeObserver(() => this.paint()).observe(this.host);
  }

  setData(headers, rows) {
    this.headers = headers || [];
    this.rows = rows || [];
    this.sortCol = -1;
    this.order = null;
    this.build();
  }

  clear() {
    this.setData([], []);
  }

  build() {
    this.host.replaceChildren();
    this.table = null;
    this.tbody = null;
    if (!this.headers.length) {
      this.host.append(el('div', { class: 'empty' }, 'Nothing to show yet.'));
      return;
    }
    if (!this.rows.length) {
      this.host.append(el('div', { class: 'empty' }, 'No rows to show.'));
      return;
    }

    this.table = el('table', { class: 'grid' });
    const thead = el('thead');
    const tr = el('tr');
    this.headers.forEach((header, i) => {
      const th = el('th', {}, header);
      th.append(el('span', { class: 'sort' }, ''));
      th.addEventListener('click', () => this.sortBy(i));
      tr.append(th);
    });
    thead.append(tr);

    this.tbody = el('tbody');
    // Two spacer rows stand in for everything scrolled out of view, so the
    // scrollbar reflects the full data set.
    this.topPad = el('tr');
    this.topPadCell = el('td', { colspan: String(this.headers.length) });
    this.topPad.append(this.topPadCell);
    this.bottomPad = el('tr');
    this.bottomPadCell = el('td', { colspan: String(this.headers.length) });
    this.bottomPad.append(this.bottomPadCell);

    this.table.append(thead, this.tbody);
    this.host.append(this.table);
    this.paint();
  }

  sortBy(col) {
    if (this.sortCol === col) {
      this.sortDir = -this.sortDir;
    } else {
      this.sortCol = col;
      this.sortDir = 1;
    }

    const indices = this.rows.map((_, i) => i);
    const dir = this.sortDir;
    indices.sort((a, b) => dir * compare(this.rows[a][col], this.rows[b][col]));
    this.order = indices;

    this.table.querySelectorAll('th .sort').forEach((span, i) => {
      span.textContent = i === col ? (dir > 0 ? '▲' : '▼') : '';
    });
    this.host.scrollTop = 0;
    this.paint();
  }

  rowAt(i) {
    return this.rows[this.order ? this.order[i] : i];
  }

  paint() {
    if (!this.tbody) return;

    const total = this.rows.length;
    const viewport = this.host.clientHeight || 400;
    const first = Math.max(0, Math.floor(this.host.scrollTop / ROW_HEIGHT) - OVERSCAN);
    const visible = Math.ceil(viewport / ROW_HEIGHT) + OVERSCAN * 2;
    const last = Math.min(total, first + visible);

    const frag = document.createDocumentFragment();
    this.topPadCell.style.height = `${first * ROW_HEIGHT}px`;
    this.topPadCell.style.padding = '0';
    this.bottomPadCell.style.height = `${(total - last) * ROW_HEIGHT}px`;
    this.bottomPadCell.style.padding = '0';
    frag.append(this.topPad);

    for (let i = first; i < last; i++) {
      const row = this.rowAt(i);
      const tr = el('tr');
      for (const cell of row) {
        tr.append(el('td', { class: isNumeric(cell) ? '' : 'text' }, format(cell)));
      }
      if (this.renderRow) this.renderRow(row, tr);
      frag.append(tr);
    }
    frag.append(this.bottomPad);
    this.tbody.replaceChildren(frag);
  }
}

// format trims the engine's fixed 6-decimal output down to something readable
// without hiding precision that matters.
function format(cell) {
  if (!isNumeric(cell)) return cell;
  const n = Number(cell);
  if (!Number.isFinite(n)) return cell;
  if (Number.isInteger(n)) return String(n);
  if (Math.abs(n) >= 1e6 || (Math.abs(n) < 1e-4 && n !== 0)) return n.toExponential(3);
  return String(Math.round(n * 1e6) / 1e6);
}

function isNumeric(value) {
  return value !== '' && !Number.isNaN(Number(value));
}

function compare(a, b) {
  const na = Number(a);
  const nb = Number(b);
  if (!Number.isNaN(na) && !Number.isNaN(nb)) return na - nb;
  return String(a).localeCompare(String(b));
}

export function el(tag, attrs = {}, ...children) {
  const node = document.createElement(tag);
  for (const [key, value] of Object.entries(attrs)) {
    if (value === '' || value == null) continue;
    node.setAttribute(key, value);
  }
  for (const child of children) {
    if (child == null) continue;
    node.append(child);
  }
  return node;
}
