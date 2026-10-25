// PROTOTYPE (prototype/premium-refresh): cycles ?variant= with the bar's arrows or the arrow keys.
(function () {
  var bar = document.querySelector(".proto-bar"), keys = bar.dataset.variants.split(" ");
  function go(step) {
    var i = (keys.indexOf(bar.dataset.current) + step + keys.length) % keys.length, u = new URL(location.href);
    if (keys[i] === "0") u.searchParams.delete("variant"); else u.searchParams.set("variant", keys[i]);
    location.replace(u);
  }
  bar.addEventListener("click", function (e) { var b = e.target.closest("[data-step]"); if (b) go(+b.dataset.step); });
  document.addEventListener("keydown", function (e) {
    if (e.target.closest("input, textarea, select, [contenteditable]") || e.altKey || e.ctrlKey || e.metaKey) return;
    if (e.key === "ArrowLeft") go(-1); else if (e.key === "ArrowRight") go(1);
  });
})();
