/* PROTOTYPE switcher: ?variant=A|B|C, ←/→ keys, floating bar. Throwaway. */
(function () {
  var VARIANTS = [
    { key: 'a', name: 'Catalogue', label: 'A · Catalogue' },
    { key: 'b', name: 'Cinematic', label: 'B · Cinematic' },
    { key: 'c', name: 'Reader', label: 'C · Reader' }
  ];
  var pages = Array.prototype.slice.call(document.querySelectorAll('.page[data-variant]'));
  var nameEl = document.getElementById('proto-name');
  var current = 0;

  function indexFromUrl() {
    var v = (new URLSearchParams(location.search).get('variant') || 'a').toLowerCase();
    for (var i = 0; i < VARIANTS.length; i++) {
      if (VARIANTS[i].key === v) return i;
    }
    return 0;
  }

  function render(next) {
    current = (next + VARIANTS.length) % VARIANTS.length;
    var key = VARIANTS[current].key;
    pages.forEach(function (page) {
      page.hidden = page.dataset.variant.toLowerCase() !== key;
    });
    nameEl.textContent = VARIANTS[current].label;
    document.title = 'Cave — home variants (prototype) — ' + VARIANTS[current].name;
    var url = new URL(location.href);
    url.searchParams.set('variant', key.toUpperCase());
    history.replaceState(null, '', url);
    window.scrollTo(0, 0);
  }

  document.getElementById('proto-prev').addEventListener('click', function () {
    render(current - 1);
  });
  document.getElementById('proto-next').addEventListener('click', function () {
    render(current + 1);
  });
  document.addEventListener('keydown', function (e) {
    var t = e.target;
    if (t && (t.tagName === 'INPUT' || t.tagName === 'TEXTAREA' || t.isContentEditable)) return;
    if (e.key === 'ArrowLeft') render(current - 1);
    else if (e.key === 'ArrowRight') render(current + 1);
  });

  render(indexFromUrl());
})();
