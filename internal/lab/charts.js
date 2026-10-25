/* Cave's chart library. It draws a Dashboard from the column choices and blocks Cave wrote into
   index.html and the rows in data.json: summary cards, a line chart by month or day, bar and donut charts,
   a table, and a filter bar. It needs no other library and loads nothing from other websites.
   Every column name and cell is escaped before it reaches the page, so a cell shows as text. */
(function () {
  'use strict';

  // The look every Theme shares; a Theme sets the colours, corners and shadows through these variables.
  var CSS = [
    '*,*::before,*::after{box-sizing:border-box}',
    'body{margin:0;min-height:100vh;background:var(--bg);color:var(--ink);font:15px/1.5 Inter,ui-sans-serif,system-ui,-apple-system,"Segoe UI",Roboto,Arial,sans-serif}',
    'main{max-width:76rem;margin:0 auto;padding:1.6rem 1.4rem 1rem}',
    'h1,h2{margin:0;line-height:1.2;font-weight:600}',
    'select{font:inherit;padding:.35rem .5rem;border:1px solid var(--line);border-radius:6px;background:var(--solid);color:inherit;max-width:11rem}',
    '.dh{display:flex;flex-wrap:wrap;gap:1rem 2rem;justify-content:space-between;align-items:flex-end;margin-bottom:1rem;padding:1.1rem 1.3rem;background:var(--head);border:1px solid var(--line);border-radius:var(--r);box-shadow:var(--shadow)}',
    '.dh h1{font-size:1.6rem;letter-spacing:-.015em;color:var(--num)}',
    '.dh p{color:var(--muted);font-size:.88rem;margin:.2rem 0 0}',
    '.dh .dq{color:var(--ink);font-size:1rem;font-weight:600}',
    '.df{display:flex;flex-wrap:wrap;gap:.6rem .9rem}',
    '.df label{display:grid;gap:.2rem;font-size:.78rem;font-weight:600;color:var(--muted)}',
    '.dcards{display:grid;grid-template-columns:var(--cols,1fr);gap:.8rem;margin-bottom:.8rem}',
    '.dcard,.dpanel{background:var(--card);border:1px solid var(--line);border-radius:var(--r);box-shadow:var(--shadow);min-width:0}',
    '.dcard{position:relative;padding:.95rem 1.1rem;display:grid;gap:.2rem;overflow:hidden}',
    '.dcard::before{content:"";position:absolute;left:0;top:.9rem;bottom:.9rem;width:3px;border-radius:0 3px 3px 0;background:var(--kpi)}',
    '.dcard span{font-size:.8rem;color:var(--muted);font-weight:600}',
    '.dcard strong{overflow-wrap:anywhere;font-size:1.6rem;font-weight:600;color:var(--num);font-variant-numeric:tabular-nums;letter-spacing:-.02em}',
    '.dcard:first-child strong{font-size:2.2rem}',
    '.dcard.is-pos strong{color:var(--pos)}.dcard.is-neg strong{color:var(--neg)}',
    '.dgrid{display:grid;grid-template-columns:1fr 1fr;gap:.8rem}',
    '.dpanel{padding:1rem 1.15rem}',
    '.dpanel.wide{grid-column:1/-1}',
    '.dpanel h2{font-size:.98rem;margin-bottom:.7rem;color:var(--ink)}',
    '.dpanel .dfind{margin:-.5rem 0 .7rem;font-size:.84rem;color:var(--muted)}',
    '.dchart{width:100%;height:auto}.dchart text{font-size:11px;fill:var(--axis)}.dchart .grid{stroke:var(--grid)}',
    '.dlegend{display:flex;flex-wrap:wrap;gap:.3rem 1rem;font-size:.82rem;color:var(--muted);margin:-.3rem 0 .5rem}',
    '.dlegend span,.ddonut li{display:flex;align-items:center;gap:.4rem}',
    '.dlegend i,.ddonut i{width:.7rem;height:.7rem;border-radius:3px;display:inline-block;flex-shrink:0}',
    '.dbars{list-style:none;margin:0;padding:0;display:grid;gap:.55rem}',
    '.dbars li{display:grid;grid-template-columns:minmax(6rem,9rem) 1fr 4rem;gap:.7rem;align-items:center;font-size:.86rem}',
    '.dbar-k{overflow:hidden;text-overflow:ellipsis;white-space:nowrap}',
    '.dbar-t{height:.8rem;background:var(--track);border-radius:99px;overflow:hidden}',
    '.dbar-t span{display:block;height:100%;border-radius:99px}',
    '.dbar-t.split{display:flex}.dbar-t.split span{border-radius:0}',
    '.dbar-v{text-align:right;font-variant-numeric:tabular-nums;color:var(--ink);font-weight:600}',
    '.ddonut{display:flex;gap:1.2rem;align-items:center}.ddonut svg{width:9.5rem;flex-shrink:0}',
    '.ddonut .dtot{font-size:19px;font-weight:600;fill:var(--num)}',
    '.ddonut ul{list-style:none;margin:0;padding:0;display:grid;gap:.4rem;font-size:.86rem;flex:1}',
    '.ddonut li span{margin-left:auto;color:var(--muted);font-variant-numeric:tabular-nums}',
    '.dtable{overflow-x:auto}.dtable table{width:100%;border-collapse:collapse;font-size:.86rem}',
    '.dtable th{text-align:left;font-size:.74rem;text-transform:uppercase;letter-spacing:.05em;color:var(--muted);padding:.45rem .6rem;border-bottom:1px solid var(--line);white-space:nowrap}',
    '.dtable td{padding:.5rem .6rem;border-bottom:1px solid var(--soft);white-space:nowrap}',
    '.dtable .num{text-align:right;font-variant-numeric:tabular-nums}',
    '.dnone{color:var(--muted);font-size:.88rem;padding:1rem 0;text-align:center}',
    '.dcard strong.dnone{font-size:.88rem;font-weight:400;padding:0;text-align:left}',
    '@media (max-width:640px){main{padding:1.1rem .9rem .6rem}.dh h1{font-size:1.3rem}.df{width:100%}.df label{flex:1 1 8rem}' +
      '.df select{max-width:none;width:100%}.dcards{grid-template-columns:1fr 1fr;gap:.6rem}.dcard:first-child,.dcard:last-child:nth-child(even){grid-column:span 2}.dcard strong{font-size:1.25rem}.dcard:first-child strong{font-size:1.7rem}' +
      '.dgrid{grid-template-columns:1fr}.dbars li{grid-template-columns:6.5rem 1fr 3.4rem}.ddonut svg{width:7.5rem}}',
    // Printed on white whatever the Theme, without the filter bar.
    '@media print{body{background:#fff!important;color:#111}main{max-width:none;padding:0}.df{display:none}.dh,.dcard,.dpanel{background:#fff!important;border-color:#ccc!important;box-shadow:none!important;color:#111;backdrop-filter:none!important;-webkit-backdrop-filter:none!important;break-inside:avoid}' +
      '.dh h1,.dh .dq,.dcard strong,.dpanel h2,.dbar-v,.ddonut .dtot{color:#111!important;fill:#111!important}.dh p,.dcard span,.dfind,.dlegend,.dtable th,.dchart text{color:#444!important;fill:#444!important}}'
  ].join('\n');

  // The five Themes (the client's specs, kept in .scratch/dashboard-themes): CSS variables, an extra rule or two,
  // and the colours charts are drawn in. bar and top colour a bar chart, top marking its largest group; stack
  // colours the parts of a split bar; line and point a line of one series; series a line split into several.
  var THEMES = {
    corporate: {
      vars: { bg: '#F6F8FB', card: '#FFFFFF', solid: '#FFFFFF', head: '#FFFFFF', ink: '#13263B', num: '#0E2A47', muted: '#5E6E80', axis: '#7A8899',
        line: '#E3E8EF', soft: '#EEF1F5', grid: '#EDF0F4', track: '#EEF1F5', r: '12px', kpi: '#C89B3C', pos: '#2E8B6F', neg: '#C2553F',
        shadow: '0 1px 2px rgba(14,42,71,.05),0 6px 16px -8px rgba(14,42,71,.10)' },
      bar: '#0E2A47', top: '#C89B3C', line: '#0E2A47', point: '#C89B3C', area: 0.08,
      donut: ['#0E2A47', '#C89B3C', '#5B7FA6', '#E2C88F', '#9AA9BA', '#2E8B6F', '#C7CED8'],
      stack: ['#2E8B6F', '#C89B3C', '#D9725B', '#5B7FA6', '#9AA9BA'],
      series: ['#0E2A47', '#C89B3C', '#2E8B6F', '#D9725B', '#5B7FA6', '#9AA9BA']
    },
    midnight: {
      vars: { bg: '#081726', card: '#10263B', solid: '#10263B', head: '#06111E', ink: '#F5F7FA', num: '#F5F7FA', muted: '#A8BACB', axis: '#8DA2B6',
        line: 'rgba(255,255,255,.09)', soft: 'rgba(255,255,255,.05)', grid: 'rgba(255,255,255,.07)', track: 'rgba(255,255,255,.07)', r: '12px',
        kpi: '#D4AA55', pos: '#48A98A', neg: '#D97979', shadow: 'inset 0 1px 0 rgba(255,255,255,.04),0 12px 28px -14px rgba(0,0,0,.6)' },
      css: ':root{color-scheme:dark}',
      bar: '#3D7CC9', top: '#D4AA55', line: '#56B4E3', point: '#D4AA55', area: 0.12,
      donut: ['#D4AA55', '#3D7CC9', '#56B4E3', '#48A98A', '#8FA3B8', '#D97979', '#5A6B80'],
      stack: ['#48A98A', '#D4AA55', '#D97979', '#56B4E3', '#8FA3B8'],
      series: ['#56B4E3', '#D4AA55', '#48A98A', '#D97979', '#8FA3B8', '#3D7CC9']
    },
    minimal: {
      vars: { bg: '#FFFFFF', card: '#FFFFFF', solid: '#FFFFFF', head: '#FFFFFF', ink: '#20252B', num: '#20252B', muted: '#68737D', axis: '#8A939C',
        line: '#E5E9ED', soft: '#F0F2F4', grid: '#F0F2F4', track: '#F1F3F5', r: '6px', kpi: 'transparent', pos: '#2F7D5B', neg: '#B5473A', shadow: 'none' },
      css: '.dh{border:0;border-bottom:1px solid var(--line);border-radius:0;padding:0 0 1.1rem;margin-bottom:1.2rem}' +
        '.dcard span{text-transform:uppercase;letter-spacing:.06em;font-size:.72rem}.dcard:first-child strong{color:#3569A8}.dgrid,.dcards{gap:1rem}',
      bar: '#3569A8', top: '#3569A8', line: '#3569A8', point: '#3569A8', area: 0, thin: true,
      donut: ['#3569A8', '#A9C0DD', '#68737D', '#C9D0D7', '#2A9D8F', '#D9A03F', '#E5E9ED'],
      stack: ['#3569A8', '#2A9D8F', '#D9A03F', '#9AA5B1', '#C9D0D7'],
      series: ['#3569A8', '#2A9D8F', '#D9A03F', '#68737D', '#9AA5B1', '#B5473A']
    },
    glass: {
      vars: { bg: 'linear-gradient(140deg,#132B54 0%,#3859A6 55%,#6C5AA8 100%) fixed', card: 'rgba(255,255,255,.09)', solid: '#22417A', head: 'rgba(255,255,255,.07)',
        ink: '#FFFFFF', num: '#FFFFFF', muted: 'rgba(255,255,255,.76)', axis: 'rgba(255,255,255,.68)', line: 'rgba(255,255,255,.18)', soft: 'rgba(255,255,255,.10)',
        grid: 'rgba(255,255,255,.12)', track: 'rgba(255,255,255,.12)', r: '16px', kpi: '#57C8E8', pos: '#7FE0C0', neg: '#FFA3A3',
        shadow: 'inset 0 1px 0 rgba(255,255,255,.12),0 14px 34px -14px rgba(10,18,48,.5)' },
      // Blur where the browser has it; elsewhere solid panels, so text never sits on the bare gradient.
      css: ':root{color-scheme:dark}.dh,.dcard,.dpanel{-webkit-backdrop-filter:blur(14px) saturate(140%);backdrop-filter:blur(14px) saturate(140%)}' +
        '@supports not ((backdrop-filter:blur(1px)) or (-webkit-backdrop-filter:blur(1px))){.dh,.dcard,.dpanel{background:rgba(26,50,98,.94)}}',
      bar: 'linear-gradient(90deg,#3E8EE0,#57C8E8)', top: 'linear-gradient(90deg,#3E8EE0,#57C8E8)', line: '#57C8E8', point: '#D8F5FF', area: 0.14,
      donut: ['#57C8E8', '#A69DF2', '#D4AA55', '#7FE0C0', '#E6E9F5', '#8FA8E8', '#6C7AA8'],
      stack: ['#3CC6B5', '#A69DF2', '#D4AA55', '#57C8E8', '#E6E9F5'],
      series: ['#57C8E8', '#A69DF2', '#D4AA55', '#7FE0C0', '#E6E9F5', '#FFA3A3']
    },
    storytelling: {
      vars: { bg: '#FAFBFC', card: '#FFFFFF', solid: '#FFFFFF', head: '#FAFBFC', ink: '#18324A', num: '#18324A', muted: '#5A6E82', axis: '#7D8E9F',
        line: '#E4E9EF', soft: '#EEF2F6', grid: '#EEF2F6', track: '#EEF2F6', r: '14px', kpi: '#3478C0', pos: '#14907F', neg: '#C9562F',
        shadow: '0 1px 2px rgba(24,50,74,.04)' },
      // Each card in its own colour, and the finding a chart's computed one-line insight, set off under its name.
      css: '.dh{border:0;border-bottom:1px solid var(--line);border-radius:0;box-shadow:none;padding:0 0 1.1rem}' +
        '.dcard:nth-child(4n+2){--kpi:#18A999}.dcard:nth-child(4n+3){--kpi:#E89A45}.dcard:nth-child(4n+4){--kpi:#8267C7}.dcard::before{top:0;bottom:auto;left:0;right:0;width:auto;height:3px;border-radius:0}' +
        '.dpanel .dfind{margin:-.3rem 0 .8rem;padding:.45rem .7rem;border-left:3px solid #3478C0;border-radius:0 6px 6px 0;background:#EEF4FB;color:#18324A;font-weight:500}',
      bar: '#3478C0', top: '#3478C0', line: '#3478C0', point: '#E89A45', area: 0.08,
      donut: ['#3478C0', '#E89A45', '#18A999', '#8267C7', '#A8B6C4', '#C9D3DD', '#5A6E82'],
      stack: ['#18A999', '#E89A45', '#8267C7', '#3478C0', '#A8B6C4'],
      series: ['#3478C0', '#E89A45', '#18A999', '#8267C7', '#A8B6C4', '#C9562F']
    }
  };
  // FONTS is Inter's @font-face rules, which Cave writes in when it serves this file (lab.ChartScript).
  var FONTS = '';
  // themeOf is the Dashboard's Theme, Corporate Executive when it names none Cave knows.
  function themeOf(key) { return THEMES[key] || THEMES.corporate; }
  // themeCSS is the stylesheet for a Theme: its variables, the shared rules, then its own.
  function themeCSS(t) {
    return FONTS + ':root{' + Object.keys(t.vars).map(function (k) { return '--' + k + ':' + t.vars[k]; }).join(';') + '}\n' + CSS + '\n' + (t.css || '');
  }

  var MONTHS = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec'];

  function esc(s) {
    return String(s == null ? '' : s).replace(/[&<>"']/g, function (c) {
      return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c];
    });
  }
  function fmt(n, money) {
    var s = Math.abs(n) >= 100 || !money ? Math.round(n).toLocaleString('en-MY')
      : n.toLocaleString('en-MY', { minimumFractionDigits: 2, maximumFractionDigits: 2 });
    return money ? (n < 0 ? '-RM' + s.replace('-', '') : 'RM' + s) : s;
  }
  function short(n, money) {
    var a = Math.abs(n), s = a >= 1e6 ? (a / 1e6).toFixed(1) + 'm' : a >= 1e3 ? (a / 1e3).toFixed(a >= 1e4 ? 0 : 1) + 'k' : String(Math.round(a));
    return (n < 0 ? '-' : '') + (money ? 'RM' : '') + s;
  }
  function dmy(iso) { var p = String(iso).split('-'); return +p[2] + ' ' + MONTHS[+p[1] - 1] + ' ' + p[0]; }
  function monthName(ym) { return MONTHS[+ym.slice(5) - 1] + ' ' + ym.slice(0, 4); }
  function nice(v) { if (!v) return 1; var p = Math.pow(10, Math.floor(Math.log10(v))), f = v / p; return (f <= 1 ? 1 : f <= 2 ? 2 : f <= 5 ? 5 : 10) * p; }
  // seriesColour is the colour for the i-th value s from a Theme's list; Masuk and Keluar always show as its positive and negative.
  function seriesColour(t, list, s, i) { return { masuk: t.vars.pos, keluar: t.vars.neg }[String(s).toLowerCase()] || list[i % list.length]; }

  function dayName(iso) { return +iso.slice(8) + ' ' + MONTHS[+iso.slice(5, 7) - 1]; }

  // The computing functions: pure, so charts_test.js checks them in node. A block is in the shape a Work
  // Dashboard stores (value, split, filter, top); a Practice block is read into it first. rows are objects
  // keyed as cols is: by column name in a Work Dashboard, by blank key in a Practice one.

  // fromPractice reads a Template's block, naming blanks, as the same block naming columns would be.
  function fromPractice(b) {
    return { kind: b.kind, label: b.label, wide: b.wide, x: b.x, by: b.kind === 'line' ? 'month' : undefined, split: b.series,
      group: b.group, top: b.kind === 'bar' ? 8 : undefined, other: b.kind === 'donut' ? 5 : undefined, cols: b.cols, sort: b.sort, limit: b.limit, where: b.where, minus: b.minus, low: b.low,
      value: b.kind === 'table' ? undefined : b.agg === 'count' || !b.of ? { agg: 'count' } : b.times ? { agg: 'calc', a: b.of, op: 'x', b: b.times, then: 'sum' } : { agg: 'sum', col: b.of } };
  }
  function num(x) { var n = +x; return x == null || x === '' || isNaN(n) ? null : n; }
  function blank(x) { return x == null || x === '' ? '(blank)' : x; }
  // valueOf is one number from rows: empty cells count as rows and are skipped in sums, averages and distinct counts.
  function valueOf(v, rows) {
    if (!v || v.agg === 'count') return rows.length;
    if (v.agg === 'distinct') { var seen = {}; rows.forEach(function (r) { if (r[v.col] != null && r[v.col] !== '') seen[r[v.col]] = 1; }); return Object.keys(seen).length; }
    var tot = 0, n = 0;
    rows.forEach(function (r) {
      var x;
      if (v.agg === 'calc') {
        var a = num(r[v.a]), b = num(r[v.b]);
        if (a == null || b == null || (v.op === '/' && !b)) return;
        x = v.op === '+' ? a + b : v.op === '-' ? a - b : v.op === '/' ? a / b : a * b;
      } else if ((x = num(r[v.col])) == null) return;
      tot += x; n++;
    });
    return v.agg === 'avg' || (v.agg === 'calc' && v.then === 'avg') ? (n ? tot / n : 0) : tot;
  }
  // where keeps the rows whose column holds one of the values: f is a block's filter, or a Template's [column, value].
  // A filter with its column picked and no value ticked yet keeps every row.
  function where(rows, f, cols) {
    if (f && !f.col) f = { col: f[0], vals: f.slice(1) };
    return f && f.vals.length && cols[f.col] ? rows.filter(function (r) { return f.vals.indexOf(String(blank(r[f.col]))) >= 0; }) : rows;
  }
  // scope is the rows a block draws: those its filter keeps, and with low, those at or below the level.
  function scope(b, rows, cols) {
    rows = where(where(rows, b.filter, cols), b.where, cols);
    var l = b.low;
    return l && cols[l[0]] && cols[l[1]] ? rows.filter(function (r) { return r[l[0]] != null && r[l[1]] != null && +r[l[0]] <= +r[l[1]]; }) : rows;
  }
  // noMatch is whether the block's filter keeps no row of the whole Dataset: it then says "No rows match".
  function noMatch(b, all, cols) { return !!b.filter && !scope(b, all, cols).length; }
  function cardValue(b, rows, cols) {
    return valueOf(b.value, scope(b, rows, cols)) - (b.minus ? valueOf(b.value, where(rows, b.minus, cols)) : 0);
  }
  // groups is [[key, value, rows]] by the column k, largest first; empty cells group as (blank).
  function groups(v, rows, k) {
    var g = {};
    rows.forEach(function (r) { var x = blank(r[k]); (g[x] = g[x] || []).push(r); });
    return Object.keys(g).map(function (x) { return [x, valueOf(v, g[x]), g[x]]; }).sort(function (a, c) { return c[1] - a[1]; });
  }
  // bars is a bar's groups, its top N or all, each with its split as [[value, number]] in name order.
  function bars(b, rows, cols) {
    var g = groups(b.value, scope(b, rows, cols), b.group);
    if (b.top) g = g.slice(0, b.top);
    return g.map(function (x) {
      return [x[0], x[1]].concat(b.split ? [groups(b.value, x[2], b.split).map(function (s) { return [s[0], s[1]]; }).sort(function (a, c) { return a[0] < c[0] ? -1 : 1; })] : []);
    });
  }
  // slices is a donut's groups, past the top 6 folded into one Other (a Practice donut's, past the top 5, as it always was).
  function slices(b, rows, cols) {
    var g = groups(b.value, scope(b, rows, cols), b.group).map(function (x) { return [x[0], x[1]]; }), n = b.other || 6;
    if (g.length > n + 1) { var rest = g.slice(n).reduce(function (a, x) { return a + x[1]; }, 0); g = g.slice(0, n).concat([['Other', rest]]); }
    return g;
  }
  // timeline is a line's values by month or day, one series per split value; empty dates are left off the axis.
  function timeline(b, rows, cols) {
    var keys = [], buckets = {}, series = {};
    scope(b, rows, cols).forEach(function (r) {
      if (!r[b.x]) return;
      var k = b.by === 'day' ? String(r[b.x]) : String(r[b.x]).slice(0, 7), s = b.split && cols[b.split] ? blank(r[b.split]) : b.label;
      if (keys.indexOf(k) < 0) keys.push(k);
      buckets[s] = buckets[s] || {};
      (buckets[s][k] = buckets[s][k] || []).push(r);
    });
    Object.keys(buckets).forEach(function (s) { series[s] = {}; Object.keys(buckets[s]).forEach(function (k) { series[s][k] = valueOf(b.value, buckets[s][k]); }); });
    return { keys: keys.sort(), series: series };
  }
  // tableRows is a table's rows, newest or largest first by its sort column with empty cells last, cut to its limit.
  function tableRows(b, rows, cols) {
    var rs = scope(b, rows, cols).slice(), k = b.sort;
    if (k && cols[k]) rs.sort(function (a, c) { var x = a[k], y = c[k]; return x == null ? (y == null ? 0 : 1) : y == null ? -1 : x < y ? 1 : x > y ? -1 : 0; });
    return b.limit ? rs.slice(0, b.limit) : rs;
  }
  // paused is whether the block names a column the Dataset lacks, or has of another type than the block needs
  // (as lab's WorkBlock.Missing); it is then left off the Dashboard.
  function paused(b, cols) {
    var v = b.value || {}, f = b.filter, need = [
      [v.agg === 'distinct' ? '' : 'number', v.col], ['number', v.a], ['number', v.b], ['date', b.x], ['text', b.split], ['text', b.group],
      ['', b.sort], ['text', f && f.col], ['text', b.where && b.where[0]], ['text', b.minus && b.minus[0]]];
    (b.cols || []).forEach(function (k) { need.push(['', k]); });
    (b.low || []).forEach(function (k) { need.push(['number', k]); });
    return need.some(function (n) { return n[1] && (!cols[n[1]] || (n[0] && cols[n[1]].type !== n[0])); });
  }

  function drawDashboard(dash, data) {
    var style = document.createElement('style');
    var theme = themeOf(dash.theme);
    style.textContent = themeCSS(theme);
    document.head.appendChild(style);

    var cols = dash.columns, blocks = dash.blocks || [], filters = dash.filters || [], state = {};
    // A Work Dashboard's blocks and filter bar name columns, so one whose column the Dataset lacks is left off.
    if (dash.work) {
      // Copied, as the layout below sets wide on them.
      blocks = blocks.filter(function (b) { return !paused(b, cols); }).map(function (b) { return Object.assign({}, b); });
      filters = filters.filter(function (k) { return cols[k]; });
    } else blocks = blocks.map(fromPractice);
    // Each row by blank key: r.amount is the cell in the column chosen for Amount. A Work column is its own key.
    var all = data.map(function (o) { var r = {}; Object.keys(cols).forEach(function (k) { r[k] = o[cols[k].name]; }); return r; });
    var dateKey = filters.filter(function (k) { return cols[k].type === 'date'; })[0];
    var months = [];
    if (dateKey) all.forEach(function (r) { var m = String(r[dateKey] || '').slice(0, 7); if (m && months.indexOf(m) < 0) months.push(m); });
    months.sort();

    document.getElementById('count').textContent = data.length + (data.length === 1 ? ' row' : ' rows');
    var form = document.getElementById('filters');
    form.innerHTML = filters.map(function (k) {
      if (cols[k].type === 'date') {
        var o = months.map(function (m) { return '<option value="' + esc(m) + '">' + esc(monthName(m)) + '</option>'; }).join('');
        return '<label>From <select data-f="from">' + o + '</select></label><label>To <select data-f="to">' + o + '</select></label>';
      }
      var vals = [];
      all.forEach(function (r) { if (r[k] != null && vals.indexOf(r[k]) < 0) vals.push(r[k]); });
      vals.sort();
      return '<label>' + esc(cols[k].name) + ' <select data-f="' + esc(k) + '"><option value="">All</option>' +
        vals.map(function (v) { return '<option value="' + esc(v) + '">' + esc(v) + '</option>'; }).join('') + '</select></label>';
    }).join('');
    if (!filters.length) form.hidden = true;
    var to = form.querySelector('[data-f="to"]');
    if (to) to.value = months[months.length - 1];
    form.addEventListener('change', function () {
      Array.prototype.forEach.call(form.querySelectorAll('select'), function (s) { state[s.getAttribute('data-f')] = s.value; });
      draw();
    });
    var cardsEl = document.getElementById('cards'), chartsEl = document.getElementById('charts');
    // One row of up to four cards, the first twice as wide, so the row ends flush with the charts below.
    // ponytail: a fifth card and on (General, one per Number column) wraps to a second row; cap or regrid if that shows up.
    // A Work Dashboard's five to eight cards sit in two even rows instead.
    var allCards = blocks.filter(function (b) { return b.kind === 'card'; }).length, cardCount = Math.min(4, allCards);
    if (dash.work && allCards > 4) cardsEl.style.setProperty('--cols', 'repeat(' + Math.ceil(allCards / 2) + ',minmax(0,1fr))');
    else if (cardCount > 1) cardsEl.style.setProperty('--cols', 'minmax(0,2fr) repeat(' + (cardCount - 1) + ',minmax(0,1fr))');
    // Panels sit two to a row; a half-width panel with no half-width neighbour spans the row, so no gap is left.
    // A Practice table always spans it; a Work table is as wide as the Learner set it.
    var panels = blocks.filter(function (b) { return b.kind !== 'card'; });
    var full = function (b) { return b.wide || (!dash.work && b.kind === 'table'); };
    for (var i = 0; i < panels.length; i++) {
      if (full(panels[i])) { panels[i].wide = true; continue; }
      if (panels[i + 1] && !full(panels[i + 1])) i++;
      else panels[i].wide = true;
    }

    function rowsNow() {
      return all.filter(function (r) {
        for (var k in state) {
          if (!state[k]) continue;
          var m = String(r[dateKey] || '').slice(0, 7);
          if (k === 'from' ? m < state.from : k === 'to' ? m > state.to : String(r[k]) !== state[k]) return false;
        }
        return true;
      });
    }
    // money is whether a value shows as RM: a sum or average of an RM column, or a calculation on one, bar division.
    function money(v) {
      var m = function (k) { return !!(k && cols[k] && cols[k].money); };
      v = v || {};
      return v.agg === 'sum' || v.agg === 'avg' ? m(v.col) : v.agg === 'calc' && v.op !== '/' ? m(v.a) || m(v.b) : false;
    }
    // finding is the one thing the chart shows, written under its name: its peak, top group or largest share.
    function panel(b, inner, finding) {
      return '<article class="dpanel' + (b.wide ? ' wide' : '') + '"><h2>' + esc(b.label) + '</h2>' +
        (finding ? '<p class="dfind">' + esc(finding) + '</p>' : '') + inner + '</article>';
    }
    function empty(b) { return panel(b, '<p class="dnone">No rows match the filters</p>'); }

    function card(b, rows) {
      if (noMatch(b, all, cols)) return '<div class="dcard"><span>' + esc(b.label) + '</span><strong class="dnone">No rows match</strong></div>';
      var v = cardValue(b, rows, cols);
      var cls = b.minus ? (v < 0 ? ' is-neg' : ' is-pos') : b.low && v > 0 ? ' is-neg' : '';
      return '<div class="dcard' + cls + '"><span>' + esc(b.label) + '</span><strong>' + fmt(v, money(b.value)) + '</strong></div>';
    }

    function line(b, rows) {
      var t = timeline(b, rows, cols), keys = t.keys, series = t.series, by = b.split && cols[b.split], m = money(b.value), day = b.by === 'day';
      var when = function (k) { return day ? dmy(k) : monthName(k); };
      if (!keys.length) return empty(b);
      var names = Object.keys(series).sort(), max = 0;
      var peak = null;
      names.forEach(function (s) { keys.forEach(function (k) { var v = series[s][k] || 0; if (!peak || v > peak[2]) peak = [s, k, v]; }); });
      max = nice(Math.max(0, peak[2]));
      var div = String(max)[0] === '2' ? 4 : 5;
      var W = Math.max(280, chartsEl.clientWidth - 40), H = W < 500 ? 190 : 240, L = 54, B = 26, T = 10, R = 22, w = W - L - R, h = H - B - T;
      var X = function (i) { return L + (keys.length === 1 ? w / 2 : i * w / (keys.length - 1)); }, Y = function (v) { return T + h - v / max * h; };
      var svg = '';
      for (var g = 0; g <= div; g++) {
        var v = max * g / div;
        svg += '<line x1="' + L + '" x2="' + (W - R) + '" y1="' + Y(v) + '" y2="' + Y(v) + '" class="grid"/><text x="' + (L - 8) + '" y="' + (Y(v) + 4) + '" text-anchor="end">' + short(v, m) + '</text>';
      }
      var step = Math.ceil(keys.length / Math.max(1, Math.floor(w / (day ? 52 : 48))));
      keys.forEach(function (k, i) { if (i % step === 0) svg += '<text x="' + X(i) + '" y="' + (H - 6) + '" text-anchor="middle">' + (day ? dayName(k) : MONTHS[+k.slice(5) - 1]) + '</text>'; });
      names.forEach(function (s, si) {
        var c = by ? seriesColour(theme, theme.series, s, si) : theme.line, dot = by ? c : theme.point;
        var pts = keys.map(function (k, i) { return X(i) + ',' + Y(series[s][k] || 0); }).join(' ');
        if (!by && theme.area) svg += '<polygon points="' + L + ',' + Y(0) + ' ' + pts + ' ' + X(keys.length - 1) + ',' + Y(0) + '" fill="' + c + '" opacity="' + theme.area + '"/>';
        svg += '<polyline points="' + pts + '" fill="none" stroke="' + c + '" stroke-width="' + (theme.thin ? 1.75 : 2.5) + '" stroke-linejoin="round"/>';
        keys.forEach(function (k, i) { svg += '<circle cx="' + X(i) + '" cy="' + Y(series[s][k] || 0) + '" r="' + (theme.thin ? 3 : 3.5) + '" fill="' + dot + '"><title>' + esc(s) + ', ' + esc(when(k)) + ': ' + fmt(series[s][k] || 0, m) + '</title></circle>'; });
      });
      var legend = by ? '<p class="dlegend">' + names.map(function (s, si) { return '<span><i style="background:' + seriesColour(theme, theme.series, s, si) + '"></i>' + esc(s) + '</span>'; }).join('') + '</p>' : '';
      var finding = keys.length > 1 && peak[2] > 0 ? 'Peak: ' + (by ? peak[0] + ', ' : '') + when(peak[1]) + ', ' + short(peak[2], m) : '';
      return panel(b, legend + '<svg viewBox="0 0 ' + W + ' ' + H + '" class="dchart" role="img" aria-label="' + esc(b.label) + '">' + svg + '</svg>', finding);
    }

    function bar(b, rows) {
      var g = bars(b, rows, cols), m = money(b.value), split = b.split && cols[b.split], names = [];
      if (!g.length) return empty(b);
      if (split) g.forEach(function (x) { x[2].forEach(function (s) { if (names.indexOf(s[0]) < 0) names.push(s[0]); }); });
      names.sort();
      var max = g[0][1] || 1, colour = function (s) { return seriesColour(theme, theme.stack, s, names.indexOf(s)); };
      // The largest group, first, in the Theme's highlight colour when there is more than one.
      var track = function (x, i) {
        if (!split) return '<span class="dbar-t"><span style="width:' + Math.max(0, x[1] / max * 100).toFixed(1) + '%;background:' + (i === 0 && g.length > 1 ? theme.top : theme.bar) + '"></span></span>';
        return '<span class="dbar-t split">' + x[2].map(function (s) {
          return '<span style="width:' + Math.max(0, s[1] / max * 100).toFixed(1) + '%;background:' + colour(s[0]) + '" title="' + esc(s[0]) + ': ' + esc(fmt(s[1], m)) + '"></span>';
        }).join('') + '</span>';
      };
      var legend = split ? '<p class="dlegend">' + names.map(function (s) { return '<span><i style="background:' + colour(s) + '"></i>' + esc(s) + '</span>'; }).join('') + '</p>' : '';
      return panel(b, legend + '<ul class="dbars">' + g.map(function (x, i) {
        return '<li><span class="dbar-k" title="' + esc(x[0]) + '">' + esc(x[0]) + '</span>' + track(x, i) + '<span class="dbar-v">' + short(x[1], m) + '</span></li>';
      }).join('') + '</ul>', g.length > 1 && g[0][1] > 0 ? 'Top: ' + g[0][0] + ', ' + short(g[0][1], m) : '');
    }

    function donut(b, rows) {
      var g = groups(b.value, scope(b, rows, cols), b.group), m = money(b.value), tot = g.reduce(function (a, x) { return a + x[1]; }, 0);
      if (!tot) return empty(b);
      // Named before the tail folds into Other, which can outweigh the largest group.
      var finding = g.length > 1 ? 'Largest: ' + g[0][0] + ', ' + Math.round(g[0][1] / tot * 100) + '%' : '';
      g = slices(b, rows, cols);
      var acc = 0, r = 60, C = 2 * Math.PI * r, segs = g.map(function (x, i) {
        var len = x[1] / tot * C, s = '<circle r="' + r + '" cx="80" cy="80" fill="none" stroke="' + theme.donut[i] + '" stroke-width="28" stroke-dasharray="' + len + ' ' + (C - len) + '" stroke-dashoffset="' + (-acc) + '" transform="rotate(-90 80 80)"><title>' + esc(x[0]) + ': ' + fmt(x[1], m) + '</title></circle>';
        acc += len; return s;
      }).join('');
      return panel(b, '<div class="ddonut"><svg viewBox="0 0 160 160" role="img" aria-label="' + esc(b.label) + '">' + segs + '<text x="80" y="86" text-anchor="middle" class="dtot">' + short(tot, m) + '</text></svg><ul>' +
        g.map(function (x, i) { return '<li><i style="background:' + theme.donut[i] + '"></i>' + esc(x[0]) + '<span>' + Math.round(x[1] / tot * 100) + '%</span></li>'; }).join('') + '</ul></div>',
        finding);
    }

    function table(b, rows) {
      var keys = (b.cols || []).filter(function (k) { return cols[k]; }), rs = tableRows(b, rows, cols);
      var head = keys.map(function (k) { return '<th' + (cols[k].type === 'number' ? ' class="num"' : '') + '>' + esc(cols[k].name) + '</th>'; }).join('');
      var body = rs.map(function (r) {
        return '<tr>' + keys.map(function (k) {
          var v = r[k], t = cols[k].type;
          if (v == null) return '<td></td>';
          return t === 'number' ? '<td class="num">' + fmt(+v, cols[k].money) + '</td>' : '<td>' + esc(t === 'date' ? dmy(v) : v) + '</td>';
        }).join('') + '</tr>';
      }).join('') || '<tr><td colspan="' + keys.length + '" class="dnone">No rows match the filters</td></tr>';
      return panel(b, '<div class="dtable"><table><thead><tr>' + head + '</tr></thead><tbody>' + body + '</tbody></table></div>');
    }

    var DRAW = { line: line, bar: bar, donut: donut, table: table };
    function draw() {
      var rows = rowsNow();
      cardsEl.innerHTML = blocks.filter(function (b) { return b.kind === 'card'; }).map(function (b) { return card(b, rows); }).join('');
      chartsEl.innerHTML = blocks.filter(function (b) { return DRAW[b.kind]; }).map(function (b) {
        return noMatch(b, all, cols) ? panel(b, '<p class="dnone">No rows match</p>') : DRAW[b.kind](b, rows);
      }).join('');
    }
    draw();
    var width = innerWidth;
    addEventListener('resize', function () { if (innerWidth !== width) { width = innerWidth; draw(); } });
  }

  drawDashboard.compute = { fromPractice: fromPractice, cardValue: cardValue, noMatch: noMatch, timeline: timeline, bars: bars,
    slices: slices, tableRows: tableRows, paused: paused };
  drawDashboard.themes = Object.keys(THEMES);
  window.drawDashboard = drawDashboard;
})();
