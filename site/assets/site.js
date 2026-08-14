/* ============================================================
   DWZ ShortURL 宣传站 - 交互脚本
   零依赖 · 尊重 prefers-reduced-motion
   ============================================================ */
(function () {
  "use strict";

  var prefersReduced = window.matchMedia("(prefers-reduced-motion: reduce)").matches;

  /* ---------- 移动端导航 ---------- */
  var toggle = document.getElementById("navToggle");
  var links = document.getElementById("navLinks");
  if (toggle && links) {
    toggle.addEventListener("click", function () {
      var open = links.classList.toggle("open");
      toggle.setAttribute("aria-expanded", open ? "true" : "false");
    });
    links.addEventListener("click", function (e) {
      if (e.target.tagName === "A") {
        links.classList.remove("open");
        toggle.setAttribute("aria-expanded", "false");
      }
    });
  }

  /* ---------- 滚动揭示 ---------- */
  var reveals = document.querySelectorAll(".reveal");
  if ("IntersectionObserver" in window && !prefersReduced) {
    var io = new IntersectionObserver(
      function (entries) {
        entries.forEach(function (en) {
          if (en.isIntersecting) {
            en.target.classList.add("in");
            io.unobserve(en.target);
          }
        });
      },
      { threshold: 0.12, rootMargin: "0px 0px -40px 0px" }
    );
    reveals.forEach(function (el) { io.observe(el); });
  } else {
    reveals.forEach(function (el) { el.classList.add("in"); });
  }

  /* ---------- 复制按钮 ---------- */
  document.querySelectorAll(".copy-btn").forEach(function (btn) {
    btn.addEventListener("click", function () {
      var target = document.getElementById(btn.getAttribute("data-copy"));
      if (!target) return;
      var text = target.innerText;
      function done() {
        btn.textContent = "已复制 ✓";
        btn.classList.add("ok");
        setTimeout(function () {
          btn.textContent = "复制";
          btn.classList.remove("ok");
        }, 1600);
      }
      if (navigator.clipboard && navigator.clipboard.writeText) {
        navigator.clipboard.writeText(text).then(done, function () { fallbackCopy(text, btn, done); });
      } else {
        fallbackCopy(text, btn, done);
      }
    });
  });

  function fallbackCopy(text, btn, done) {
    var ta = document.createElement("textarea");
    ta.value = text;
    ta.style.position = "fixed";
    ta.style.opacity = "0";
    document.body.appendChild(ta);
    ta.select();
    try { document.execCommand("copy"); done(); } catch (e) { btn.textContent = "复制失败"; }
    document.body.removeChild(ta);
  }

  /* ---------- 滚动高亮导航 ---------- */
  var navA = Array.prototype.slice.call(document.querySelectorAll(".nav-links a[href^='#']"));
  var sections = navA
    .map(function (a) { return document.querySelector(a.getAttribute("href")); })
    .filter(Boolean);
  if ("IntersectionObserver" in window && sections.length) {
    var navIO = new IntersectionObserver(
      function (entries) {
        entries.forEach(function (en) {
          if (en.isIntersecting) {
            navA.forEach(function (a) {
              a.classList.toggle("active", a.getAttribute("href") === "#" + en.target.id);
            });
          }
        });
      },
      { threshold: 0.35 }
    );
    sections.forEach(function (s) { navIO.observe(s); });
  }
})();
