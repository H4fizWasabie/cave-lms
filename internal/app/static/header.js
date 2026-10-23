// Escape closes the open header menu, innermost first, and puts focus back on its button.
document.addEventListener("keydown", function (e) {
  if (e.key !== "Escape") return;
  var open = (document.activeElement && document.activeElement.closest(".site-header details[open]")) ||
    document.querySelector(".site-header details[open]");
  if (!open) return;
  open.open = false;
  open.querySelector("summary").focus();
});
