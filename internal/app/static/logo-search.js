// Keeps the Organisation form's "Search the web for this logo" link on the name as it is typed.
(function () {
  var name = document.getElementById("name"), link = document.querySelector("[data-logo-search]");
  name.addEventListener("input", function () {
    link.href = link.dataset.logoSearch + encodeURIComponent(name.value);
  });
})();
