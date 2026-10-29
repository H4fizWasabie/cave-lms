// Asks before sending a form with data-confirm, or through a button with data-confirm; with data-confirm-if, only
// when the form field it names is not empty. A form with data-busy is sent once: its buttons are disabled and
// its cave-loader shows, rotating the lines in its data-lines. A form with data-upload sends as soon as a file is chosen;
// the file field hides behind its button, or covers the drop area it holds. Without JavaScript all of these send as they are.
(function () {
  document.addEventListener("submit", function (e) {
    var form = e.target, asker = e.submitter && e.submitter.dataset.confirm ? e.submitter : form;
    var field = asker.dataset.confirmIf && form.elements[asker.dataset.confirmIf];
    if (asker.dataset.confirm && (!field || field.value.trim()) && !window.confirm(asker.dataset.confirm)) {
      e.preventDefault();
      return;
    }
    if (!form.hasAttribute("data-busy")) return;
    if (form.dataset.sent) {
      e.preventDefault();
      return;
    }
    form.dataset.sent = "1";
    form.setAttribute("aria-busy", "true");
    form.querySelectorAll("button").forEach(function (b) { b.disabled = true; });
    var loader = form.querySelector(".cave-loader");
    if (!loader) return;
    loader.hidden = false;
    var label = loader.querySelector(".cave-loader-label");
    label.textContent = label.dataset.label; // after showing, so screen readers announce it
    var line = loader.querySelector(".cave-loader-line"), lines = line.dataset.lines.split("|"), i = 0;
    line.textContent = lines[0];
    form.rotating = setInterval(function () {
      line.classList.add("is-out");
      setTimeout(function () {
        i = (i + 1) % lines.length;
        line.textContent = lines[i];
        line.classList.remove("is-out");
      }, 250);
    }, 4000);
  });
  document.querySelectorAll("form[data-upload]").forEach(function (form) {
    var input = form.querySelector("input[type=file]"), button = form.querySelector("button");
    var name = form.querySelector("[data-name]");
    if (name) {
      form.classList.add("is-live");
      button.hidden = true;
      ["dragenter", "dragover"].forEach(function (t) { input.addEventListener(t, function () { form.classList.add("is-over"); }); });
      ["dragleave", "drop"].forEach(function (t) { input.addEventListener(t, function () { form.classList.remove("is-over"); }); });
    } else {
      input.hidden = true;
      button.addEventListener("click", function (e) {
        if (!input.files.length) { e.preventDefault(); input.click(); }
      });
    }
    input.addEventListener("change", function () {
      if (!input.files.length) return;
      if (name) name.textContent = input.files[0].name;
      form.requestSubmit();
    });
  });
  // A button with data-copy copies the text of the field with that id.
  document.addEventListener("click", function (e) {
    var b = e.target.closest("[data-copy]");
    if (b && navigator.clipboard) navigator.clipboard.writeText(document.getElementById(b.dataset.copy).value);
  });
  // Coming back to the page from the history shows the form ready again.
  window.addEventListener("pageshow", function (e) {
    if (!e.persisted) return;
    document.querySelectorAll("form[data-busy]").forEach(function (form) {
      delete form.dataset.sent;
      form.removeAttribute("aria-busy");
      clearInterval(form.rotating);
      form.querySelectorAll("button").forEach(function (b) { b.disabled = false; });
      form.querySelectorAll(".cave-loader").forEach(function (l) { l.hidden = true; });
    });
  });
})();
