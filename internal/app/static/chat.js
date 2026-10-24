// The live Chat: opens the Course's event stream and puts each new or changed Message in place, sends Messages,
// Reactions, deletes and hides without leaving the page, and fills in a reply from its Reply link. Without JavaScript
// every one of these is a plain link or form and the Chat works by reloading.
(function () {
  var list = document.getElementById("chat"), compose = document.getElementById("compose");
  if (!list || !compose) return;
  var body = compose.querySelector("textarea"), replyTo = compose.querySelector("[name=reply_to]");
  var replying = document.getElementById("replying");

  // put shows a Message's fragment: replacing the Message when it is on the page, else adding it at the end, unless
  // this is an earlier page. The page follows a new Message only when it was already at the bottom.
  function put(html) {
    var t = document.createElement("template");
    t.innerHTML = html.trim();
    var li = t.content.firstElementChild;
    if (!li || !li.id) return;
    var old = document.getElementById(li.id);
    if (old) {
      var typing = old.querySelector(".msg-report[open]"), fresh = li.querySelector(".msg-report");
      if (typing && fresh) { // keep a Report being written
        fresh.open = true;
        fresh.querySelector("textarea").value = typing.querySelector("textarea").value;
      }
      old.replaceWith(li);
      return;
    }
    if (list.hasAttribute("data-paged")) return;
    var atBottom = window.innerHeight + window.scrollY >= document.documentElement.scrollHeight - 80;
    var empty = list.querySelector(".chat-empty");
    if (empty) empty.remove();
    list.appendChild(li);
    if (atBottom) li.scrollIntoView({ block: "nearest" });
  }

  function showError(text) {
    var p = compose.querySelector(".field-err");
    if (!p) {
      p = document.createElement("p");
      p.className = "field-err";
      p.setAttribute("role", "alert");
      compose.prepend(p);
    }
    p.textContent = text;
  }

  function clearError() {
    var p = compose.querySelector(".field-err");
    if (p) p.remove();
  }

  function setReply(id, writer, line) {
    replyTo.value = id;
    replying.querySelector("strong").textContent = writer;
    replying.querySelector("strong + span").textContent = line;
    replying.hidden = !id;
  }

  // The browser reconnects by itself when the stream drops, after a restart or deploy.
  new EventSource(list.dataset.stream).addEventListener("message", function (e) { put(e.data); });

  document.addEventListener("submit", function (e) {
    var form = e.target;
    if (form !== compose && !(list.contains(form) && !form.closest(".msg-report"))) return; // Reports post as they are
    e.preventDefault();
    var data = new URLSearchParams(new FormData(form));
    if (e.submitter && e.submitter.name) data.append(e.submitter.name, e.submitter.value);
    fetch(form.action, { method: "POST", body: data, headers: { "X-Chat": "1" } })
      .then(function (res) {
        return res.text().then(function (text) {
          if (!res.ok) {
            showError(res.headers.get("Content-Type").indexOf("text/plain") === 0 ? text : compose.dataset.failed);
            return;
          }
          clearError();
          put(text);
          if (form === compose) {
            body.value = "";
            setReply("", "", "");
          }
        });
      })
      .catch(function () { showError(compose.dataset.failed); });
  });

  document.addEventListener("click", function (e) {
    var reply = e.target.closest("[data-reply]");
    if (reply) {
      e.preventDefault();
      setReply(reply.dataset.reply, reply.dataset.writer, reply.dataset.line);
      body.focus();
      return;
    }
    var cancel = e.target.closest("#replying a");
    if (cancel) {
      e.preventDefault();
      setReply("", "", "");
      return;
    }
    var quote = e.target.closest("[data-to]"), original = quote && document.getElementById(quote.dataset.to);
    if (original) { // on this page: go to it without reloading
      e.preventDefault();
      original.scrollIntoView({ block: "center" });
      history.replaceState(null, "", "#" + original.id);
    }
  });
})();
