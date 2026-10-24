// The Blocks tab saves a form with data-autosave as soon as one of its fields changes, so each change is live at
// once, and puts the focus back on that field when the page comes back. Without JavaScript the forms' own Save
// buttons send them.
(function () {
  var key = "blocks-focus";
  document.querySelectorAll("form[data-autosave]").forEach(function (form) {
    form.querySelectorAll("[data-apply]").forEach(function (b) { b.hidden = true; });
    form.addEventListener("change", function (e) {
      sessionStorage.setItem(key, (e.target.id || e.target.name) + "=" + e.target.value);
      form.requestSubmit();
    });
  });
  var was = sessionStorage.getItem(key);
  sessionStorage.removeItem(key);
  if (!was) return;
  var at = was.indexOf("="), name = was.slice(0, at), value = was.slice(at + 1);
  var el = document.getElementById(name) || document.querySelector('[name="' + CSS.escape(name) + '"][value="' + CSS.escape(value) + '"]') ||
    document.querySelector('[name="' + CSS.escape(name) + '"]');
  if (el) el.focus();
})();
