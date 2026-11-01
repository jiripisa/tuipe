(function () {
  "use strict";

  var root = document.documentElement;
  var STORE = "tuipe-style";

  // The style switch: the page takes the colours of the chosen app style.
  function setStyle(id, save) {
    root.setAttribute("data-style", id);
    document.querySelectorAll("[data-set-style]").forEach(function (b) {
      b.setAttribute("aria-pressed", String(b.getAttribute("data-set-style") === id));
    });
    if (save) {
      try { localStorage.setItem(STORE, id); } catch (e) { /* the choice lasts for this page */ }
    }
  }

  document.querySelectorAll("[data-set-style]").forEach(function (b) {
    b.addEventListener("click", function () { setStyle(b.getAttribute("data-set-style"), true); });
  });
  setStyle(root.getAttribute("data-style") || "editor", false);

  // Install tabs.
  var tabs = Array.prototype.slice.call(document.querySelectorAll('[role="tab"]'));

  function selectTab(tab, focus) {
    tabs.forEach(function (t) {
      var on = t === tab;
      t.setAttribute("aria-selected", String(on));
      t.tabIndex = on ? 0 : -1;
      document.getElementById(t.getAttribute("aria-controls")).hidden = !on;
    });
    if (focus) tab.focus();
  }

  tabs.forEach(function (tab, i) {
    tab.addEventListener("click", function () { selectTab(tab, false); });
    tab.addEventListener("keydown", function (e) {
      var step = { ArrowRight: 1, ArrowLeft: -1 }[e.key];
      if (step) {
        e.preventDefault();
        selectTab(tabs[(i + step + tabs.length) % tabs.length], true);
      }
    });
  });
  if (tabs.length) {
    var windows = /Win/.test(navigator.platform || "");
    selectTab(tabs[windows ? 1 : 0], false);
  }

  // Copy buttons take the command from the element next to them, so what is
  // copied is what is shown.
  function fallbackCopy(text) {
    var area = document.createElement("textarea");
    area.value = text;
    area.setAttribute("readonly", "");
    area.style.position = "fixed";
    area.style.opacity = "0";
    document.body.appendChild(area);
    area.select();
    var ok = false;
    try { ok = document.execCommand("copy"); } catch (e) { ok = false; }
    document.body.removeChild(area);
    return ok;
  }

  function copied(button, ok) {
    button.textContent = ok ? "Zkopírováno" : "Vyberte ručně";
    button.classList.toggle("copied", ok);
    setTimeout(function () {
      button.textContent = "Kopírovat";
      button.classList.remove("copied");
    }, 2000);
  }

  document.querySelectorAll("[data-copy]").forEach(function (button) {
    button.addEventListener("click", function () {
      var text = document.getElementById(button.getAttribute("data-copy")).textContent;
      if (navigator.clipboard && navigator.clipboard.writeText) {
        navigator.clipboard.writeText(text).then(
          function () { copied(button, true); },
          function () { copied(button, fallbackCopy(text)); }
        );
      } else {
        copied(button, fallbackCopy(text));
      }
    });
  });

  // The contents of the manual stay open on a wide screen and out of the way
  // on a phone.
  var toc = document.querySelector("details.toc");
  if (toc && window.matchMedia("(max-width: 61.99rem)").matches) toc.open = false;

  // The headline is typed like a round: the ghost runs ahead, the cursor stops
  // on a wrong key, the hint appears, and the mark of the error stays.
  var title = document.querySelector("[data-typed]");
  var hint = document.querySelector("[data-typed-hint]");
  if (!title) return;

  function finish() {
    title.classList.add("typed");
    if (hint) hint.classList.add("shown");
  }

  if (window.matchMedia("(prefers-reduced-motion: reduce)").matches) {
    finish();
    return;
  }

  var cells = [];
  Array.prototype.slice.call(title.childNodes).forEach(function (node) {
    if (node.nodeType !== Node.TEXT_NODE) return;
    var run = document.createDocumentFragment();
    node.textContent.split("").forEach(function (ch) {
      var c = document.createElement("span");
      c.className = "c";
      c.setAttribute("aria-hidden", "true");
      c.textContent = ch;
      run.appendChild(c);
      cells.push(c);
    });
    title.replaceChild(run, node);
  });
  title.classList.add("typed");

  var stop = cells.findIndex(function (c) { return c.textContent === "{"; });
  var pos = 0;
  var ghost = 0;

  function draw() {
    cells.forEach(function (c, i) {
      c.classList.toggle("done", i < pos);
      c.classList.toggle("cursor", i === pos);
      c.classList.toggle("ghost", i === ghost && i !== pos);
    });
  }

  function step() {
    ghost = Math.min(ghost + 1, cells.length);
    if (pos === stop) {
      cells[stop].classList.add("wrong");
      draw();
      if (hint) hint.classList.add("shown");
      stop = -1;
      setTimeout(step, 1250);
      return;
    }
    pos += 1;
    draw();
    if (pos < cells.length) setTimeout(step, 60 + Math.random() * 45);
  }

  draw();
  setTimeout(step, 450);
})();
