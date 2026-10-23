// Counts down the time left and, at zero, submits the answers given so far. The server checks the time too.
(function () {
  var timer = document.querySelector("[data-seconds-left]"), out = timer.querySelector(".x-countdown");
  var form = document.querySelector(".x-form"), end = Date.now() + timer.dataset.secondsLeft * 1000;
  (function tick() {
    var left = Math.max(0, Math.round((end - Date.now()) / 1000));
    out.textContent = Math.floor(left / 60) + ":" + String(left % 60).padStart(2, "0");
    if (left === 0) {
      form.submit();
    } else {
      setTimeout(tick, 1000);
    }
  })();
})();
