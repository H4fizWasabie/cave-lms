// The check of charts.js's computing functions, run by charts_test.go when node is installed.
//   node charts_test.js                       checks every block option on made-up rows
//   node charts_test.js themes                prints the keys of the Themes it draws
//   node charts_test.js index.html data.json  draws that Dashboard and prints its style, cards and charts as JSON
'use strict';
var fs = require('fs'), vm = require('vm'), assert = require('assert');

var els = {};
function el() {
  return { style: { setProperty: function (k, v) { this[k] = v; } }, addEventListener: function () {},
    querySelector: function () { return null; }, querySelectorAll: function () { return []; }, clientWidth: 800 };
}
var style = '';
var document = { head: { appendChild: function (e) { style += e.textContent || ''; } }, createElement: el,
  getElementById: function (id) { return els[id] = els[id] || el(); } };
var window = {};
vm.runInNewContext(fs.readFileSync(__dirname + '/charts.js', 'utf8'),
  { window: window, document: document, addEventListener: function () {}, innerWidth: 1000 });

if (process.argv[2] === 'themes') {
  console.log(window.drawDashboard.themes.join(' '));
  return;
}
if (process.argv[2] === 'icons') {
  console.log(window.drawDashboard.icons.join(' '));
  return;
}
if (process.argv[2]) {
  var page = fs.readFileSync(process.argv[2], 'utf8');
  var dash = JSON.parse(page.match(/const dashboard = (\{[\s\S]*?\n    \});/)[1]);
  window.drawDashboard(dash, JSON.parse(fs.readFileSync(process.argv[3], 'utf8')));
  console.log(JSON.stringify({ style: style, cards: els.cards.innerHTML, cardCols: els.cards.style['--cols'] || '', charts: els.charts.innerHTML }));
  return;
}

var C = window.drawDashboard.compute;
var cols = { Tarikh: { type: 'date' }, Staf: { type: 'text' }, Kategori: { type: 'text' }, Jumlah: { type: 'number' },
  Kos: { type: 'number' }, Pelanggan: { type: 'text' } };
var rows = [
  { Tarikh: '2026-01-05', Staf: 'Ali', Kategori: 'Ubat', Jumlah: 100, Kos: 40, Pelanggan: 'P1' },
  { Tarikh: '2026-01-05', Staf: 'Ali', Kategori: 'Servis', Jumlah: 50, Kos: null, Pelanggan: 'P2' },
  { Tarikh: '2026-01-20', Staf: 'Siti', Kategori: 'Ubat', Jumlah: null, Kos: 10, Pelanggan: 'P1' },
  { Tarikh: '2026-02-03', Staf: null, Kategori: 'Ubat', Jumlah: 30, Kos: 20, Pelanggan: null },
  { Tarikh: null, Staf: 'Siti', Kategori: 'Servis', Jumlah: 20, Kos: 5, Pelanggan: 'P3' },
  { Tarikh: '2026-02-10', Staf: 'Ali', Kategori: 'Servis', Jumlah: 8, Kos: 0, Pelanggan: 'P1' }
];
function eq(got, want, what) { assert.deepStrictEqual(JSON.parse(JSON.stringify(got)), want, what); }
function card(value, extra) { var b = { kind: 'card', value: value }; for (var k in extra) b[k] = extra[k]; return C.cardValue(b, rows, cols); }

// Values: empty cells count as rows and are skipped in sums, averages and distinct counts.
eq(card({ agg: 'count' }), 6, 'count');
eq(card({ agg: 'sum', col: 'Jumlah' }), 208, 'sum');
eq(card({ agg: 'avg', col: 'Jumlah' }), 41.6, 'avg');
eq(card({ agg: 'distinct', col: 'Pelanggan' }), 3, 'distinct');
// A op B row by row, skipping rows with either cell empty, and division by 0.
eq(card({ agg: 'calc', a: 'Jumlah', op: '+', b: 'Kos', then: 'sum' }), 140 + 50 + 25 + 8, 'calc +');
eq(card({ agg: 'calc', a: 'Jumlah', op: '-', b: 'Kos', then: 'sum' }), 60 + 10 + 15 + 8, 'calc -');
eq(card({ agg: 'calc', a: 'Jumlah', op: 'x', b: 'Kos', then: 'sum' }), 4000 + 600 + 100, 'calc x');
eq(card({ agg: 'calc', a: 'Jumlah', op: '/', b: 'Kos', then: 'sum' }), 2.5 + 1.5 + 4, 'calc /');
eq(card({ agg: 'calc', a: 'Jumlah', op: '-', b: 'Kos', then: 'avg' }), 93 / 4, 'calc then avg');

// One filter per block; a ticked (blank) matches empty cells.
eq(card({ agg: 'count' }, { filter: { col: 'Staf', vals: ['Siti'] } }), 2, 'filter');
eq(card({ agg: 'count' }, { filter: { col: 'Staf', vals: ['Siti', '(blank)'] } }), 3, 'filter with (blank)');
eq(card({ agg: 'count' }, { filter: { col: 'Staf', vals: [] } }), rows.length, 'a filter with nothing ticked keeps every row');
eq(C.noMatch({ filter: { col: 'Staf', vals: ['Dr Farah'] } }, rows, cols), true, 'a filter value not in the Dataset matches no rows');
eq(C.noMatch({ filter: { col: 'Staf', vals: ['Ali'] } }, rows, cols), false, 'a filter value in the Dataset');
eq(C.noMatch({}, [], cols), false, 'no filter is never "No rows match"');
// Locked Template blocks keep their calculation: Balance, and Low stock.
eq(card({ agg: 'sum', col: 'Jumlah' }, { where: ['Kategori', 'Ubat'], minus: ['Kategori', 'Servis'] }), 130 - 78, 'balance');
eq(card({ agg: 'count' }, { low: ['Kos', 'Jumlah'] }), 4, 'low');

// Line: by month (the default) or by day, empty dates left off the axis; split gives one line per value.
eq(C.timeline({ value: { agg: 'count' }, x: 'Tarikh', label: 'Rows' }, rows, cols),
  { keys: ['2026-01', '2026-02'], series: { Rows: { '2026-01': 3, '2026-02': 2 } } }, 'by month');
eq(C.timeline({ value: { agg: 'count' }, x: 'Tarikh', by: 'day', label: 'Rows' }, rows, cols).series.Rows,
  { '2026-01-05': 2, '2026-01-20': 1, '2026-02-03': 1, '2026-02-10': 1 }, 'by day');
eq(C.timeline({ value: { agg: 'sum', col: 'Jumlah' }, x: 'Tarikh', split: 'Staf' }, rows, cols).series,
  { Ali: { '2026-01': 150, '2026-02': 8 }, Siti: { '2026-01': 0 }, '(blank)': { '2026-02': 30 } }, 'split');

// Bar: largest first, top 8 by default when Practice, top N or all (0) when Work; split stacks each bar.
var bar = { value: { agg: 'sum', col: 'Jumlah' }, group: 'Staf' };
eq(C.bars(bar, rows, cols).map(function (g) { return g.slice(0, 2); }), [['Ali', 158], ['(blank)', 30], ['Siti', 20]], 'bar, all');
bar.top = 2;
eq(C.bars(bar, rows, cols).map(function (g) { return g[0]; }), ['Ali', '(blank)'], 'bar top 2');
bar.split = 'Kategori';
eq(C.bars(bar, rows, cols)[0], ['Ali', 158, [['Servis', 58], ['Ubat', 100]]], 'bar split');

// Heatmap: Group down, Split across, both largest first; a cell with no rows is null.
var heat = { kind: 'heatmap', value: { agg: 'count' }, group: 'Staf', split: 'Kategori', top: 8 };
eq(C.heat(heat, rows, cols), { rows: ['Ali', 'Siti', '(blank)'], cols: ['Ubat', 'Servis'], cells: [[1, 2], [1, 1], [1, null]] }, 'heatmap count');
heat.value = { agg: 'sum', col: 'Jumlah' };
eq(C.heat(heat, rows, cols), { rows: ['Ali', '(blank)', 'Siti'], cols: ['Ubat', 'Servis'], cells: [[100, 58], [30, null], [0, 20]] }, 'heatmap sum');
heat.top = 1;
eq(C.heat(heat, rows, cols), { rows: ['Ali'], cols: ['Ubat', 'Servis'], cells: [[100, 58]] }, 'heatmap top keeps rows; columns from the rows kept');
heat.top = 8; heat.filter = { col: 'Kategori', vals: ['Servis'] };
eq(C.heat(heat, rows, cols), { rows: ['Ali', 'Siti'], cols: ['Servis'], cells: [[58], [20]] }, 'heatmap filter');
// Past the 10 largest columns, the rest fold into one Other.
var wide = 'ABCDEFGHIJKL'.split('').reduce(function (a, k, i) { for (var n = 0; n < 12 - i; n++) a.push({ R: n % 2 ? 'x' : 'y', S: k }); return a; }, []);
var h = C.heat({ value: { agg: 'count' }, group: 'R', split: 'S' }, wide, { R: { type: 'text' }, S: { type: 'text' } });
eq(h.cols, 'ABCDEFGHIJ'.split('').concat(['Other']), 'heatmap columns past 10 fold into Other');
eq(h.rows, ['y', 'x'], 'heatmap rows');
var h11 = C.heat({ value: { agg: 'count' }, group: 'R', split: 'S' }, wide.filter(function (r) { return r.S !== 'L'; }), { R: { type: 'text' }, S: { type: 'text' } });
eq(h11.cols, 'ABCDEFGHIJ'.split('').concat(['Other']), 'heatmap folds an eleventh column too');
eq([h.cells[0][10], h.cells[1][10]], [2, 1], 'heatmap Other cells');

// Donut: a Work donut folds everything past the top 6 into one Other slice; a Practice donut, past the top 5, as before.
var many = 'ABCDEFGH'.split('').reduce(function (a, k, i) { for (var n = 0; n < 8 - i; n++) a.push({ G: k }); return a; }, []);
eq(C.slices({ kind: 'donut', value: { agg: 'count' }, group: 'G' }, many, { G: { type: 'text' } }),
  [['A', 8], ['B', 7], ['C', 6], ['D', 5], ['E', 4], ['F', 3], ['Other', 3]], 'donut Other past 6');
eq(C.slices({ kind: 'donut', value: { agg: 'count' }, group: 'G' }, many.filter(function (r) { return r.G !== 'H'; }), { G: { type: 'text' } }).length,
  7, 'donut of 7 groups shows all 7');
eq(C.slices(C.fromPractice({ kind: 'donut', agg: 'count', group: 'G' }), many, { G: { type: 'text' } }),
  [['A', 8], ['B', 7], ['C', 6], ['D', 5], ['E', 4], ['Other', 6]], 'Practice donut Other past 5');
eq(C.slices({ value: { agg: 'count' }, group: 'Staf' }, rows, cols), [['Ali', 3], ['Siti', 2], ['(blank)', 1]], 'donut (blank)');

// Table: newest or largest first, empty cells last, 10, 25 or 50 rows.
eq(C.tableRows({ sort: 'Jumlah', limit: 3 }, rows, cols).map(function (r) { return r.Jumlah; }), [100, 50, 30], 'largest first');
eq(C.tableRows({ sort: 'Tarikh', limit: 10 }, rows, cols).map(function (r) { return r.Tarikh; }),
  ['2026-02-10', '2026-02-03', '2026-01-20', '2026-01-05', '2026-01-05', null], 'newest first, blanks last');

// Download CSV: the table's columns by name, its filter and sort but not its limit, numbers raw, dates as
// YYYY-MM-DD, empty cells empty, quoted per RFC 4180, with a BOM so Excel reads it as UTF-8.
var named = { Tarikh: { name: 'Tarikh', type: 'date' }, Kategori: { name: 'Kategori', type: 'text' }, Staf: { name: 'Nama staf', type: 'text' }, Jumlah: { name: 'Jumlah', type: 'number', money: true } };
var odd = rows.concat([{ Tarikh: '2026-03-01T00:00:00Z', Staf: 'Wan "Abu", Jr\nKL', Jumlah: 1234.5 }]);
eq(C.csv({ cols: ['Staf', 'Jumlah', 'Tarikh', 'Gone'], sort: 'Jumlah', limit: 2, filter: { col: 'Kategori', vals: ['Ubat'] } }, odd, named),
  '﻿Nama staf,Jumlah,Tarikh\r\nAli,100,2026-01-05\r\n,30,2026-02-03\r\nSiti,,2026-01-20\r\n', 'csv: filter, sort, no limit, raw, empty');
eq(C.csv({ cols: ['Staf', 'Jumlah', 'Tarikh'], sort: 'Jumlah', limit: 1 }, odd, named).split('\r\n')[1],
  '"Wan ""Abu"", Jr\nKL",1234.5,2026-03-01', 'csv quoting, raw number, date');
eq(C.csv({ cols: ['Staf'] }, [], named), '﻿Nama staf\r\n', 'csv of no rows is its header');

// A block naming a column the Dataset lacks is paused.
eq(C.paused({ kind: 'bar', value: { agg: 'sum', col: 'Jumlah' }, group: 'Staf' }, cols), false, 'not paused');
eq(C.paused({ kind: 'card', value: { agg: 'calc', a: 'Jumlah', op: 'x', b: 'Harga', then: 'sum' } }, cols), true, 'paused by value');
eq(C.paused({ kind: 'line', value: { agg: 'count' }, x: 'Tarikh', split: 'Cawangan' }, cols), true, 'paused by split');
eq(C.paused({ kind: 'table', cols: ['Tarikh', 'Nota'], sort: 'Tarikh' }, cols), true, 'paused by table column');
eq(C.paused({ kind: 'card', value: { agg: 'count' }, filter: { col: 'Doktor', vals: ['A'] } }, cols), true, 'paused by filter');
eq(C.paused({ kind: 'card', value: { agg: 'count' }, low: ['Kos', 'Paras'] }, cols), true, 'paused by low');
// A column of the same name and another type is as good as missing.
eq(C.paused({ kind: 'card', value: { agg: 'sum', col: 'Staf' } }, cols), true, 'paused by a retyped value column');
eq(C.paused({ kind: 'bar', value: { agg: 'count' }, group: 'Jumlah' }, cols), true, 'paused by a retyped group');
eq(C.paused({ kind: 'line', value: { agg: 'count' }, x: 'Staf' }, cols), true, 'paused by a retyped date');
eq(C.paused({ kind: 'card', value: { agg: 'count' }, where: ['Kos', 'Masuk'] }, cols), true, 'paused by a retyped lock column');
eq(C.paused({ kind: 'card', value: { agg: 'distinct', col: 'Jumlah' } }, cols), false, 'distinct counts any type');
eq(C.paused({ kind: 'table', cols: ['Jumlah', 'Staf'], sort: 'Kos' }, cols), false, 'a table shows any type');

// A card's icon: the one it names, none, or with no icon (a card saved before icons) the one its value suggests.
var money = { Jumlah: { type: 'number', money: true }, Kos: { type: 'number' }, Staf: { type: 'text' } };
eq(C.iconOf({ icon: 'star', value: { agg: 'count' } }, money), 'star', 'named icon');
eq(C.iconOf({ icon: 'none', value: { agg: 'count' } }, money), '', 'no icon');
eq(C.iconOf({ value: { agg: 'count' } }, money), 'people', 'count: people');
eq(C.iconOf({ value: { agg: 'sum', col: 'Jumlah' } }, money), 'money', 'sum of RM: money');
eq(C.iconOf({ value: { agg: 'avg', col: 'Jumlah' } }, money), 'money', 'average of RM: money');
eq(C.iconOf({ value: { agg: 'sum', col: 'Kos' } }, money), 'chart-up', 'sum of a plain number: chart-up');
eq(C.iconOf({ value: { agg: 'distinct', col: 'Staf' } }, money), 'tag', 'distinct: tag');
eq(C.iconOf({ value: { agg: 'calc', a: 'Jumlah', op: '-', b: 'Kos', then: 'sum' } }, money), 'chart-up', 'calculation: chart-up');

// A Practice block reads as the same Work block.
eq(C.fromPractice({ kind: 'card', label: 'Stock value', agg: 'sum', of: 'q', times: 'p' }).value,
  { agg: 'calc', a: 'q', op: 'x', b: 'p', then: 'sum' }, 'practice times');
eq(C.fromPractice({ kind: 'card', label: 'Low stock', icon: 'alert', agg: 'count' }).icon, 'alert', 'practice icon');
eq(C.fromPractice({ kind: 'bar', group: 'c', of: 'a', where: ['t', 'Keluar'] }),
  { kind: 'bar', value: { agg: 'sum', col: 'a' }, group: 'c', top: 8, where: ['t', 'Keluar'] }, 'practice bar');

// A vertical bar draws columns from the same groups, the largest in the Theme's highlight colour, its value on
// top and its full name on hover; a split one stacks in the Theme's stack colours.
var named = {};
Object.keys(cols).forEach(function (k) { named[k] = { name: k, type: cols[k].type }; });
window.drawDashboard({ work: true, theme: 'corporate', columns: named, blocks: [
  { kind: 'bar', label: 'Tegak', dir: 'vertical', value: { agg: 'sum', col: 'Jumlah' }, group: 'Staf', wide: true },
  { kind: 'bar', label: 'Tindan', dir: 'vertical', value: { agg: 'sum', col: 'Jumlah' }, group: 'Staf', split: 'Kategori', wide: true }] }, rows);
var drawn = els.charts.innerHTML.split('<article');
[['<div class="dcols">', 'columns'], ['Top: Ali, 158', 'finding'], ['<span class="dcol-v">158</span><span class="dcol-t" style="height:100.0%"><span style="height:100%;background:#C89B3C"', 'largest in the highlight colour'],
  ['<span class="dcol-v">20</span><span class="dcol-t" style="height:12.7%"><span style="height:100%;background:#0E2A47"', 'the rest in the bar colour'],
  ['<span class="dcol-k" title="(blank)">(blank)</span>', 'name under the axis']].forEach(function (w) { assert(drawn[1].indexOf(w[0]) >= 0, 'vertical ' + w[1] + ':\n' + drawn[1]); });
[['<span style="height:36.7%;background:#2E8B6F" title="Servis: 58"></span><span style="height:63.3%;background:#C89B3C" title="Ubat: 100"></span>', 'stacked'],
  ['<i style="background:#2E8B6F"></i>Servis', 'legend']].forEach(function (w) { assert(drawn[2].indexOf(w[0]) >= 0, 'vertical ' + w[1] + ':\n' + drawn[2]); });

console.log('ok');
