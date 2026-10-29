// Escape closes the open header menu, innermost first, and puts focus back on its button.
document.addEventListener("keydown", function (e) {
  if (e.key !== "Escape") return;
  var open = (document.activeElement && document.activeElement.closest(".site-header details[open]")) ||
    document.querySelector(".site-header details[open]");
  if (!open) return;
  open.open = false;
  open.querySelector("summary").focus();
});

// Choosing a link in a header menu closes the menus, since a link to a section of the same page (/#about) leaves them open.
document.addEventListener("click", function (e) {
  if (!e.target.closest(".site-header details[open] a")) return;
  document.querySelectorAll(".site-header details[open]").forEach(function (d) { d.open = false; });
});
