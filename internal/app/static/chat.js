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
        fresh.open = fresh.closest(".msg-menu").open = true;
        fresh.querySelector("textarea").value = typing.querySelector("textarea").value;
      }
      li.classList.toggle("follows", old.classList.contains("follows"));
      old.replaceWith(li);
      return;
    }
    if (list.hasAttribute("data-paged")) return;
    var atBottom = window.innerHeight + window.scrollY >= document.documentElement.scrollHeight - 80;
    var empty = list.querySelector(".chat-empty");
    if (empty) empty.remove();
    if (follows(list.lastElementChild, li)) li.classList.add("follows");
    list.appendChild(li);
    if (atBottom) li.scrollIntoView({ block: "nearest" });
  }

  // follows reports whether Message li, just in, comes soon enough after Message prev by the same writer to show under
  // prev's name, as the page groups them. The writer is told by the name shown, so two Learners of the same name
  // group together here. The page's date headings are not added to, so one that arrives after midnight has none.
  function follows(prev, li) {
    if (!prev || !prev.classList.contains("msg")) return false;
    var name = function (m) { return m.querySelector(".msg-head strong").textContent + m.classList.contains("is-mine"); };
    var at = function (m) { return Date.parse(m.querySelector("time").getAttribute("datetime")); };
    return name(prev) === name(li) && at(li) - at(prev) < Number(list.dataset.followsWithin);
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
    if (form.method === "get") return; // Delete and Hide open their confirmation page
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

  // A Message's open menu or Reaction picker closes on a click elsewhere, on opening another, or on Escape.
  function closeTools(except) {
    list.querySelectorAll(".msg-tools details[open]").forEach(function (d) {
      if (!d.contains(except)) d.open = false;
    });
  }
  list.addEventListener("toggle", function (e) { if (e.target.open) closeTools(e.target); }, true);
  document.addEventListener("keydown", function (e) {
    var open = e.key === "Escape" && document.activeElement.closest(".msg-tool[open]");
    if (open) {
      closeTools(null);
      open.querySelector("summary").focus();
    }
  });
  body.addEventListener("keydown", function (e) { // Ctrl+Enter or Cmd+Enter sends
    if (e.key === "Enter" && (e.ctrlKey || e.metaKey)) {
      e.preventDefault();
      compose.requestSubmit();
    }
  });

  document.addEventListener("click", function (e) {
    if (!e.target.closest(".msg-tools details")) closeTools(null);
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
