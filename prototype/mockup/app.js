/* PROTOTYPE behaviour for every screen: shared chrome, the demo tour, and scripted moments.
   Demo state lives in sessionStorage so a journey hangs together across pages; closing the
   tab or pressing "Reset demo" starts over. Throwaway: the real thing is Go templates. */
(function () {
  'use strict';
  var D = window.DATA;
  var KEY = 'tcc-demo-v1';
  var NEW_CERT = 'TCC-8KQ2-M4TZ';
  var TODAY = '15 Oct 2026';

  /* ---------- helpers ---------- */
  function $(s, el) { return (el || document).querySelector(s); }
  function $$(s, el) { return Array.prototype.slice.call((el || document).querySelectorAll(s)); }
  function esc(s) { return String(s).replace(/[&<>"]/g, function (c) { return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' }[c]; }); }
  function icon(name, cls) { return '<svg class="i ' + (cls || '') + '" aria-hidden="true"><use href="img/icons.svg#i-' + name + '"/></svg>'; }
  function param(k) { return new URLSearchParams(location.search).get(k); }
  function course(id) { return D.courses.filter(function (c) { return c.id === id; })[0]; }
  function lessonsOf(id) {
    if (D.lessons[id]) return D.lessons[id];
    return (D.outlines[id] || []).map(function (t, i) { return { n: i + 1, title: t, format: i % 3 === 1 ? 'text' : 'video', length: i % 3 === 1 ? '5 min read' : (6 + i) + ' min' }; });
  }
  var FORMAT = { video: 'Video', ebook: 'Ebook', picture: 'Pictures', text: 'Text' };
  var FORMAT_ICON = { video: 'video-camera', ebook: 'file-pdf', picture: 'image', text: 'text-t' };
  function fmtLabel(g) { return g.format === 'picture' ? g.meta : FORMAT[g.format] + ', ' + g.meta; }
  var VIS = { everyone: 'Everyone', learners: 'Logged-in Learners', members: 'Course members only' };

  function toast(msg) {
    var t = document.createElement('div');
    t.className = 'toast';
    t.setAttribute('role', 'status');
    t.innerHTML = icon('check-circle') + '<span>' + esc(msg) + '</span>';
    document.body.appendChild(t);
    requestAnimationFrame(function () { t.classList.add('show'); });
    setTimeout(function () { t.classList.remove('show'); setTimeout(function () { t.remove(); }, 300); }, 2600);
  }

  /* ---------- demo state ---------- */
  function fresh() {
    return { role: 'visitor', joined: D.start.joined.slice(), done: JSON.parse(JSON.stringify(D.start.done)), certs: Object.assign({}, D.start.certs), attempts: {}, revoked: [], uploads: [] };
  }
  var S;
  try { S = JSON.parse(sessionStorage.getItem(KEY)) || fresh(); } catch (e) { S = fresh(); }
  function save() { sessionStorage.setItem(KEY, JSON.stringify(S)); }
  function setRole(r) { S.role = r; save(); }
  function isJoined(id) { return S.joined.indexOf(id) >= 0; }
  function doneOf(id) { return S.done[id] || []; }
  function progress(id) { return { done: doneOf(id).length, total: lessonsOf(id).length }; }
  function allDone(id) { var p = progress(id); return p.total > 0 && p.done >= p.total; }
  function certFor(id) {
    if (S.certs[id]) return S.certs[id];
    var c = course(id);
    if (c && !c.exam && allDone(id)) { S.certs[id] = id === 'website' ? 'TCC-5W3B-N7YA' : NEW_CERT; save(); return S.certs[id]; }
    return null;
  }
  function findCert(id) {
    id = (id || '').trim().toUpperCase();
    var c = D.certificates.filter(function (x) { return x.id === id; })[0];
    if (!c) {
      for (var k in S.certs) if (S.certs[k] === id) c = { id: id, name: D.me.name, course: k, date: k === 'content' ? '19 Sep 2026' : TODAY, status: 'valid' };
    }
    if (c && S.revoked.indexOf(c.id) >= 0) c = Object.assign({}, c, { status: 'revoked' });
    return c || null;
  }

  /* ---------- shared bits of markup ---------- */
  function bar(done, total) {
    var pct = total ? Math.round(done / total * 100) : 0;
    return '<span class="bar" role="img" aria-label="' + done + ' of ' + total + ' lessons done"><span style="width:' + pct + '%"></span></span>';
  }
  function badge(c) { return c.open ? '<span class="badge badge-open">Open</span>' : '<span class="badge badge-closed">Closed</span>'; }
  function examLine(c) { return c.exam ? 'Exam and certificate' : 'Certificate on completion'; }

  function courseCard(c) {
    var joined = S.role === 'learner' && isJoined(c.id);
    var action = joined ? '<a class="btn btn-solid btn-sm" href="course.html?c=' + c.id + '">Continue</a>'
      : c.open ? '<a class="btn btn-solid btn-sm" href="course.html?c=' + c.id + '">View course</a>'
      : '<a class="btn btn-ghost btn-sm" href="course.html?c=' + c.id + '">View course</a>';
    return '<li class="course-card' + (c.open ? '' : ' is-closed') + '">' +
      '<a class="course-card-img" href="course.html?c=' + c.id + '" tabindex="-1" aria-hidden="true"><img src="' + c.img + '" alt="" loading="lazy"></a>' +
      '<div class="course-card-body">' +
        '<div class="course-card-top"><h3><a href="course.html?c=' + c.id + '">' + esc(c.title) + '</a></h3>' + (joined ? '<span class="badge badge-joined">Joined</span>' : badge(c)) + '</div>' +
        '<p class="course-card-desc">' + esc(c.blurb) + '</p>' +
        '<ul class="meta"><li>' + (c.open ? 'Starts ' : 'Started ') + c.start + '</li><li>' + c.lessons + ' lessons</li><li>' + examLine(c) + '</li></ul>' +
        '<div class="course-card-foot">' + action + '</div>' +
      '</div></li>';
  }

  function galleryCard(g) {
    return '<li class="g-card">' +
      '<a class="g-plate" href="#" data-open="' + g.id + '"><img src="' + g.img + '" alt="" loading="lazy">' +
      (g.format === 'video' ? '<span class="g-play">' + icon('play') + '</span>' : '') + '</a>' +
      '<p class="g-cap"><strong>' + esc(g.title) + '</strong><span>' + esc(fmtLabel(g)) + '</span></p></li>';
  }

  /* ---------- public header / footer ---------- */
  function publicHeader() {
    var right;
    if (S.role === 'learner') {
      right = '<a class="hdr-link" href="my-courses.html">My courses</a>' +
        '<a class="avatar" href="my-courses.html" title="' + esc(D.me.name) + '">' + D.me.initials + '</a>';
    } else if (S.role === 'admin') {
      right = '<a class="btn btn-solid btn-sm" href="admin.html">Admin area</a>';
    } else {
      right = '<a class="hdr-link" href="login.html">Log in</a><a class="btn btn-solid btn-sm" href="register.html">Register</a>';
    }
    return '<header class="site-header"><div class="wrap site-header-in">' +
      '<a class="brand" href="index.html"><img src="img/logo.svg" alt="' + esc(D.org.name) + '" class="brand-logo"><span class="brand-tag">' + D.org.portal + '</span></a>' +
      '<nav class="site-nav" aria-label="Main"><a href="courses.html">Courses</a><a href="gallery.html">Gallery</a><a href="verify.html">Check a certificate</a></nav>' +
      '<div class="site-header-right">' + right + '</div></div></header>';
  }

  function publicFooter() {
    var o = D.org;
    return '<footer class="site-footer"><div class="wrap footer-in">' +
      '<div class="footer-brand"><img src="img/logo-light.svg" alt="' + esc(o.name) + '" class="footer-logo">' +
        '<p>Online courses by ' + esc(o.arm) + ', the training arm of ' + esc(o.name) + ', in partnership with ' + esc(o.partner) + '.</p></div>' +
      '<nav class="footer-col" aria-label="Learn"><h3>Learn</h3><a href="courses.html">All courses</a><a href="gallery.html">Gallery</a><a href="verify.html">Check a certificate</a></nav>' +
      '<nav class="footer-col" aria-label="Account"><h3>Account</h3><a href="login.html">Log in</a><a href="register.html">Register</a><a href="login.html">Reset password</a></nav>' +
      '<div class="footer-col"><h3>Contact</h3><a href="tel:+601135667300">' + o.phone + '</a><a href="mailto:' + o.email + '">' + o.email + '</a><address>' + esc(o.address) + '</address></div>' +
      '</div><div class="wrap footer-base">© 2026 ' + esc(o.name) + '. Sample content for design review.</div></footer>';
  }

  /* ---------- admin shell ---------- */
  function adminShell(active) {
    var items = [['admin.html', 'house', 'Dashboard'], ['admin-courses.html', 'book-open', 'Courses'], ['admin-gallery.html', 'images', 'Gallery'], ['admin-learners.html', 'users', 'Learners'], ['admin-certificates.html', 'certificate', 'Certificates']];
    var nav = items.map(function (it) {
      return '<a href="' + it[0] + '"' + (it[2] === active ? ' aria-current="page"' : '') + '>' + icon(it[1]) + '<span>' + it[2] + '</span></a>';
    }).join('');
    return '<aside class="adm-side" id="adm-side"><div class="adm-side-top">' +
        '<a class="adm-brand" href="admin.html"><img src="img/logo-light.svg" alt="' + esc(D.org.name) + '"><span>Learning admin</span></a>' +
        '<button class="adm-close" type="button" aria-label="Close menu" data-menu>' + icon('x') + '</button></div>' +
      '<nav class="adm-nav" aria-label="Admin">' + nav + '</nav>' +
      '<div class="adm-side-foot"><a href="index.html">' + icon('arrow-square-out') + '<span>View learning site</span></a>' +
        '<div class="adm-user"><span class="avatar avatar-gold">TCC</span><span><strong>' + esc(D.org.name) + '</strong><small>Admin</small></span></div></div>' +
      '</aside>' +
      '<div class="adm-topbar"><button class="adm-menu" type="button" aria-label="Open menu" data-menu>' + icon('list') + '</button>' +
        '<a class="adm-brand" href="admin.html"><img src="img/logo-light.svg" alt="' + esc(D.org.name) + '"></a><span class="avatar avatar-gold">TCC</span></div>';
  }

  /* ---------- the demo tour bar ---------- */
  function tourBar(page) {
    var roles = [['visitor', 'Visitor', 'index.html'], ['learner', 'Learner', 'my-courses.html'], ['admin', 'Admin', 'admin.html']];
    var html = '<div class="tour" role="group" aria-label="Demo tour"><span class="tour-tag">Demo</span>' +
      roles.map(function (r) { return '<a href="' + r[2] + '" data-role="' + r[0] + '"' + (S.role === r[0] ? ' aria-current="true"' : '') + '>' + r[1] + '</a>'; }).join('');
    if (page === 'home') html += '<span class="tour-sep"></span><button type="button" data-look="A">Look A</button><button type="button" data-look="B">Look B</button>';
    html += '<span class="tour-sep"></span><button type="button" data-reset title="Start the demo over">Reset</button></div>';
    document.body.insertAdjacentHTML('beforeend', html);
    $$('.tour [data-role]').forEach(function (a) { a.addEventListener('click', function () { setRole(a.dataset.role); }); });
    $('.tour [data-reset]').addEventListener('click', function () { sessionStorage.removeItem(KEY); location.href = 'index.html'; });
  }

  /* ---------- pages ---------- */
  var pages = {};

  pages.home = function () {
    $('#home-courses').innerHTML = D.courses.map(courseCard).join('');
    $('#home-gallery').innerHTML = D.gallery.filter(function (g) { return g.promo; }).map(galleryCard).join('');
    $('#b-rail').innerHTML = D.courses.filter(function (c) { return c.open; }).map(function (c) {
      return '<article class="b-card"><a class="b-cover" href="course.html?c=' + c.id + '"><img class="b-cover-img" src="' + c.img + '" alt="" loading="lazy"></a>' +
        '<p class="b-status b-status-open">Starts ' + c.start + '</p><h3><a href="course.html?c=' + c.id + '">' + esc(c.title) + '</a></h3>' +
        '<p class="b-meta">' + c.lessons + ' lessons, ' + examLine(c).toLowerCase() + '.</p></article>';
    }).join('');
    $('#b-thumbs').innerHTML = D.gallery.filter(function (g) { return g.promo; }).map(function (g, i) {
      return '<a class="b-thumb' + (i === 0 ? ' b-thumb-wide' : '') + '" href="gallery.html"><span class="b-thumb-art"><img src="' + g.img + '" alt="" loading="lazy"></span>' +
        '<span class="b-thumb-cap">' + esc(g.title) + '</span><span class="b-format">' + esc(fmtLabel(g)) + '</span></a>';
    }).join('');
    $('#home-quotes').innerHTML = D.testimonials.map(function (t) {
      return '<figure class="quote"><blockquote>“' + esc(t.quote) + '”</blockquote><figcaption><strong>' + esc(t.name) + '</strong> ' + esc(t.place) + '</figcaption></figure>';
    }).join('');

    var looks = $$('[data-look-page]');
    function show(v) {
      v = (v || 'A').toUpperCase() === 'B' ? 'B' : 'A';
      looks.forEach(function (p) { p.hidden = p.dataset.lookPage !== v; });
      document.body.classList.toggle('look-b', v === 'B');
      $$('.tour [data-look]').forEach(function (b) { b.setAttribute('aria-pressed', String(b.dataset.look === v)); });
      var u = new URL(location.href); u.searchParams.set('variant', v); history.replaceState(null, '', u);
    }
    $$('.tour [data-look]').forEach(function (b) { b.addEventListener('click', function () { show(b.dataset.look); window.scrollTo(0, 0); }); });
    show(param('variant'));
    verifyForms();
    galleryViewer(D.gallery);
    dialogs();
  };

  pages.courses = function () {
    var list = $('#all-courses'), onlyOpen = $('#only-open');
    function draw() { list.innerHTML = D.courses.filter(function (c) { return !onlyOpen.checked || c.open; }).map(courseCard).join(''); }
    onlyOpen.addEventListener('change', draw);
    draw();
  };

  pages.course = function () {
    var c = course(param('c')) || D.courses[0];
    document.title = c.title + ' - ' + D.org.name;
    var ls = lessonsOf(c.id);
    $('#c-img').src = c.img;
    $('#c-title').textContent = c.title;
    $('#c-blurb').textContent = c.blurb;
    $('#c-badge').innerHTML = badge(c);
    $('#c-facts').innerHTML = '<li>' + icon('clock') + (c.open ? 'Starts ' : 'Started ') + c.start + ', self-paced</li>' +
      '<li>' + icon('book-open') + c.lessons + ' lessons: videos, readings and ebooks</li>' +
      '<li>' + icon('exam') + (c.exam ? 'Exam: ' + c.exam.questions + ' questions, pass mark ' + c.exam.pass + '%' : 'No exam') + '</li>' +
      '<li>' + icon('certificate') + 'Certificate anyone can check online</li>';
    if (c.learn) {
      $('#c-learn').innerHTML = '<h2>What you will learn</h2><ul class="ticks">' + c.learn.map(function (l) { return '<li>' + icon('check') + esc(l) + '</li>'; }).join('') + '</ul>';
    } else $('#c-learn').remove();

    function drawLessons() {
      var joined = S.role === 'learner' && isJoined(c.id);
      $('#c-lessons').innerHTML = ls.map(function (l) {
        var done = joined && doneOf(c.id).indexOf(l.n) >= 0;
        var inner = '<span class="ls-n">' + (done ? icon('check') : l.n) + '</span><span class="ls-t">' + esc(l.title) + '</span>' +
          '<span class="ls-f">' + icon(FORMAT_ICON[l.format]) + FORMAT[l.format] + ', ' + esc(l.length) + '</span>' + (joined ? '' : icon('lock-simple', 'ls-lock'));
        return '<li class="' + (done ? 'is-done' : '') + '">' + (joined ? '<a href="lesson.html?c=' + c.id + '&n=' + l.n + '">' + inner + '</a>' : '<div>' + inner + '</div>') + '</li>';
      }).join('');
    }

    function drawPanel() {
      var p = $('#c-panel'), joined = S.role === 'learner' && isJoined(c.id);
      if (joined) {
        var pr = progress(c.id), next = ls.filter(function (l) { return doneOf(c.id).indexOf(l.n) < 0; })[0];
        var cert = certFor(c.id);
        p.innerHTML = '<p class="panel-k">Your progress</p><p class="panel-big">' + pr.done + ' of ' + pr.total + ' lessons</p>' + bar(pr.done, pr.total) +
          (cert ? '<a class="btn btn-solid btn-block" href="certificate.html?id=' + cert + '">' + icon('certificate') + 'View certificate</a>'
            : next ? '<a class="btn btn-solid btn-block" href="lesson.html?c=' + c.id + '&n=' + next.n + '">' + (pr.done ? 'Continue: lesson ' + next.n : 'Start lesson 1') + '</a>'
            : '<a class="btn btn-solid btn-block" href="exam.html?c=' + c.id + '">Sit the exam</a>') +
          '<button class="btn btn-quiet btn-block" type="button" id="leave">Leave course</button>';
        $('#leave').addEventListener('click', function () {
          if (!confirm('Leave this course? Your completed lessons are kept if you join again.')) return;
          S.joined = S.joined.filter(function (x) { return x !== c.id; }); save(); toast('You left ' + c.title); drawPanel(); drawLessons();
        });
      } else if (!c.open) {
        p.innerHTML = '<p class="panel-k">Closed</p><p class="panel-msg">This course is closed to new learners. Learners who already joined keep their access.</p><a class="btn btn-ghost btn-block" href="courses.html">See open courses</a>';
      } else if (S.role === 'learner') {
        p.innerHTML = '<p class="panel-k">Ready when you are</p><p class="panel-msg">Join in one click. No approval needed, and you can leave any time.</p><button class="btn btn-solid btn-block" type="button" id="join">Join course</button>';
        $('#join').addEventListener('click', function () {
          S.joined.push(c.id); save(); toast('You joined ' + c.title); drawPanel(); drawLessons();
        });
      } else {
        p.innerHTML = '<p class="panel-k">Ready when you are</p><p class="panel-msg">Log in or create an account to join. It takes a minute.</p>' +
          '<a class="btn btn-solid btn-block" href="login.html?next=' + encodeURIComponent('course.html?c=' + c.id) + '">Log in to join</a>' +
          '<a class="btn btn-ghost btn-block" href="register.html">Create an account</a>';
      }
    }
    drawLessons(); drawPanel();
    $('#c-quotes').innerHTML = D.testimonials.map(function (t) {
      return '<figure class="quote"><blockquote>“' + esc(t.quote) + '”</blockquote><figcaption><strong>' + esc(t.name) + '</strong> ' + esc(t.place) + '</figcaption></figure>';
    }).join('');
  };

  pages.gallery = function () {
    var vis = S.role === 'learner' ? ['everyone', 'learners', 'members'] : ['everyone'];
    var items = D.gallery.filter(function (g) { return vis.indexOf(g.visibility) >= 0 && (g.visibility !== 'members' || isJoined('genai')); });
    $('#promo').innerHTML = items.filter(function (g) { return g.promo; }).map(galleryCard).join('');
    var fmt = 'all';
    function draw() {
      var cat = $('#cat').value;
      var list = items.filter(function (g) { return (fmt === 'all' || g.format === fmt) && (cat === 'all' || g.category === cat); });
      $('#g-all').innerHTML = list.length ? list.map(galleryCard).join('') : '<li class="empty">Nothing matches these filters yet. Try another format or category.</li>';
    }
    $$('[data-fmt]').forEach(function (b) {
      b.addEventListener('click', function () {
        fmt = b.dataset.fmt;
        $$('[data-fmt]').forEach(function (x) { x.setAttribute('aria-pressed', String(x === b)); });
        draw();
      });
    });
    $('#cat').addEventListener('change', draw);
    if (S.role !== 'learner') $('#g-more').hidden = false;
    draw();
    galleryViewer(items);
    dialogs();
  };

  function galleryViewer(items) {
    var dlg = $('#viewer');
    document.addEventListener('click', function (e) {
      var a = e.target.closest('[data-open]');
      if (!a || !dlg) return;
      e.preventDefault();
      var g = items.filter(function (x) { return x.id === a.dataset.open; })[0] || D.gallery.filter(function (x) { return x.id === a.dataset.open; })[0];
      $('#viewer-body').innerHTML = '<div class="viewer-media"><img src="' + g.img + '" alt="">' + (g.format === 'video' ? '<span class="g-play g-play-lg">' + icon('play') + '</span>' : '') + '</div>' +
        '<div class="viewer-cap"><h2>' + esc(g.title) + '</h2><p>' + esc(fmtLabel(g)) + '</p>' +
        (g.format === 'ebook' ? '<a class="btn btn-solid btn-sm" href="#">' + icon('download-simple') + 'Download PDF</a>' : '') + '</div>';
      dlg.showModal();
    });
  }

  function verifyForms() {
    $$('form[data-verify]').forEach(function (f) {
      f.addEventListener('submit', function (e) {
        e.preventDefault();
        location.href = 'verify.html?id=' + encodeURIComponent($('input', f).value.trim());
      });
    });
  }

  pages.verify = function () {
    verifyForms();
    var id = param('id');
    if (!id) return;
    $('#v-id').value = id;
    var c = findCert(id), out = $('#v-result');
    if (!c) {
      out.innerHTML = '<div class="result result-unknown">' + icon('warning-circle') + '<div><h2>No certificate found</h2><p>We couldn’t find a certificate with the ID <strong>' + esc(id.toUpperCase()) + '</strong>. Check for typing mistakes: IDs look like TCC-7F3K-92QX.</p></div></div>';
    } else if (c.status === 'revoked') {
      out.innerHTML = '<div class="result result-revoked">' + icon('x-circle') + '<div><p class="result-k">Revoked</p><h2>This certificate is no longer valid</h2>' +
        '<dl><dt>Issued to</dt><dd>' + esc(c.name) + '</dd><dt>Course</dt><dd>' + esc(course(c.course).title) + '</dd><dt>Issued</dt><dd>' + c.date + '</dd></dl></div></div>';
    } else {
      out.innerHTML = '<div class="result result-valid">' + icon('seal-check') + '<div><p class="result-k">Valid</p><h2>This certificate is genuine</h2>' +
        '<dl><dt>Issued to</dt><dd>' + esc(c.name) + '</dd><dt>Course</dt><dd>' + esc(course(c.course).title) + '</dd><dt>Issued</dt><dd>' + c.date + '</dd><dt>Issued by</dt><dd>' + esc(D.org.name) + '</dd></dl></div></div>';
    }
  };

  pages.register = function () {
    $('#reg').addEventListener('submit', function (e) {
      e.preventDefault();
      var f = e.target, err = $('#reg-err');
      if (f.password.value.length < 8) { err.hidden = false; f.password.setAttribute('aria-invalid', 'true'); f.password.focus(); return; }
      location.href = 'check-email.html?email=' + encodeURIComponent(f.email.value);
    });
  };

  pages.checkEmail = function () {
    var e = param('email');
    if (e) $('#ce-email').textContent = e;
    $('#resend').addEventListener('click', function () { toast('We sent the link again'); });
  };

  pages.login = function () {
    if (param('verified')) $('#verified').hidden = false;
    var next = param('next') || 'my-courses.html';
    $('#login').addEventListener('submit', function (e) { e.preventDefault(); setRole('learner'); location.href = next; });
    $('#as-learner').addEventListener('click', function () { setRole('learner'); location.href = next; });
    $('#as-admin').addEventListener('click', function () { setRole('admin'); location.href = 'admin.html'; });
  };

  pages.myCourses = function () {
    setRole('learner');
    $('#mc-name').textContent = D.me.first;
    var mine = D.courses.filter(function (c) { return isJoined(c.id); });
    var current = mine.filter(function (c) { return !certFor(c.id) && progress(c.id).done > 0; })[0] || mine.filter(function (c) { return !certFor(c.id); })[0];
    if (current) {
      var pr = progress(current.id), next = lessonsOf(current.id).filter(function (l) { return doneOf(current.id).indexOf(l.n) < 0; })[0];
      $('#mc-continue').innerHTML = '<img src="' + current.img + '" alt="">' +
        '<div class="cont-body"><p class="panel-k">Pick up where you left off</p><h2>' + esc(current.title) + '</h2>' +
        '<p class="cont-next">' + (next ? 'Next: lesson ' + next.n + ', ' + esc(next.title) : 'All lessons done. The exam is open.') + '</p>' +
        '<div class="cont-prog">' + bar(pr.done, pr.total) + '<span>' + pr.done + ' of ' + pr.total + ' lessons</span></div>' +
        '<a class="btn btn-solid" href="' + (next ? 'lesson.html?c=' + current.id + '&n=' + next.n : 'exam.html?c=' + current.id) + '">' + (next ? 'Continue lesson ' + next.n : 'Sit the exam') + '</a></div>';
    } else $('#mc-continue').remove();

    $('#mc-list').innerHTML = mine.length ? mine.map(function (c) {
      var pr = progress(c.id), cert = certFor(c.id);
      return '<li class="mc-row"><img src="' + c.img + '" alt=""><div class="mc-main"><h3><a href="course.html?c=' + c.id + '">' + esc(c.title) + '</a></h3>' +
        '<div class="cont-prog">' + bar(pr.done, pr.total) + '<span>' + (cert ? 'Completed' : pr.done + ' of ' + pr.total + ' lessons') + '</span></div></div>' +
        (cert ? '<a class="btn btn-ghost btn-sm" href="certificate.html?id=' + cert + '">' + icon('certificate') + 'Certificate</a>'
          : '<a class="btn btn-solid btn-sm" href="course.html?c=' + c.id + '">' + (pr.done ? 'Continue' : 'Start') + '</a>') + '</li>';
    }).join('') : '<li class="empty">You haven’t joined a course yet. Pick one below to get started.</li>';

    var certs = Object.keys(S.certs).filter(function (k) { return isJoined(k) || true; });
    $('#mc-certs').innerHTML = certs.length ? certs.map(function (k) {
      var c = findCert(S.certs[k]);
      return '<li><a class="cert-chip" href="certificate.html?id=' + c.id + '">' + icon('certificate') + '<span><strong>' + esc(course(k).title) + '</strong><small>' + c.id + ', issued ' + c.date + '</small></span></a></li>';
    }).join('') : '<li class="empty">Finish a course to earn your first certificate.</li>';

    $('#mc-more').innerHTML = D.courses.filter(function (c) { return c.open && !isJoined(c.id); }).map(courseCard).join('');
  };

  pages.lesson = function () {
    setRole('learner');
    var c = course(param('c')) || D.courses[0], ls = lessonsOf(c.id);
    var n = Math.min(Math.max(parseInt(param('n'), 10) || 1, 1), ls.length), l = ls[n - 1];
    if (!isJoined(c.id)) { S.joined.push(c.id); save(); }
    document.title = l.title + ' - ' + c.title;
    $('#l-course').textContent = c.title;
    $('#l-course').href = 'course.html?c=' + c.id;

    function drawOutline() {
      var pr = progress(c.id);
      $('#l-prog').innerHTML = bar(pr.done, pr.total) + '<span>' + pr.done + ' of ' + pr.total + ' done</span>';
      $('#l-outline').innerHTML = ls.map(function (x) {
        var d = doneOf(c.id).indexOf(x.n) >= 0;
        return '<li class="' + (d ? 'is-done' : '') + (x.n === n ? ' is-current' : '') + '"><a href="lesson.html?c=' + c.id + '&n=' + x.n + '"' + (x.n === n ? ' aria-current="page"' : '') + '>' +
          '<span class="ls-n">' + (d ? icon('check') : x.n) + '</span><span>' + esc(x.title) + '<small>' + FORMAT[x.format] + ', ' + esc(x.length) + '</small></span></a></li>';
      }).join('') +
        '<li class="outline-exam' + (allDone(c.id) ? '' : ' is-locked') + '">' + (c.exam ? '<a href="exam.html?c=' + c.id + '"><span class="ls-n">' + icon(allDone(c.id) ? 'exam' : 'lock-simple') + '</span><span>Exam<small>' + c.exam.questions + ' questions, pass mark ' + c.exam.pass + '%</small></span></a>' : '<div><span class="ls-n">' + icon('certificate') + '</span><span>Certificate<small>Issued when you finish every lesson</small></span></div>') + '</li>';
    }

    $('#l-kicker').textContent = 'Lesson ' + n + ' of ' + ls.length;
    $('#l-title').textContent = l.title;
    var body = $('#l-body');
    if (l.format === 'video') {
      body.innerHTML = '<div class="player"><img src="' + (l.img || c.img) + '" alt=""><button class="player-play" type="button" aria-label="Play video">' + icon('play') + '</button>' +
        '<div class="player-bar"><span class="player-time">0:00 / ' + l.length.replace(' min', ':00') + '</span><span class="player-track"><span></span></span></div></div>' +
        '<p class="l-note">In this video, a trainer from ' + esc(D.org.name) + ' walks through real examples you can copy. Pause at any time and try each step yourself.</p>';
      $('.player-play').addEventListener('click', function () { this.closest('.player').classList.add('is-playing'); });
    } else if (l.format === 'ebook') {
      body.innerHTML = '<div class="ebook"><div class="ebook-cover"><img src="img/logo.svg" alt=""><strong>' + esc(l.title) + '</strong><span>' + esc(c.title) + '</span></div>' +
        '<div class="ebook-info"><h2>Read the ebook</h2><p>' + esc(l.length) + '. Covers privacy, copyright, bias and how to check AI answers before you use them at work.</p>' +
        '<a class="btn btn-solid" href="#">' + icon('book-open') + 'Open ebook</a> <a class="btn btn-ghost" href="#">' + icon('download-simple') + 'Download PDF</a></div></div>';
    } else if (c.id === 'genai' && n === 2) {
      body.innerHTML = '<div class="prose"><p>A prompt is the instruction you give an AI tool. The clearer the instruction, the more useful the answer. Most weak answers come from prompts that leave the AI guessing.</p>' +
        '<h2>The four parts of a good prompt</h2><ol><li><strong>Role.</strong> Tell the AI who it is writing as: <em>You are a customer service lead at a furniture shop.</em></li><li><strong>Task.</strong> Say exactly what you need: <em>Write a reply to this complaint.</em></li><li><strong>Context.</strong> Paste what it needs to know: the complaint, your refund policy, the customer’s order date.</li><li><strong>Format.</strong> Describe the result: <em>Under 120 words, friendly, in Bahasa Melayu.</em></li></ol>' +
        '<div class="callout">' + icon('chalkboard-teacher') + '<p><strong>Try it now.</strong> Open ChatGPT and ask it to write a reply to a late-delivery complaint. Then add one more detail and compare the two answers.</p></div>' +
        '<h2>Ask for changes, not a fresh start</h2><p>If the first answer is close, tell the AI what to change: <em>Make it shorter and remove the apology in the second sentence.</em> Small corrections usually beat starting again.</p></div>';
    } else {
      body.innerHTML = '<div class="prose"><p>This lesson brings together everything so far into a routine you can repeat every week. Write down the three tasks that take most of your time, then decide which one AI can help with first.</p>' +
        '<h2>Your one-page plan</h2><ol><li>Pick one task you repeat every week.</li><li>Write the prompt you will use for it, using the four parts from lesson 2.</li><li>Decide how you will check the result before it goes out.</li><li>Share the plan with one colleague and try it together for two weeks.</li></ol>' +
        '<div class="callout">' + icon('chalkboard-teacher') + '<p><strong>Tip from our trainers.</strong> Start small. One task done well is worth more than ten half-tried tools.</p></div></div>';
    }

    var btn = $('#l-done');
    function drawDone() {
      var d = doneOf(c.id).indexOf(n) >= 0;
      btn.classList.toggle('is-done', d);
      btn.setAttribute('aria-pressed', String(d));
      btn.innerHTML = d ? icon('check-circle') + 'Completed' : icon('check') + 'Mark as complete';
      var next = $('#l-next');
      if (n < ls.length) { next.href = 'lesson.html?c=' + c.id + '&n=' + (n + 1); next.innerHTML = 'Next lesson'; }
      else if (allDone(c.id)) { next.href = c.exam ? 'exam.html?c=' + c.id : 'certificate.html?id=' + certFor(c.id); next.innerHTML = c.exam ? 'Go to the exam' : 'View certificate'; }
      else { next.href = 'course.html?c=' + c.id; next.innerHTML = 'Back to course'; }
      var banner = $('#l-banner');
      banner.hidden = !allDone(c.id);
      if (allDone(c.id)) banner.innerHTML = icon('seal-check') + '<div><strong>All ' + ls.length + ' lessons complete.</strong> ' + (c.exam ? 'You can sit the exam now.' : 'Your certificate is ready.') + '</div>' +
        '<a class="btn btn-gold btn-sm" href="' + (c.exam ? 'exam.html?c=' + c.id : 'certificate.html?id=' + certFor(c.id)) + '">' + (c.exam ? 'Start the exam' : 'View certificate') + '</a>';
      drawOutline();
    }
    btn.addEventListener('click', function () {
      var d = doneOf(c.id), i = d.indexOf(n);
      if (i >= 0) d.splice(i, 1); else d.push(n);
      S.done[c.id] = d; save();
      if (i < 0) toast('Lesson ' + n + ' marked complete');
      drawDone();
    });
    if (n > 1) { $('#l-prev').href = 'lesson.html?c=' + c.id + '&n=' + (n - 1); } else $('#l-prev').hidden = true;
    drawDone();
  };

  pages.exam = function () {
    setRole('learner');
    var c = course(param('c')) || D.courses[0], ex = c.exam;
    var qs = D.exam[c.id] || D.exam.genai;
    var root = $('#exam');
    var used = (S.attempts[c.id] || []).length;
    var left = ex.attempts ? ex.attempts - used : null;
    $('#x-course').textContent = c.title;
    $('#x-course').href = 'course.html?c=' + c.id;

    function intro() {
      if (!allDone(c.id)) {
        var pr = progress(c.id);
        root.innerHTML = '<div class="x-card x-locked">' + icon('lock-simple', 'x-icon') + '<h1>Finish every lesson first</h1><p>The exam opens when all ' + pr.total + ' lessons are complete. You have done ' + pr.done + '.</p>' +
          '<a class="btn btn-solid" href="course.html?c=' + c.id + '">Back to lessons</a></div>';
        return;
      }
      if (certFor(c.id)) {
        root.innerHTML = '<div class="x-card">' + icon('seal-check', 'x-icon x-pass') + '<h1>You passed this exam</h1><p>Your certificate is ready to view and download.</p><a class="btn btn-solid" href="certificate.html?id=' + certFor(c.id) + '">View certificate</a></div>';
        return;
      }
      if (left !== null && left <= 0) {
        root.innerHTML = '<div class="x-card x-locked">' + icon('warning-circle', 'x-icon') + '<h1>No attempts left</h1><p>You have used all ' + ex.attempts + ' attempts. Contact ' + esc(D.org.name) + ' at ' + D.org.email + ' if you need help.</p></div>';
        return;
      }
      root.innerHTML = '<div class="x-card"><p class="panel-k">Final exam</p><h1>' + esc(c.title) + '</h1>' +
        '<ul class="x-facts"><li><strong>' + qs.length + '</strong>questions</li><li><strong>' + ex.pass + '%</strong>to pass</li><li><strong>' + ex.minutes + ' min</strong>time limit</li><li><strong>' + (left === null ? 'Unlimited' : left) + '</strong>' + (left === 1 ? 'attempt left' : 'attempts left') + '</li></ul>' +
        '<p class="x-help">Questions come in a different order each time. The timer starts when you press Start, and your answers are sent automatically when time runs out.</p>' +
        '<button class="btn btn-solid btn-lg" type="button" id="x-start">Start the exam</button></div>';
      $('#x-start').addEventListener('click', run);
    }

    function run() {
      var order = qs.map(function (q, i) { return i; }).sort(function () { return Math.random() - 0.5; });
      var secs = ex.minutes * 60, timer;
      root.innerHTML = '<form id="x-form" class="x-form" novalidate><div class="x-top"><span>' + qs.length + ' questions</span><span class="x-timer" id="x-timer">' + icon('clock') + '<span></span></span></div>' +
        order.map(function (qi, k) {
          var q = qs[qi];
          return '<fieldset class="x-q"><legend><span class="x-qn">Question ' + (k + 1) + '</span>' + esc(q.q) + '</legend>' +
            q.options.map(function (o, oi) { return '<label class="x-opt"><input type="radio" name="q' + qi + '" value="' + oi + '"><span>' + esc(o) + '</span></label>'; }).join('') + '</fieldset>';
        }).join('') +
        '<div class="x-submit"><p id="x-left"></p><button class="btn btn-solid btn-lg" type="submit">Submit answers</button></div></form>';
      var form = $('#x-form');
      function tick() {
        var m = Math.floor(secs / 60), s = secs % 60;
        $('#x-timer span').textContent = m + ':' + (s < 10 ? '0' : '') + s + ' left';
        if (secs-- <= 0) { clearInterval(timer); finish(true); }
      }
      function answered() { return qs.filter(function (q, i) { return form['q' + i] && form['q' + i].value !== ''; }).length; }
      form.addEventListener('change', function () { $('#x-left').textContent = answered() + ' of ' + qs.length + ' answered'; });
      $('#x-left').textContent = '0 of ' + qs.length + ' answered';
      form.addEventListener('submit', function (e) {
        e.preventDefault();
        var a = answered();
        if (a < qs.length && !confirm('You answered ' + a + ' of ' + qs.length + ' questions. Submit anyway?')) return;
        clearInterval(timer); finish(false);
      });
      function finish(timeUp) {
        var wrong = [], k = 0;
        order.forEach(function (qi, pos) { var v = form['q' + qi].value; if (String(qs[qi].correct) !== v) wrong.push({ pos: pos + 1, q: qs[qi].q }); else k++; });
        var pct = Math.round(k / qs.length * 100), pass = pct >= ex.pass;
        (S.attempts[c.id] = S.attempts[c.id] || []).push(pct);
        if (pass) S.certs[c.id] = NEW_CERT;
        save();
        left = ex.attempts ? ex.attempts - S.attempts[c.id].length : null;
        result(pct, k, pass, wrong, timeUp);
      }
      tick(); timer = setInterval(tick, 1000);
      window.scrollTo(0, 0);
    }

    function result(pct, k, pass, wrong, timeUp) {
      root.innerHTML = '<div class="x-card x-result ' + (pass ? 'is-pass' : 'is-fail') + '">' +
        icon(pass ? 'seal-check' : 'x-circle', 'x-icon ' + (pass ? 'x-pass' : 'x-fail')) +
        '<p class="panel-k">' + (timeUp ? 'Time is up. ' : '') + (pass ? 'Passed' : 'Not passed yet') + '</p>' +
        '<p class="x-score">' + pct + '%</p><p>You answered ' + k + ' of ' + qs.length + ' correctly. The pass mark is ' + ex.pass + '%.</p>' +
        (wrong.length ? '<div class="x-wrong"><h2>Questions to review</h2><ul>' + wrong.map(function (w) { return '<li><span>Question ' + w.pos + '</span>' + esc(w.q) + '</li>'; }).join('') + '</ul></div>' : '') +
        (pass ? '<div class="x-cert">' + icon('certificate') + '<div><strong>Your certificate has been issued.</strong><span>ID ' + NEW_CERT + '. Anyone can check it online.</span></div></div>' +
          '<a class="btn btn-gold btn-lg" href="certificate.html?id=' + NEW_CERT + '">View your certificate</a>'
          : (left === null || left > 0 ? '<p class="x-help">' + (left === null ? 'You can try again as many times as you need.' : left + (left === 1 ? ' attempt' : ' attempts') + ' left.') + '</p><button class="btn btn-solid" type="button" id="x-again">Try again</button> <a class="btn btn-ghost" href="course.html?c=' + c.id + '">Review the lessons</a>' : '<p class="x-help">No attempts left.</p>')) +
        '</div>';
      if ($('#x-again')) $('#x-again').addEventListener('click', run);
      if (pass) toast('Certificate issued');
      window.scrollTo(0, 0);
    }
    intro();
  };

  pages.certificate = function () {
    var id = (param('id') || NEW_CERT).toUpperCase(), c = findCert(id);
    if (!c) { $('#cert-wrap').innerHTML = '<p class="empty">No certificate with that ID.</p>'; return; }
    var co = course(c.course);
    $('#ct-name').textContent = c.name;
    $('#ct-course').textContent = co.title;
    if (!co.exam) $('#ct-pre').textContent = 'has completed every lesson of the course';
    $('#ct-date').textContent = c.date;
    $('#ct-id').textContent = c.id;
    $('#ct-url').textContent = D.org.checkBase + c.id;
    $('#ct-sig').textContent = D.org.signatory;
    $('#ct-signame').textContent = D.org.signatory;
    $('#ct-sigtitle').textContent = D.org.signatoryTitle;
    $('#ct-qr').src = 'img/qr-' + (['TCC-8KQ2-M4TZ', 'TCC-4M8P-Q2RW', 'TCC-9D2L-X7NB', 'TCC-3H6T-K8VC'].indexOf(c.id) >= 0 ? c.id : 'TCC-8KQ2-M4TZ') + '.svg';
    if (c.status === 'revoked') $('#cert').classList.add('is-revoked');
    $('#ct-check').href = 'verify.html?id=' + c.id;
    $('#ct-print').addEventListener('click', function () { window.print(); });
    $('#ct-copy').addEventListener('click', function () {
      var u = 'https://' + D.org.checkBase + c.id;
      (navigator.clipboard ? navigator.clipboard.writeText(u) : Promise.reject()).then(function () { toast('Check link copied'); }, function () { toast(u); });
    });
  };

  /* ---------- admin pages ---------- */
  function dialogs() {
    $$('[data-dialog]').forEach(function (b) { b.addEventListener('click', function () { $('#' + b.dataset.dialog).showModal(); }); });
    $$('dialog [data-close]').forEach(function (b) { b.addEventListener('click', function () { b.closest('dialog').close(); }); });
  }

  pages.admin = function () {
    var learners = 248, enr = D.courses.reduce(function (a, c) { return a + c.enrolled; }, 0);
    $('#k-stats').innerHTML = [
      ['users', learners, 'Learners', '+12 this week'],
      ['book-open', enr, 'Enrollments', 'Across ' + D.courses.length + ' courses'],
      ['certificate', 142, 'Certificates issued', '9 this week'],
      ['exam', '78%', 'Exam pass rate', 'Last 30 days']
    ].map(function (s) { return '<li class="stat">' + icon(s[0]) + '<strong>' + s[1] + '</strong><span>' + s[2] + '</span><small>' + s[3] + '</small></li>'; }).join('');
    $('#k-courses').innerHTML = D.courses.map(function (c) {
      var pct = Math.round(c.completed / c.enrolled * 100);
      return '<tr><td><a class="cell-course" href="admin-course.html?c=' + c.id + '"><img src="' + c.img + '" alt="">' + esc(c.title) + '</a></td><td>' + badge(c) + '</td><td class="num">' + c.enrolled + '</td><td class="num">' + c.completed + ' <small>(' + pct + '%)</small></td></tr>';
    }).join('');
    $('#k-activity').innerHTML = D.activity.map(function (a) { return '<li>' + icon(a.icon) + '<p><strong>' + esc(a.who) + '</strong> ' + esc(a.what) + '<small>' + a.when + '</small></p></li>'; }).join('');
  };

  pages.adminCourses = function () {
    $('#ac-rows').innerHTML = D.courses.map(function (c) {
      return '<tr><td><a class="cell-course" href="admin-course.html?c=' + c.id + '"><img src="' + c.img + '" alt=""><span>' + esc(c.title) + '<small>' + (c.open ? 'Starts ' : 'Started ') + c.start + '</small></span></a></td>' +
        '<td><label class="switch"><input type="checkbox" ' + (c.open ? 'checked' : '') + ' data-toggle="' + c.id + '"><span></span><em>' + (c.open ? 'Open' : 'Closed') + '</em></label></td>' +
        '<td class="num">' + c.lessons + '</td><td>' + (c.exam ? c.exam.questions + ' questions' : '<span class="muted">No exam</span>') + '</td><td class="num">' + c.enrolled + '</td>' +
        '<td class="actions"><a class="btn btn-ghost btn-sm" href="admin-course.html?c=' + c.id + '">' + icon('pencil-simple') + 'Edit</a></td></tr>';
    }).join('');
    $$('[data-toggle]').forEach(function (t) {
      t.addEventListener('change', function () {
        var c = course(t.dataset.toggle);
        t.nextElementSibling.nextElementSibling.textContent = t.checked ? 'Open' : 'Closed';
        toast(c.title + (t.checked ? ' is open for joining' : ' is closed to new learners'));
      });
    });
  };

  pages.adminCourse = function () {
    var isNew = param('new'), c = isNew ? null : (course(param('c')) || D.courses[0]);
    dialogs();
    $('#ad-title').textContent = c ? c.title : 'New course';
    $('#ad-crumb').textContent = c ? c.title : 'New course';
    if (c) {
      $('#ad-view').href = 'course.html?c=' + c.id;
      var f = $('#ad-form');
      f.title.value = c.title; f.blurb.value = c.blurb; f.start.value = c.start;
      f.state.value = c.open ? 'open' : 'closed';
      $('#ad-cover').src = c.img;
    } else {
      $('#ad-view').hidden = true;
      $('#ad-cover-wrap').classList.add('is-empty');
      $$('[data-tab]').forEach(function (t) { if (t.dataset.tab !== 'details') t.disabled = true; });
      $('#ad-delete').hidden = true;
    }

    /* tabs */
    function tab(name) {
      $$('[data-tab]').forEach(function (t) { t.setAttribute('aria-selected', String(t.dataset.tab === name)); });
      $$('[data-panel]').forEach(function (p) { p.hidden = p.dataset.panel !== name; });
      history.replaceState(null, '', '#' + name);
    }
    $$('[data-tab]').forEach(function (t) { t.addEventListener('click', function () { tab(t.dataset.tab); }); });
    tab((location.hash || '#details').slice(1));
    window.addEventListener('hashchange', function () { tab((location.hash || '#details').slice(1)); });

    $('#ad-form').addEventListener('submit', function (e) {
      e.preventDefault();
      toast(isNew ? 'Course created. Now add its lessons.' : 'Changes saved');
      if (isNew) setTimeout(function () { location.href = 'admin-course.html?c=genai#lessons'; }, 900);
    });
    $('#ad-cover-input').addEventListener('change', function () {
      var file = this.files[0]; if (!file) return;
      $('#ad-cover').src = URL.createObjectURL(file); $('#ad-cover-wrap').classList.remove('is-empty');
    });
    if (!c) return;

    /* lessons */
    var ls = lessonsOf(c.id).map(function (l) { return Object.assign({}, l); });
    function drawLessons() {
      $('#ad-lessons').innerHTML = ls.map(function (l, i) {
        return '<li class="lb-row"><span class="lb-n">' + (i + 1) + '</span><span class="lb-ic">' + icon(FORMAT_ICON[l.format]) + '</span>' +
          '<span class="lb-t"><strong>' + esc(l.title) + '</strong><small>' + (l.format === 'text' ? 'Short text' : 'Gallery item: ' + FORMAT[l.format] + ', ' + esc(l.length)) + '</small></span>' +
          '<span class="lb-act"><button type="button" class="icon-btn" data-up="' + i + '" aria-label="Move up"' + (i === 0 ? ' disabled' : '') + '>' + icon('arrow-up') + '</button>' +
          '<button type="button" class="icon-btn" data-down="' + i + '" aria-label="Move down"' + (i === ls.length - 1 ? ' disabled' : '') + '>' + icon('arrow-down') + '</button>' +
          '<button type="button" class="icon-btn" aria-label="Edit lesson">' + icon('pencil-simple') + '</button>' +
          '<button type="button" class="icon-btn icon-danger" data-rm="' + i + '" aria-label="Remove lesson">' + icon('trash') + '</button></span></li>';
      }).join('');
      $('#ad-lcount').textContent = ls.length + ' lessons';
    }
    $('#ad-lessons').addEventListener('click', function (e) {
      var b = e.target.closest('button'); if (!b) return;
      var i;
      if (b.dataset.up) { i = +b.dataset.up; ls.splice(i - 1, 0, ls.splice(i, 1)[0]); toast('Lesson moved up'); }
      else if (b.dataset.down) { i = +b.dataset.down; ls.splice(i + 1, 0, ls.splice(i, 1)[0]); toast('Lesson moved down'); }
      else if (b.dataset.rm) { i = +b.dataset.rm; if (!confirm('Remove “' + ls[i].title + '” from this course? The Gallery item itself is kept.')) return; ls.splice(i, 1); toast('Lesson removed'); }
      else return;
      drawLessons();
    });
    drawLessons();

    /* add-lesson dialog */
    var kind = 'gallery', pick = null;
    $('#al-items').innerHTML = D.gallery.map(function (g) {
      return '<label class="pick"><input type="radio" name="pick" value="' + g.id + '"><img src="' + g.img + '" alt=""><span><strong>' + esc(g.title) + '</strong><small>' + esc(fmtLabel(g)) + '</small></span></label>';
    }).join('');
    $$('[data-kind]').forEach(function (b) {
      b.addEventListener('click', function () {
        kind = b.dataset.kind;
        $$('[data-kind]').forEach(function (x) { x.setAttribute('aria-pressed', String(x === b)); });
        $('#al-gallery').hidden = kind !== 'gallery'; $('#al-text').hidden = kind !== 'text';
      });
    });
    $('#al-items').addEventListener('change', function (e) { pick = e.target.value; });
    $('#al-form').addEventListener('submit', function (e) {
      e.preventDefault();
      if (kind === 'gallery') {
        var g = D.gallery.filter(function (x) { return x.id === pick; })[0];
        if (!g) { toast('Choose a Gallery item first'); return; }
        ls.push({ title: g.title, format: g.format, length: g.meta });
      } else {
        var t = $('#al-title').value.trim() || 'Untitled lesson';
        ls.push({ title: t, format: 'text', length: 'short text' });
      }
      $('#add-lesson').close(); drawLessons(); toast('Lesson added at the end');
    });

    /* exam */
    var ex = c.exam;
    if (!ex) {
      $('#ad-exam').innerHTML = '<div class="empty-state">' + icon('exam') + '<h3>This course has no exam</h3><p>Learners get their certificate as soon as they finish every lesson. Add an exam if you want them to prove what they learned first.</p><button class="btn btn-solid" type="button" id="add-exam">' + icon('plus') + 'Add an exam</button></div>';
      $('#add-exam').addEventListener('click', function () { toast('Exam created. Add at least one question to switch it on.'); });
    } else {
      var f2 = $('#ex-settings');
      f2.pass.value = ex.pass; f2.minutes.value = ex.minutes; f2.attempts.value = ex.attempts || '';
      var qsrc = D.exam[c.id] || D.exam.genai;
      if (!D.exam[c.id]) $('#ex-note').hidden = false;
      $('#ex-qcount').textContent = (D.exam[c.id] ? qsrc.length : ex.questions) + ' questions';
      $('#ex-list').innerHTML = qsrc.map(function (q, i) {
        return '<li class="q-card"><div class="q-head"><span class="q-n">Q' + (i + 1) + '</span><p>' + esc(q.q) + '</p>' +
          '<span class="lb-act"><button type="button" class="icon-btn" aria-label="Move up"' + (i === 0 ? ' disabled' : '') + '>' + icon('arrow-up') + '</button><button type="button" class="icon-btn" aria-label="Move down"' + (i === qsrc.length - 1 ? ' disabled' : '') + '>' + icon('arrow-down') + '</button><button type="button" class="icon-btn" aria-label="Edit question">' + icon('pencil-simple') + '</button><button type="button" class="icon-btn icon-danger" aria-label="Remove question">' + icon('trash') + '</button></span></div>' +
          '<ol class="q-opts">' + q.options.map(function (o, oi) { return '<li class="' + (oi === q.correct ? 'is-correct' : '') + '">' + (oi === q.correct ? icon('check-circle') : '<span class="dot"></span>') + esc(o) + '</li>'; }).join('') + '</ol></li>';
      }).join('');
      f2.addEventListener('submit', function (e) { e.preventDefault(); toast('Exam settings saved'); });
      var opts = $('#nq-opts');
      function addOpt(text) {
        if ($$('.nq-opt', opts).length >= 5) { toast('A question can have at most 5 options'); return; }
        var i = $$('.nq-opt', opts).length;
        opts.insertAdjacentHTML('beforeend', '<div class="nq-opt"><input type="radio" name="nq-correct" value="' + i + '" aria-label="Correct answer"><input type="text" placeholder="Option ' + (i + 1) + '" value="' + esc(text || '') + '"></div>');
      }
      addOpt(); addOpt();
      $('#nq-add').addEventListener('click', function () { addOpt(); });
      $('#nq-form').addEventListener('submit', function (e) {
        e.preventDefault();
        var err = $('#nq-err'), filled = $$('.nq-opt input[type=text]', opts).filter(function (x) { return x.value.trim(); }).length;
        var correct = $('input[name=nq-correct]:checked', opts);
        if (!$('#nq-q').value.trim()) { err.textContent = 'Write the question.'; err.hidden = false; return; }
        if (filled < 2) { err.textContent = 'Add at least 2 options.'; err.hidden = false; return; }
        if (!correct) { err.textContent = 'Choose the one correct answer.'; err.hidden = false; return; }
        err.hidden = true; $('#new-q').close(); toast('Question added');
      });
    }

    /* learners */
    var total = lessonsOf(c.id).length;
    var rows = D.learners.slice();
    function drawLearners() {
      $('#ad-learners').innerHTML = rows.map(function (l, i) {
        var p = Math.min(l.progress, total);
        return '<tr><td><strong>' + esc(l.name) + '</strong><small>' + esc(l.email) + '</small></td>' +
          '<td><div class="cont-prog">' + bar(p, total) + '<span>' + p + ' of ' + total + '</span></div></td>' +
          '<td class="num">' + (l.best === null ? '<span class="muted">Not taken</span>' : l.best + '%' + (c.exam && l.best < c.exam.pass ? ' <small class="fail">below pass</small>' : '')) + '</td>' +
          '<td>' + (l.cert ? '<a class="badge badge-open" href="certificate.html?id=' + l.cert + '">Issued</a>' : '<span class="muted">Not yet</span>') + '</td>' +
          '<td class="actions"><button type="button" class="btn btn-quiet btn-sm" data-remove="' + i + '">Remove</button></td></tr>';
      }).join('');
      $('#ad-ecount').textContent = rows.length + ' enrolled';
    }
    $('#ad-learners').addEventListener('click', function (e) {
      var b = e.target.closest('[data-remove]'); if (!b) return;
      var l = rows[+b.dataset.remove];
      if (!confirm('Remove ' + l.name + ' from this course? They lose access to its lessons straight away. Their certificates stay valid.')) return;
      rows.splice(+b.dataset.remove, 1); drawLearners(); toast(l.name + ' was removed from the course');
    });
    drawLearners();

    $('#ad-delete').addEventListener('click', function () { $('#del-course').showModal(); });
    $('#del-msg').textContent = c.title + ' has ' + c.enrolled + ' enrolled learners and ' + c.lessons + ' lessons. Close it instead to stop new joins while current learners finish.';
  };

  pages.adminGallery = function () {
    dialogs();
    var items = D.gallery.concat(S.uploads);
    var fmt = 'all';
    function draw() {
      var list = items.filter(function (g) { return fmt === 'all' || g.format === fmt; });
      $('#ag-grid').innerHTML = list.map(function (g) {
        return '<li class="ag-card"><div class="ag-thumb"><img src="' + g.img + '" alt="">' + (g.promo ? '<span class="ag-promo">' + icon('star') + 'Promotional</span>' : '') + '</div>' +
          '<div class="ag-body"><strong>' + esc(g.title) + '</strong><span class="ag-meta">' + icon(FORMAT_ICON[g.format]) + esc(fmtLabel(g)) + '</span>' +
          '<span class="ag-vis">' + icon(g.visibility === 'everyone' ? 'globe-simple' : 'lock-simple') + VIS[g.visibility] + '</span>' +
          '<span class="ag-cat">' + esc(g.category || 'No category') + '</span></div>' +
          '<div class="ag-act"><button type="button" class="btn btn-ghost btn-sm" data-edit="' + g.id + '">' + icon('pencil-simple') + 'Edit</button><button type="button" class="icon-btn icon-danger" data-del="' + g.id + '" aria-label="Delete">' + icon('trash') + '</button></div></li>';
      }).join('');
      $('#ag-count').textContent = items.length + ' items';
    }
    $$('[data-fmt]').forEach(function (b) {
      b.addEventListener('click', function () { fmt = b.dataset.fmt; $$('[data-fmt]').forEach(function (x) { x.setAttribute('aria-pressed', String(x === b)); }); draw(); });
    });
    draw();

    var dlg = $('#upload'), form = $('#up-form'), file = null, editing = null;
    function resetForm() {
      form.reset(); file = null; editing = null;
      $('#up-preview').innerHTML = ''; $('#up-drop').hidden = false; $('#up-err').hidden = true;
      $('#up-heading').textContent = 'Upload to the Gallery'; $('#up-save').textContent = 'Upload';
    }
    $('#open-upload').addEventListener('click', function () { resetForm(); dlg.showModal(); });
    if (param('upload')) { resetForm(); dlg.showModal(); }
    var input = $('#up-file');
    function take(f) {
      var err = $('#up-err');
      var ok = /^(video\/mp4|application\/pdf|image\/(jpeg|png|webp))$/.test(f.type);
      if (!ok) { err.innerHTML = icon('warning-circle') + '<span><strong>' + esc(f.name) + '</strong> can’t be uploaded. Use an MP4 video, a PDF ebook, or a JPEG, PNG or WebP picture.</span>'; err.hidden = false; return; }
      if (f.size > 2 * 1024 * 1024 * 1024) { err.innerHTML = icon('warning-circle') + '<span>That file is bigger than 2 GB. Shorten or compress it, then try again.</span>'; err.hidden = false; return; }
      err.hidden = true; file = f;
      var kindOf = f.type.indexOf('video') === 0 ? 'video' : f.type === 'application/pdf' ? 'ebook' : 'picture';
      form.format.value = kindOf;
      if (!form.title.value) form.title.value = f.name.replace(/\.[^.]+$/, '').replace(/[-_]+/g, ' ');
      var url = URL.createObjectURL(f);
      $('#up-preview').innerHTML = (kindOf === 'picture' ? '<img src="' + url + '" alt="">' : kindOf === 'video' ? '<video src="' + url + '" muted controls></video>' : '<span class="up-file">' + icon('file-pdf') + '</span>') +
        '<p><strong>' + esc(f.name) + '</strong><small>' + FORMAT[kindOf] + ', ' + (f.size / 1048576).toFixed(1) + ' MB</small></p><button type="button" class="btn btn-quiet btn-sm" id="up-change">Change file</button>';
      $('#up-drop').hidden = true;
      $('#up-change').addEventListener('click', function () { input.click(); });
    }
    input.addEventListener('change', function () { if (this.files[0]) take(this.files[0]); });
    var drop = $('#up-drop');
    ['dragenter', 'dragover'].forEach(function (ev) { drop.addEventListener(ev, function (e) { e.preventDefault(); drop.classList.add('is-over'); }); });
    ['dragleave', 'drop'].forEach(function (ev) { drop.addEventListener(ev, function (e) { e.preventDefault(); drop.classList.remove('is-over'); }); });
    drop.addEventListener('drop', function (e) { if (e.dataTransfer.files[0]) take(e.dataTransfer.files[0]); });
    form.promo.addEventListener('change', function () { if (this.checked) { form.visibility.value = 'everyone'; $('#up-promo-hint').hidden = false; } });

    $('#ag-grid').addEventListener('click', function (e) {
      var ed = e.target.closest('[data-edit]'), del = e.target.closest('[data-del]');
      if (ed) {
        var g = items.filter(function (x) { return x.id === ed.dataset.edit; })[0];
        resetForm(); editing = g;
        $('#up-heading').textContent = 'Edit Gallery item'; $('#up-save').textContent = 'Save changes';
        form.title.value = g.title; form.category.value = g.category || ''; form.visibility.value = g.visibility; form.promo.checked = g.promo; form.format.value = g.format;
        $('#up-preview').innerHTML = '<img src="' + g.img + '" alt=""><p><strong>' + esc(g.title) + '</strong><small>' + esc(fmtLabel(g)) + '</small></p>';
        $('#up-drop').hidden = true;
        dlg.showModal();
      } else if (del) {
        var gi = items.filter(function (x) { return x.id === del.dataset.del; })[0];
        if (!confirm('Delete “' + gi.title + '” and its file? Lessons that use it will show it as missing.')) return;
        items = items.filter(function (x) { return x !== gi; }); draw(); toast('Item deleted');
      }
    });

    form.addEventListener('submit', function (e) {
      e.preventDefault();
      if (editing) {
        editing.title = form.title.value; editing.category = form.category.value; editing.visibility = form.visibility.value; editing.promo = form.promo.checked;
        dlg.close(); draw(); toast('Changes saved'); return;
      }
      if (!file) { var err = $('#up-err'); err.innerHTML = icon('warning-circle') + '<span>Choose a file to upload first.</span>'; err.hidden = false; return; }
      var bar2 = $('#up-progress'); bar2.hidden = false;
      var pct = 0, t = setInterval(function () {
        pct += 9 + Math.random() * 14; bar2.firstElementChild.style.width = Math.min(pct, 100) + '%';
        if (pct >= 100) {
          clearInterval(t);
          var item = { id: 'u' + Date.now(), title: form.title.value || file.name, format: form.format.value, meta: (file.size / 1048576).toFixed(1) + ' MB',
            img: form.format.value === 'picture' ? URL.createObjectURL(file) : 'img/class-group-screen.jpg', category: form.category.value, visibility: form.visibility.value, promo: form.promo.checked };
          items.unshift(item);
          if (form.format.value !== 'picture') { S.uploads.push(item); save(); }
          bar2.hidden = true; bar2.firstElementChild.style.width = '0';
          dlg.close(); draw(); toast('Uploaded to the Gallery');
        }
      }, 140);
    });
  };

  pages.adminLearners = function () {
    var q = $('#al-search');
    function draw() {
      var s = q.value.trim().toLowerCase();
      var list = D.learners.filter(function (l) { return !s || l.name.toLowerCase().indexOf(s) >= 0 || l.email.toLowerCase().indexOf(s) >= 0; });
      $('#al-rows').innerHTML = list.length ? list.map(function (l) {
        return '<tr><td><span class="who"><span class="avatar avatar-sm">' + l.name.split(' ').slice(0, 2).map(function (w) { return w[0]; }).join('') + '</span><span><strong>' + esc(l.name) + '</strong><small>' + esc(l.email) + '</small></span></span></td>' +
          '<td>' + l.joined + '</td><td class="num">' + l.courses + '</td><td class="num">' + (l.cert ? 1 + (l.courses > 2 ? 1 : 0) : 0) + '</td>' +
          '<td><div class="cont-prog">' + bar(l.progress, 6) + '<span>' + l.progress + ' of 6</span></div></td></tr>';
      }).join('') : '<tr><td colspan="5" class="empty">No Learner matches “' + esc(q.value) + '”.</td></tr>';
      $('#al-count').textContent = list.length + ' of 248 Learners';
    }
    q.addEventListener('input', draw);
    draw();
  };

  pages.adminCertificates = function () {
    dialogs();
    $$('[data-ctab]').forEach(function (t) {
      t.addEventListener('click', function () {
        $$('[data-ctab]').forEach(function (x) { x.setAttribute('aria-selected', String(x === t)); });
        $$('[data-cpanel]').forEach(function (p) { p.hidden = p.dataset.cpanel !== t.dataset.ctab; });
      });
    });
    var certs = D.certificates.map(function (c) { return Object.assign({}, c, { status: S.revoked.indexOf(c.id) >= 0 ? 'revoked' : c.status }); });
    var q = $('#cs-search'), target = null;
    function draw() {
      var s = q.value.trim().toLowerCase();
      var list = certs.filter(function (c) { return !s || c.id.toLowerCase().indexOf(s) >= 0 || c.name.toLowerCase().indexOf(s) >= 0; });
      $('#cs-rows').innerHTML = list.length ? list.map(function (c) {
        return '<tr class="' + (c.status === 'revoked' ? 'is-revoked' : '') + '"><td><code>' + c.id + '</code>' + (c.note ? '<small>' + esc(c.note) + '</small>' : '') + '</td><td>' + esc(c.name) + '</td><td>' + esc(course(c.course).title) + '</td><td>' + c.date + '</td>' +
          '<td>' + (c.status === 'revoked' ? '<span class="badge badge-revoked">Revoked</span>' : '<span class="badge badge-open">Valid</span>') + '</td>' +
          '<td class="actions"><a class="btn btn-quiet btn-sm" href="certificate.html?id=' + c.id + '">View</a>' +
          (c.status === 'revoked' ? '' : '<button type="button" class="btn btn-quiet btn-sm btn-danger" data-revoke="' + c.id + '">Revoke</button>') + '</td></tr>';
      }).join('') : '<tr><td colspan="6" class="empty">No certificate matches “' + esc(q.value) + '”.</td></tr>';
    }
    q.addEventListener('input', draw);
    $('#cs-rows').addEventListener('click', function (e) {
      var b = e.target.closest('[data-revoke]'); if (!b) return;
      target = certs.filter(function (c) { return c.id === b.dataset.revoke; })[0];
      $('#rv-what').textContent = target.id + ', issued to ' + target.name + ' for ' + course(target.course).title + '.';
      $('#rv-name').value = target.name;
      $('#revoke').showModal();
    });
    $('#rv-form').addEventListener('submit', function (e) {
      e.preventDefault();
      target.status = 'revoked'; S.revoked.push(target.id); save();
      if ($('#rv-reissue').checked) {
        var nid = 'TCC-' + Math.random().toString(36).slice(2, 6).toUpperCase() + '-' + Math.random().toString(36).slice(2, 6).toUpperCase();
        target.note = 'Replaced by ' + nid;
        certs.unshift({ id: nid, name: $('#rv-name').value, course: target.course, date: TODAY, status: 'valid', note: 'Replaces ' + target.id });
        toast('Revoked and reissued as ' + nid);
      } else toast(target.id + ' revoked');
      $('#revoke').close(); draw();
    });
    draw();

    var sf = $('#cs-settings');
    sf.org.value = D.org.name; sf.signatory.value = D.org.signatory; sf.sigtitle.value = D.org.signatoryTitle;
    function live() {
      $('#pv-org').textContent = sf.org.value; $('#pv-sig').textContent = sf.signatory.value; $('#pv-signame').textContent = sf.signatory.value; $('#pv-sigtitle').textContent = sf.sigtitle.value;
    }
    sf.addEventListener('input', live); live();
    sf.logo.addEventListener('change', function () { if (this.files[0]) $('#pv-logo').src = URL.createObjectURL(this.files[0]); });
    sf.addEventListener('submit', function (e) { e.preventDefault(); toast('Certificate design saved. New certificates use it from now on.'); });
  };

  /* ---------- boot ---------- */
  var body = document.body, page = body.dataset.page, chrome = body.dataset.chrome;
  if (body.dataset.role) setRole(body.dataset.role);
  if (chrome === 'admin') {
    setRole('admin');
    var main = $('main');
    var shell = document.createElement('div');
    shell.className = 'adm';
    shell.innerHTML = adminShell(body.dataset.section);
    main.parentNode.insertBefore(shell, main);
    shell.appendChild(main);
    $$('[data-menu]').forEach(function (b) { b.addEventListener('click', function () { body.classList.toggle('adm-open'); }); });
  } else if (chrome === 'public') {
    body.insertAdjacentHTML('afterbegin', publicHeader());
    body.insertAdjacentHTML('beforeend', publicFooter());
  }
  tourBar(page);
  if (pages[page]) pages[page]();
})();
