// Function reference dialog and expression autocomplete, both driven by the
// catalog the server exposes at /api/meta.

import { el } from './grid.js';

const IDENT = /[A-Za-z_][A-Za-z0-9_]*$/;

export class FunctionHelp {
  constructor(meta) {
    this.functions = meta.functions;
    this.categories = meta.categories;
    this.extraNames = []; // user column names, refreshed as they are edited

    this.dialog = document.getElementById('function-dialog');
    this.list = document.getElementById('function-list');
    this.search = document.getElementById('function-search');
    this.popup = document.getElementById('autocomplete');

    this.search.addEventListener('input', () => this.renderList());
    document.getElementById('btn-close-functions').addEventListener('click', () => this.dialog.close());
    document.getElementById('btn-functions').addEventListener('click', () => {
      this.renderList();
      this.dialog.showModal();
      this.search.focus();
    });

    this.renderList();
    this.hideSuggestions();
  }

  setColumnNames(names) {
    this.extraNames = names;
  }

  renderList() {
    const query = this.search.value.trim().toLowerCase();
    const matches = this.functions.filter(
      (f) => !query || f.name.toLowerCase().includes(query) || f.doc.toLowerCase().includes(query),
    );

    const frag = document.createDocumentFragment();
    for (const category of this.categories) {
      const group = matches.filter((f) => f.category === category);
      if (!group.length) continue;
      frag.append(el('div', { class: 'func-group' }, category));
      for (const f of group) {
        const row = el('div', { class: 'func' },
          el('code', {}, f.signature),
          el('span', {}, f.doc));
        row.addEventListener('click', () => {
          navigator.clipboard?.writeText(f.signature);
          this.dialog.close();
        });
        frag.append(row);
      }
    }
    if (!frag.childElementCount) {
      frag.append(el('div', { class: 'empty' }, 'No matching functions.'));
    }
    this.list.replaceChildren(frag);
  }

  /** Attach autocomplete behaviour to an expression input. */
  attach(input) {
    input.addEventListener('input', () => this.showSuggestions(input));
    input.addEventListener('blur', () => setTimeout(() => this.hideSuggestions(), 120));
    input.addEventListener('keydown', (event) => this.onKeyDown(event, input));
  }

  candidates(prefix) {
    const lower = prefix.toLowerCase();
    const columns = this.extraNames
      .filter((name) => name.toLowerCase().startsWith(lower))
      .map((name) => ({ name, signature: name, doc: 'column', insert: name }));
    const functions = this.functions
      .filter((f) => f.name.toLowerCase().startsWith(lower))
      .map((f) => ({
        name: f.name,
        signature: f.signature,
        doc: f.doc,
        insert: f.kind === 'matype' ? f.name : `${f.name}(`,
      }));
    return [...columns, ...functions].slice(0, 40);
  }

  showSuggestions(input) {
    const before = input.value.slice(0, input.selectionStart ?? input.value.length);
    const match = before.match(IDENT);
    if (!match || match[0].length < 1) {
      this.hideSuggestions();
      return;
    }

    const items = this.candidates(match[0]);
    if (!items.length || (items.length === 1 && items[0].name === match[0])) {
      this.hideSuggestions();
      return;
    }

    this.active = 0;
    this.items = items;
    this.target = input;
    this.prefixLength = match[0].length;

    const frag = document.createDocumentFragment();
    items.forEach((item, i) => {
      const node = el('div', { class: i === 0 ? 'ac-item active' : 'ac-item' },
        el('code', {}, item.signature),
        el('span', {}, item.doc));
      node.addEventListener('mousedown', (event) => {
        event.preventDefault();
        this.accept(i);
      });
      frag.append(node);
    });
    this.popup.replaceChildren(frag);

    // Positioned against the viewport: the config panel scrolls independently
    // of the page, so page coordinates would drift.
    const rect = input.getBoundingClientRect();
    this.popup.style.left = `${rect.left}px`;
    this.popup.style.top = `${rect.bottom + 2}px`;
    this.popup.style.width = `${Math.max(rect.width, 260)}px`;
    this.popup.hidden = false;
  }

  hideSuggestions() {
    this.popup.hidden = true;
    this.items = null;
    this.target = null;
  }

  onKeyDown(event, input) {
    if (this.popup.hidden || !this.items || this.target !== input) return;

    switch (event.key) {
      case 'ArrowDown':
        event.preventDefault();
        this.move(1);
        break;
      case 'ArrowUp':
        event.preventDefault();
        this.move(-1);
        break;
      case 'Enter':
      case 'Tab':
        event.preventDefault();
        this.accept(this.active);
        break;
      case 'Escape':
        this.hideSuggestions();
        break;
      default:
        break;
    }
  }

  move(delta) {
    const nodes = [...this.popup.children];
    nodes[this.active]?.classList.remove('active');
    this.active = (this.active + delta + nodes.length) % nodes.length;
    nodes[this.active]?.classList.add('active');
    nodes[this.active]?.scrollIntoView({ block: 'nearest' });
  }

  accept(index) {
    const item = this.items?.[index];
    const input = this.target;
    if (!item || !input) return;

    const caret = input.selectionStart ?? input.value.length;
    const head = input.value.slice(0, caret - this.prefixLength);
    const tail = input.value.slice(caret);
    input.value = head + item.insert + tail;

    const position = head.length + item.insert.length;
    input.setSelectionRange(position, position);
    this.hideSuggestions();
    input.dispatchEvent(new Event('input', { bubbles: true }));
  }
}
