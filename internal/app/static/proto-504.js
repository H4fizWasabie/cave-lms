// PROTOTYPE (#504, throwaway): variant switcher keys and the one-button Upload. Delete with the prototype branch.
(function () {
  var prev = document.querySelector('[data-pf-prev]');
  var next = document.querySelector('[data-pf-next]');
  document.addEventListener('keydown', function (e) {
    if (e.metaKey || e.ctrlKey || e.altKey || e.shiftKey) return;
    if (e.target && e.target.closest && e.target.closest('input, textarea, select, [contenteditable]')) return;
    if (e.key === 'ArrowLeft' && prev) location.href = prev.href;
    else if (e.key === 'ArrowRight' && next) location.href = next.href;
  });

  // With script the file input hides behind the button: click opens the picker, choosing a file submits.
  document.querySelectorAll('form[data-pf-upload]').forEach(function (form) {
    var input = form.querySelector('.pf-file');
    var button = form.querySelector('button');
    input.hidden = true;
    button.addEventListener('click', function (e) {
      if (!input.files.length) { e.preventDefault(); input.click(); }
    });
    input.addEventListener('change', function () { if (input.files.length) form.requestSubmit(); });
  });

  // The drop area shows the chosen file's name.
  document.querySelectorAll('.pf-drop-input').forEach(function (input) {
    var name = input.form.querySelector('[data-pf-name]');
    input.addEventListener('change', function () { name.textContent = input.files.length ? input.files[0].name : ''; });
  });
})();
