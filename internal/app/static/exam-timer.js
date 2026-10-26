// Counts the questions answered and, with a time limit, counts down the time left, warns at 5 and 1 minutes in a polite
// live region and, at zero, submits the answers given so far without asking about blanks. The server checks the time too.
(function () {
  var form = document.querySelector(".x-form"), count = document.querySelector("[data-answered]");
  if (!form) return;
  var questions = form.querySelectorAll(".x-q");
  function answered() {
    var n = 0;
    questions.forEach(function (q) {
      var text = q.querySelector("textarea");
      if (text ? text.value.trim() !== "" : q.querySelector("input:checked")) n++;
    });
    count.textContent = count.dataset.answered.replace("{n}", n);
  }
  answered();
  form.addEventListener("input", answered);
  form.addEventListener("change", answered);

  var timer = document.querySelector("[data-seconds-left]");
  if (!timer) return;
  var out = timer.querySelector(".x-countdown"), warn = document.querySelector(".x-warn");
  var end = Date.now() + timer.dataset.secondsLeft * 1000, long = timer.dataset.minutes > 5;
  var warn5 = warn.getAttribute("data-warn-5"), warn1 = warn.getAttribute("data-warn-1");
  (function tick() {
    var left = Math.max(0, Math.round((end - Date.now()) / 1000));
    out.textContent = Math.floor(left / 60) + ":" + String(left % 60).padStart(2, "0");
    var msg = left <= 60 ? warn1 : left <= 300 && long ? warn5 : "";
    if (warn.textContent !== msg) warn.textContent = msg;
    if (left === 0) {
      var ok = document.createElement("input");
      ok.type = "hidden";
      ok.name = "confirm";
      ok.value = "yes";
      form.appendChild(ok);
      form.submit();
    } else {
      setTimeout(tick, 1000);
    }
  })();
})();
