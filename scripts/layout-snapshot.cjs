// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.
//
// Layout snapshot: records every box and every type property the shell draws
// on its reference surfaces, so a stylesheet or markup change shows exactly
// what it moved (ADR-025). Run through make, against the dev container:
//
//   make layout-snapshot LAYOUT_OUT=build/layout/before
//   make layout-snapshot LAYOUT_OUT=build/layout/after
//   make layout-diff FROM=build/layout/before TO=build/layout/after
//
// The reference set is found, not listed: every page the rail links to and
// every page those link to, the first drawer of each kind a page opens through
// its ?open= links, the sign-in page, the self-contained pages written to
// build/layout-fixtures, and the interactions scripts/layout-reference.json
// names (a device's tabs, a confirm, Arrange). Each is recorded at 1440px and
// at 390px wide. A live list the reference names under "trim" (the system
// log) is cut to its first rows before measuring, since its length is live.
//
// What a record holds, per element: its border box, margins, paddings,
// borders, radius, inks, shadow and type (family, size, weight, line height,
// tracking, numerals, case). An element is keyed by its path of tags and
// sibling positions, never by class or attribute, so renaming a class or
// marking a role is not a change; adding or removing an element is. Before
// measuring, every run of text holding a figure is written as a single 0 and
// motion is switched off, so live readings (an uptime that gains a unit, a rate
// that gains a digit, and the table columns that reflow around them) and a
// transition caught half-way do not read as moves. A 0 still takes its face,
// size and weight, so a type change still shows. Inside an <svg> only the svg's own box is
// kept: a chart redraws its marks from live data. LAYOUT_EXTRA_CSS, when set, is
// laid over every surface, to check that a diff names what a rule moves.
'use strict';

const fs = require('node:fs');
const path = require('node:path');
const http = require('node:http');

const BASE = process.env.VERSO_URL || 'http://localhost:8080';
const USER = process.env.VERSO_USER || 'root';
const PASSWORD = process.env.VERSO_PASSWORD || '12345678';
const WIDTHS = [[1440, 900], [390, 844]];
const ROOT = path.join(__dirname, '..');
const FIXTURES = path.join(ROOT, 'build', 'layout-fixtures');
const REFERENCE = path.join(__dirname, 'layout-reference.json');
const SESSION = path.join(ROOT, 'build', 'layout', 'session.json');
// NOISE lists, per surface, the elements two records of an unchanged app
// disagreed on (live state: a device going idle, a rate appearing). A diff
// leaves them out; `noise` adds to it.
const NOISE = path.join(ROOT, 'build', 'layout', 'noise.json');

// measure runs in the page. It is one synchronous pass, so nothing the page
// streams in can land between the digits being written and the boxes read.
function measure(trims) {
  for (const { selector, keep } of trims) {
    for (const list of document.querySelectorAll(selector)) {
      while (list.children.length > keep) list.lastElementChild.remove();
    }
  }
  const walker = document.createTreeWalker(document.body, NodeFilter.SHOW_TEXT);
  for (let n = walker.nextNode(); n; n = walker.nextNode()) {
    const p = n.parentElement;
    if (p && (p.closest('script, style, template'))) continue;
    if (/[0-9]/.test(n.data)) n.data = '0';
  }
  const props = [
    'display', 'position',
    'margin-top', 'margin-right', 'margin-bottom', 'margin-left',
    'padding-top', 'padding-right', 'padding-bottom', 'padding-left',
    'border-top-width', 'border-right-width', 'border-bottom-width', 'border-left-width',
    'border-top-color', 'border-right-color', 'border-bottom-color', 'border-left-color',
    'border-top-left-radius', 'color', 'background-color', 'box-shadow', 'opacity',
    'font-family', 'font-size', 'font-weight', 'line-height', 'letter-spacing',
    'font-variant-numeric', 'text-transform', 'font-style',
  ];
  const out = {};
  const r1 = (v) => Math.round(v * 10) / 10;
  const visit = (el, key) => {
    const cs = getComputedStyle(el);
    if (cs.display === 'none') return;
    const b = el.getBoundingClientRect();
    const rec = {
      _label: (el.tagName.toLowerCase() + ' ' + (el.getAttribute('class') || '').split(/\s+/).filter((c) => c.startsWith('verso-')).join(' ') + ' ' + (el.textContent || '').trim().replace(/\s+/g, ' ').slice(0, 32)).trim(),
      _text: [...el.childNodes].some((n) => n.nodeType === 3 && n.data.trim() !== ''),
      box: [r1(b.left + scrollX), r1(b.top + scrollY), r1(b.width), r1(b.height)].join(' '),
    };
    for (const p of props) rec[p] = cs.getPropertyValue(p);
    rec['font-family'] = rec['font-family'].split(',')[0].trim();
    out[key] = rec;
    if (el instanceof SVGElement) return;
    const seen = {};
    for (const c of el.children) {
      const t = c.tagName.toLowerCase();
      if (t === 'script' || t === 'style' || t === 'template' || t === 'link' || t === 'meta') continue;
      seen[t] = (seen[t] || 0) + 1;
      visit(c, key + '/' + t + ':' + seen[t]);
    }
  };
  visit(document.body, 'body');
  return out;
}

const QUIET = '*, *::before, *::after { transition: none !important; animation: none !important; caret-color: transparent !important; scroll-behavior: auto !important; }';

function slug(name) {
  return name.replace(/^\//, '').replace(/[^a-zA-Z0-9]+/g, '-').replace(/^-|-$/g, '') || 'overview';
}

async function settle(page) {
  await page.waitForLoadState('networkidle', { timeout: 2500 }).catch(() => {});
  await page.addStyleTag({ content: QUIET + (process.env.LAYOUT_EXTRA_CSS || '') });
  await page.evaluate(() => document.fonts.ready);
  await page.waitForTimeout(250);
}

async function record(page, outDir, name, width, trims = []) {
  const file = path.join(outDir, `${slug(name)}@${width}`);
  const data = await page.evaluate(measure, trims);
  fs.writeFileSync(file + '.json', JSON.stringify(data, null, 1));
  await page.screenshot({ path: file + '.png', fullPage: true });
}

// signIn reuses the session the last run kept, so runs do not pile sessions
// up on Access's "Signed in now" list, and signs in afresh only when it ended.
async function signIn(browser, opts) {
  const kept = fs.existsSync(SESSION) ? SESSION : undefined;
  const context = await browser.newContext({ ...opts, storageState: kept });
  const page = await context.newPage();
  await page.goto(BASE + '/');
  if (new URL(page.url()).pathname === '/login') {
    await page.fill('#username', USER);
    await page.fill('#password', PASSWORD);
    await Promise.all([page.waitForNavigation(), page.click('[data-login-submit]')]);
  }
  fs.mkdirSync(path.dirname(SESSION), { recursive: true });
  await context.storageState({ path: SESSION });
  return { context, page };
}

// discover reads the reference set off the app itself.
async function discover(page) {
  await page.goto(BASE + '/');
  const rail = await page.$$eval('a[href^="/"]', (as) => as.map((a) => a.getAttribute('href')));
  const pages = new Set(rail.filter((h) => !h.includes('?') && !h.includes('#')));
  const drawers = new Map();
  for (const u of [...pages]) {
    const res = await page.goto(BASE + u).catch(() => null);
    if (!res || !(res.headers()['content-type'] || '').includes('text/html')) continue;
    const hrefs = await page.$$eval('a[href^="/"]', (as) => as.map((a) => a.getAttribute('href')));
    for (const h of hrefs) {
      if (h.includes('?open=')) {
        const kind = h.replace(/(open=)(?!new)[^&]+/, '$1*').replace(/&.*$/, '');
        if (!drawers.has(kind)) drawers.set(kind, h);
      } else if (!h.includes('?') && !h.includes('#')) {
        pages.add(h);
      }
    }
  }
  return { pages: [...pages].sort(), drawers: [...drawers.values()].sort() };
}

// fixtures serves build/layout-fixtures, passing /assets/ through to the dev
// container so a fixture's fonts and icons are the app's own.
function fixtures() {
  if (!fs.existsSync(FIXTURES)) return null;
  const server = http.createServer((req, res) => {
    if (req.url.startsWith('/assets/')) {
      http.get(BASE + req.url, (up) => { res.writeHead(up.statusCode, up.headers); up.pipe(res); })
        .on('error', () => { res.writeHead(502); res.end(); });
      return;
    }
    const file = path.join(FIXTURES, path.normalize(req.url).replace(/^(\.\.[/\\])+/, ''));
    if (!file.startsWith(FIXTURES) || !fs.existsSync(file)) { res.writeHead(404); res.end(); return; }
    res.writeHead(200, { 'content-type': 'text/html; charset=utf-8' });
    fs.createReadStream(file).pipe(res);
  });
  return new Promise((resolve) => server.listen(0, '127.0.0.1', () => resolve(server)));
}

async function snap(outDir) {
  const { chromium } = require(process.env.PLAYWRIGHT_CORE || 'playwright-core');
  fs.mkdirSync(outDir, { recursive: true });
  const reference = fs.existsSync(REFERENCE) ? JSON.parse(fs.readFileSync(REFERENCE, 'utf8')) : {};
  const skip = new Set(reference.skip || []);
  const browser = await chromium.launch();
  const opts = { bypassCSP: true, reducedMotion: 'reduce', deviceScaleFactor: 1 };

  const guest = await browser.newContext(opts);
  const login = await guest.newPage();
  const server = await fixtures();
  for (const [w, h] of WIDTHS) {
    await login.setViewportSize({ width: w, height: h });
    await login.goto(BASE + '/login');
    await settle(login);
    await record(login, outDir, '/login', w);
    if (server) {
      const port = server.address().port;
      for (const f of fs.readdirSync(FIXTURES).filter((n) => n.endsWith('.html')).sort()) {
        await login.goto(`http://127.0.0.1:${port}/${f}`);
        await settle(login);
        await record(login, outDir, 'fixture-' + f.replace(/\.html$/, ''), w);
      }
    }
  }
  if (server) server.close();
  await guest.close();

  const { page } = await signIn(browser, opts);
  const found = await discover(page);
  const surfaces = [
    ...found.pages.filter((u) => !skip.has(u)).map((u) => ({ name: u, url: u })),
    ...found.drawers.filter((u) => !skip.has(u)).map((u) => ({ name: u.replace(/(open=)(?!new)[^&]+/, '$1one'), url: u, drawer: true })),
    ...(reference.interactions || []),
  ];
  let count = 0;
  for (const s of surfaces) {
    for (const [w, h] of WIDTHS) {
      await page.setViewportSize({ width: w, height: h });
      const res = await page.goto(BASE + s.url).catch(() => null);
      if (!res || !(res.headers()['content-type'] || '').includes('text/html')) break;
      if (s.drawer) await page.waitForSelector('dialog[open]', { timeout: 5000 }).catch(() => {});
      for (const step of s.click || []) {
        await page.click(step, { timeout: 5000 });
        await page.waitForTimeout(150);
      }
      await settle(page);
      await record(page, outDir, s.name, w, reference.trim || []);
      for (const step of s.undo || []) {
        await page.click(step, { timeout: 5000 }).catch(() => {});
        await page.waitForTimeout(150);
      }
      count++;
    }
  }
  await browser.close();
  console.log(`recorded ${count + WIDTHS.length} surfaces into ${outDir}`);
}

// compare yields, per surface file, the keys that differ between two records.
function* compare(fromDir, toDir) {
  const names = new Set([...fs.readdirSync(fromDir), ...fs.readdirSync(toDir)].filter((n) => n.endsWith('.json')));
  for (const n of [...names].sort()) {
    const fa = path.join(fromDir, n), fb = path.join(toDir, n);
    if (!fs.existsSync(fa) || !fs.existsSync(fb)) { yield { n, only: fs.existsSync(fa) ? fromDir : toDir }; continue; }
    yield { n, a: JSON.parse(fs.readFileSync(fa, 'utf8')), b: JSON.parse(fs.readFileSync(fb, 'utf8')) };
  }
}

function differs(x, y) {
  return Object.keys(x).some((p) => !p.startsWith('_') && x[p] !== y[p]);
}

// noise records what two records of the same app disagree on.
function noise(fromDir, toDir) {
  const known = fs.existsSync(NOISE) ? JSON.parse(fs.readFileSync(NOISE, 'utf8')) : {};
  let added = 0;
  for (const { n, a, b } of compare(fromDir, toDir)) {
    if (!a) continue;
    const set = new Set(known[n] || []);
    for (const k of Object.keys(a)) if (!(k in b) || differs(a[k], b[k])) { if (!set.has(k)) added++; set.add(k); }
    for (const k of Object.keys(b)) if (!(k in a)) { if (!set.has(k)) added++; set.add(k); }
    if (set.size) known[n] = [...set].sort();
  }
  fs.writeFileSync(NOISE, JSON.stringify(known, null, 1));
  console.log(`${added} noisy element(s) added to ${NOISE}`);
}

// diff prints what moved between two records, surface by surface, leaving out
// known noise, and exits 1 when anything did.
function diff(fromDir, toDir, limit = 25) {
  const known = fs.existsSync(NOISE) ? JSON.parse(fs.readFileSync(NOISE, 'utf8')) : {};
  let changed = 0;
  for (const { n, a, b, only } of compare(fromDir, toDir)) {
    if (only) {
      console.log(`${n}: only in ${only}`);
      changed++;
      continue;
    }
    const quiet = new Set(known[n] || []);
    // An element whose own style changed is a cause; one whose box alone moved
    // or resized follows from a cause, and is counted rather than listed.
    const lines = [];
    let shifted = 0;
    for (const k of new Set([...Object.keys(a), ...Object.keys(b)])) {
      if (quiet.has(k)) continue;
      if (!(k in a)) { lines.push(`  + ${b[k]._label}  (${k})`); continue; }
      if (!(k in b)) { lines.push(`  - ${a[k]._label}  (${k})`); continue; }
      const moved = Object.keys(a[k]).filter((p) => !p.startsWith('_') && a[k][p] !== b[k][p]);
      if (!moved.length) continue;
      if (moved.length === 1 && moved[0] === 'box') { shifted++; continue; }
      lines.push(`  ~ ${b[k]._label}  (${k})\n      ` + moved.map((p) => `${p}: ${a[k][p]} -> ${b[k][p]}`).join('\n      '));
    }
    if (lines.length || shifted) {
      changed++;
      console.log(`${n}: ${lines.length} element(s) restyled, ${shifted} moved or resized by them`);
      console.log(lines.slice(0, limit).join('\n'));
      if (lines.length > limit) console.log(`  … ${lines.length - limit} more`);
    }
  }
  if (changed) { console.log(`${changed} surface(s) differ`); process.exit(1); }
  console.log('no differences');
}

const [cmd, a, b] = process.argv.slice(2);
if (cmd === 'snap' && a) snap(a).catch((e) => { console.error(e); process.exit(2); });
else if (cmd === 'diff' && a && b) diff(a, b);
else if (cmd === 'noise' && a && b) noise(a, b);
else { console.error('usage: layout-snapshot.cjs snap <dir> | diff <from> <to> | noise <a> <b>'); process.exit(2); }
