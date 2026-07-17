// Theme handling — defaults to system preference, remembers choice.
(function () {
  const KEY = 'hsl-theme';
  const root = document.documentElement;
  const saved = localStorage.getItem(KEY);
  const prefersDark = window.matchMedia('(prefers-color-scheme: dark)').matches;
  root.setAttribute('data-theme', saved || (prefersDark ? 'dark' : 'light'));

  function updateIcon() {
    const btn = document.getElementById('theme-btn');
    if (btn) btn.textContent = root.getAttribute('data-theme') === 'dark' ? '☀️' : '🌙';
  }

  window.addEventListener('DOMContentLoaded', function () {
    updateIcon();

    const themeBtn = document.getElementById('theme-btn');
    if (themeBtn) {
      themeBtn.addEventListener('click', function () {
        const next = root.getAttribute('data-theme') === 'dark' ? 'light' : 'dark';
        root.setAttribute('data-theme', next);
        localStorage.setItem(KEY, next);
        updateIcon();
      });
    }

    // Mobile nav toggle
    const toggle = document.getElementById('nav-toggle');
    const links = document.getElementById('nav-links');
    if (toggle && links) {
      toggle.addEventListener('click', function () { links.classList.toggle('open'); });
      links.querySelectorAll('a').forEach(function (a) {
        a.addEventListener('click', function () { links.classList.remove('open'); });
      });
    }

    // Highlight active nav link. Compare page basenames without the ".html"
    // extension so it works whether the URL is served as /features, /features.html
    // or /features/ (GitHub Pages accepts all three).
    function pageKey(path) {
      let p = path.split('#')[0].split('?')[0];
      p = p.substring(p.lastIndexOf('/') + 1);
      p = p.replace(/\.html$/, '');
      return p === '' ? 'index' : p;
    }
    const here = pageKey(location.pathname);
    document.querySelectorAll('#nav-links a').forEach(function (a) {
      if (a.classList.contains('lang-link')) return; // language switch keeps its hard-coded state
      const href = a.getAttribute('href');
      if (/^[a-z]+:/i.test(href)) return;            // skip external / mailto links
      if (pageKey(href) === here) a.classList.add('active');
    });

    // Copy buttons on code blocks
    document.querySelectorAll('pre').forEach(function (pre) {
      const wrap = document.createElement('div');
      wrap.className = 'code-wrap';
      pre.parentNode.insertBefore(wrap, pre);
      wrap.appendChild(pre);
      const btn = document.createElement('button');
      btn.className = 'copy-btn';
      btn.type = 'button';
      btn.textContent = 'Copy';
      btn.addEventListener('click', function () {
        navigator.clipboard.writeText(pre.innerText.replace(/\n?Copy$/, '')).then(function () {
          btn.textContent = 'Copied!';
          setTimeout(function () { btn.textContent = 'Copy'; }, 1600);
        });
      });
      wrap.appendChild(btn);
    });
  });
})();
