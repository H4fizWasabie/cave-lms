// Asks before sending a form with data-confirm. A form with data-busy is sent once: its buttons are disabled and
// its cave-loader shows, rotating the lines in its data-lines. Without JavaScript both forms send as they are.
(function () {
  document.addEventListener("submit", function (e) {
    var form = e.target;
    if (form.dataset.confirm && !window.confirm(form.dataset.confirm)) {
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
