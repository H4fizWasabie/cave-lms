/* Cave's chart library. It draws a Dashboard from the column choices and blocks Cave wrote into
   index.html and the rows in data.json: summary cards, a line chart by month, bar and donut charts,
   a table, and a filter bar. It needs no other library and loads nothing from other websites.
   Every column name and cell is escaped before it reaches the page, so a cell shows as text. */
(function () {
  'use strict';

  var CSS = [
    '*,*::before,*::after{box-sizing:border-box}',
    'body{margin:0;background:#f3f4f6;color:#1c2430;font:15px/1.5 ui-sans-serif,system-ui,-apple-system,"Segoe UI",Roboto,Arial,sans-serif}',
    'main{max-width:76rem;margin:0 auto;padding:1.6rem 1.4rem 1rem}',
    'h1,h2{margin:0;line-height:1.2}',
    'select{font:inherit;padding:.35rem .5rem;border:1px solid #cfd4dc;border-radius:6px;background:#fff;color:inherit;max-width:11rem}',
    '.dh{display:flex;flex-wrap:wrap;gap:1rem 2rem;justify-content:space-between;align-items:flex-end;margin-bottom:1.2rem}',
    '.dh h1{font-size:1.6rem;letter-spacing:-.01em}',
    '.dh p{color:#5b6573;font-size:.88rem;margin:.2rem 0 0}',
    '.dh .dq{color:#1c2430;font-size:1rem;font-weight:600}',
    '.df{display:flex;flex-wrap:wrap;gap:.6rem .9rem}',
    '.df label{display:grid;gap:.2rem;font-size:.78rem;font-weight:600;color:#5b6573}',
    '.dcards{display:grid;grid-template-columns:repeat(auto-fit,minmax(10.5rem,1fr));gap:.8rem;margin-bottom:.8rem}',
    '.dcard{background:#fff;border:1px solid #e3e6eb;border-radius:10px;padding:.9rem 1rem;display:grid;gap:.15rem}',
    '.dcard span{font-size:.82rem;color:#5b6573;font-weight:600}',
    '.dcard strong{font-size:1.45rem;font-variant-numeric:tabular-nums;letter-spacing:-.01em}',
    '.dcard:first-child{grid-column:span 2}.dcard:first-child strong{font-size:2.1rem}',
    '.dcard.is-pos strong{color:#2d7a52}.dcard.is-neg strong{color:#b4432a}',
    '.dgrid{display:grid;grid-template-columns:1fr 1fr;gap:.8rem}',
    '.dpanel{background:#fff;border:1px solid #e3e6eb;border-radius:10px;padding:1rem 1.1rem;min-width:0}',
    '.dpanel.wide{grid-column:1/-1}',
    '.dpanel h2{font-size:.98rem;margin-bottom:.7rem}',
    '.dpanel .dfind{margin:-.5rem 0 .7rem;font-size:.84rem;color:#5b6573}',
    '.dchart{width:100%;height:auto}.dchart text{font-size:11px;fill:#6b7482}.dchart .grid{stroke:#eceef2}',
    '.dlegend{display:flex;gap:1rem;font-size:.82rem;color:#5b6573;margin:-.3rem 0 .4rem}',
    '.dlegend span,.ddonut li{display:flex;align-items:center;gap:.4rem}',
    '.dlegend i,.ddonut i{width:.7rem;height:.7rem;border-radius:3px;display:inline-block;flex-shrink:0}',
    '.dbars{list-style:none;margin:0;padding:0;display:grid;gap:.55rem}',
    '.dbars li{display:grid;grid-template-columns:minmax(6rem,9rem) 1fr 4rem;gap:.7rem;align-items:center;font-size:.86rem}',
    '.dbar-k{overflow:hidden;text-overflow:ellipsis;white-space:nowrap}',
    '.dbar-t{height:.8rem;background:#eef0f3;border-radius:99px;overflow:hidden}',
    '.dbar-t span{display:block;height:100%;background:#2f5d8a;border-radius:99px}',
    '.dbar-v{text-align:right;font-variant-numeric:tabular-nums;color:#3c4552}',
    '.ddonut{display:flex;gap:1.2rem;align-items:center}.ddonut svg{width:9.5rem;flex-shrink:0}',
    '.ddonut .dtot{font-size:17px;font-weight:700;fill:#1c2430}',
    '.ddonut ul{list-style:none;margin:0;padding:0;display:grid;gap:.4rem;font-size:.86rem;flex:1}',
    '.ddonut li span{margin-left:auto;color:#5b6573;font-variant-numeric:tabular-nums}',
    '.dtable{overflow-x:auto}.dtable table{width:100%;border-collapse:collapse;font-size:.86rem}',
    '.dtable th{text-align:left;font-size:.76rem;text-transform:uppercase;letter-spacing:.04em;color:#6b7482;padding:.45rem .6rem;border-bottom:1px solid #e3e6eb;white-space:nowrap}',
    '.dtable td{padding:.5rem .6rem;border-bottom:1px solid #f0f1f4;white-space:nowrap}',
    '.dtable .num{text-align:right;font-variant-numeric:tabular-nums}',
    '.dnone{color:#6b7482;font-size:.88rem;padding:1rem 0;text-align:center}',
    '@media (max-width:640px){main{padding:1.1rem .9rem .6rem}.dh h1{font-size:1.3rem}.df{width:100%}.df label{flex:1 1 8rem}' +
      '.df select{max-width:none;width:100%}.dcards{grid-template-columns:1fr 1fr;gap:.6rem}.dcard strong{font-size:1.15rem}.dcard:first-child strong{font-size:1.6rem}' +
      '.dgrid{grid-template-columns:1fr}.dbars li{grid-template-columns:6.5rem 1fr 3.4rem}.ddonut svg{width:7.5rem}}'
  ].join('\n');

  var MONTHS = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec'];
  var PALETTE = ['#2f5d8a', '#d2a649', '#3f8a63', '#b45a3c', '#7a5c9e', '#5e8f9c', '#a3a39a'];
  var IN_OUT = { masuk: PALETTE[2], keluar: PALETTE[3] };

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
  function seriesColour(s, i) { return IN_OUT[String(s).toLowerCase()] || PALETTE[i % PALETTE.length]; }

  function drawDashboard(dash, data) {
    var style = document.createElement('style');
    style.textContent = CSS;
    document.head.appendChild(style);

    var cols = dash.columns, blocks = dash.blocks || [], filters = dash.filters || [], state = {};
    // Each row by blank key: r.amount is the cell in the column chosen for Amount.
    var all = data.map(function (o) { var r = {}; Object.keys(cols).forEach(function (k) { r[k] = o[cols[k].name]; }); return r; });
    var dateKey = filters.filter(function (k) { return cols[k].type === 'date'; })[0];
    var months = [];
    if (dateKey) all.forEach(function (r) { var m = String(r[dateKey] || '').slice(0, 7); if (m && months.indexOf(m) < 0) months.push(m); });
    months.sort();

    document.getElementById('count').textContent = data.length + (data.length === 1 ? ' row' : ' rows');
    var form = document.getElementById('filters');
    form.innerHTML = filters.map(function (k) {
      if (cols[k].type === 'date') {
        var o = months.map(function (m) { return '<option value="' + m + '">' + monthName(m) + '</option>'; }).join('');
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
    function where(rows, w) { return w && cols[w[0]] ? rows.filter(function (r) { return r[w[0]] === w[1]; }) : rows; }
    // The rows a block draws: those matching its where, and with low, those at or below the level.
    function scope(rows, b) {
      rows = where(rows, b.where);
      var l = b.low;
      return l && cols[l[0]] && cols[l[1]] ? rows.filter(function (r) { return r[l[0]] != null && r[l[1]] != null && +r[l[0]] <= +r[l[1]]; }) : rows;
    }
    function sum(rows, k, times) { return rows.reduce(function (a, r) { return a + (+r[k] || 0) * (times ? +r[times] || 0 : 1); }, 0); }
    function money(k) { return !!(k && cols[k] && cols[k].money); }
    function grouped(rows, b) {
      var g = {};
      scope(rows, b).forEach(function (r) { var k = r[b.group] == null ? '(blank)' : r[b.group]; g[k] = (g[k] || 0) + (b.of ? +r[b.of] || 0 : 1); });
      return Object.keys(g).map(function (k) { return [k, g[k]]; }).sort(function (a, c) { return c[1] - a[1]; });
    }
    // finding is the one thing the chart shows, written under its name: its peak, top group or largest share.
    function panel(b, inner, finding) {
      return '<article class="dpanel' + (b.wide || b.kind === 'table' ? ' wide' : '') + '"><h2>' + esc(b.label) + '</h2>' +
        (finding ? '<p class="dfind">' + esc(finding) + '</p>' : '') + inner + '</article>';
    }
    function empty(b) { return panel(b, '<p class="dnone">No rows match the filters</p>'); }

    function card(b, rows) {
      var v = b.agg === 'count' ? scope(rows, b).length : sum(scope(rows, b), b.of, b.times) - (b.minus ? sum(where(rows, b.minus), b.of) : 0);
      var cls = b.minus ? (v < 0 ? ' is-neg' : ' is-pos') : b.low && v > 0 ? ' is-neg' : '';
      return '<div class="dcard' + cls + '"><span>' + esc(b.label) + '</span><strong>' + fmt(v, money(b.of) || money(b.times)) + '</strong></div>';
    }

    function line(b, rows) {
      var keys = [], series = {}, by = b.series && cols[b.series] ? b.series : null, m = money(b.of);
      rows.forEach(function (r) {
        if (!r[b.x]) return;
        var k = String(r[b.x]).slice(0, 7), s = by ? r[by] : b.label;
        if (keys.indexOf(k) < 0) keys.push(k);
        series[s] = series[s] || {};
        series[s][k] = (series[s][k] || 0) + (b.of ? +r[b.of] || 0 : 1);
      });
      keys.sort();
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
      var step = Math.ceil(keys.length / Math.max(1, Math.floor(w / 48)));
      keys.forEach(function (k, i) { if (i % step === 0) svg += '<text x="' + X(i) + '" y="' + (H - 6) + '" text-anchor="middle">' + MONTHS[+k.slice(5) - 1] + '</text>'; });
      names.forEach(function (s, si) {
        var c = by ? seriesColour(s, si) : PALETTE[0];
        var pts = keys.map(function (k, i) { return X(i) + ',' + Y(series[s][k] || 0); }).join(' ');
        if (!by) svg += '<polygon points="' + L + ',' + Y(0) + ' ' + pts + ' ' + X(keys.length - 1) + ',' + Y(0) + '" fill="' + c + '" opacity=".1"/>';
        svg += '<polyline points="' + pts + '" fill="none" stroke="' + c + '" stroke-width="2.5" stroke-linejoin="round"/>';
        keys.forEach(function (k, i) { svg += '<circle cx="' + X(i) + '" cy="' + Y(series[s][k] || 0) + '" r="3.5" fill="' + c + '"><title>' + esc(s) + ', ' + monthName(k) + ': ' + fmt(series[s][k] || 0, m) + '</title></circle>'; });
      });
      var legend = by ? '<p class="dlegend">' + names.map(function (s, si) { return '<span><i style="background:' + seriesColour(s, si) + '"></i>' + esc(s) + '</span>'; }).join('') + '</p>' : '';
      var finding = keys.length > 1 && peak[2] > 0 ? 'Peak: ' + (by ? peak[0] + ', ' : '') + monthName(peak[1]) + ', ' + short(peak[2], m) : '';
      return panel(b, legend + '<svg viewBox="0 0 ' + W + ' ' + H + '" class="dchart" role="img" aria-label="' + esc(b.label) + '">' + svg + '</svg>', finding);
    }

    function bar(b, rows) {
      var g = grouped(rows, b).slice(0, 8), m = money(b.of);
      if (!g.length) return empty(b);
      var max = g[0][1] || 1;
      return panel(b, '<ul class="dbars">' + g.map(function (x) {
        return '<li><span class="dbar-k" title="' + esc(x[0]) + '">' + esc(x[0]) + '</span><span class="dbar-t"><span style="width:' + Math.max(0, x[1] / max * 100).toFixed(1) + '%"></span></span><span class="dbar-v">' + short(x[1], m) + '</span></li>';
      }).join('') + '</ul>', g.length > 1 && g[0][1] > 0 ? 'Top: ' + g[0][0] + ', ' + short(g[0][1], m) : '');
    }

    function donut(b, rows) {
      var g = grouped(rows, b), m = money(b.of), tot = g.reduce(function (a, x) { return a + x[1]; }, 0);
      if (!tot) return empty(b);
      // Named before the tail folds into Other, which can outweigh the largest group.
      var finding = g.length > 1 ? 'Largest: ' + g[0][0] + ', ' + Math.round(g[0][1] / tot * 100) + '%' : '';
      if (g.length > 6) { var rest = g.slice(5).reduce(function (a, x) { return a + x[1]; }, 0); g = g.slice(0, 5).concat([['Other', rest]]); }
      var acc = 0, r = 60, C = 2 * Math.PI * r, segs = g.map(function (x, i) {
        var len = x[1] / tot * C, s = '<circle r="' + r + '" cx="80" cy="80" fill="none" stroke="' + PALETTE[i] + '" stroke-width="28" stroke-dasharray="' + len + ' ' + (C - len) + '" stroke-dashoffset="' + (-acc) + '" transform="rotate(-90 80 80)"><title>' + esc(x[0]) + ': ' + fmt(x[1], m) + '</title></circle>';
        acc += len; return s;
      }).join('');
      return panel(b, '<div class="ddonut"><svg viewBox="0 0 160 160" role="img" aria-label="' + esc(b.label) + '">' + segs + '<text x="80" y="86" text-anchor="middle" class="dtot">' + short(tot, m) + '</text></svg><ul>' +
        g.map(function (x, i) { return '<li><i style="background:' + PALETTE[i] + '"></i>' + esc(x[0]) + '<span>' + Math.round(x[1] / tot * 100) + '%</span></li>'; }).join('') + '</ul></div>',
        finding);
    }

    function table(b, rows) {
      var keys = (b.cols || []).filter(function (k) { return cols[k]; }), rs = scope(rows, b).slice(), sk = b.sort;
      if (sk && cols[sk]) rs.sort(function (a, c) { var x = a[sk], y = c[sk]; return x < y ? 1 : x > y ? -1 : 0; });
      if (b.limit) rs = rs.slice(0, b.limit);
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
      chartsEl.innerHTML = blocks.filter(function (b) { return DRAW[b.kind]; }).map(function (b) { return DRAW[b.kind](b, rows); }).join('');
    }
    draw();
    var width = innerWidth;
    addEventListener('resize', function () { if (innerWidth !== width) { width = innerWidth; draw(); } });
  }

  window.drawDashboard = drawDashboard;
})();
