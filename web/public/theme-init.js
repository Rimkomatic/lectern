// Runs before the page paints so there is no flash of the wrong theme.
// It has to be a separate file because the server's Content-Security-Policy
// does not allow inline scripts. Keep the storage key in step with src/lib/theme.svelte.ts.
(function () {
  var pref = null;
  try {
    pref = localStorage.getItem('lectern-theme');
  } catch (e) {
    // storage blocked: behave like "system"
  }
  var dark =
    pref === 'dark' ||
    (pref !== 'light' && !!window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)').matches);
  document.documentElement.setAttribute('data-theme', dark ? 'dark' : 'light');
  var meta = document.querySelector('meta[name="theme-color"]');
  if (meta) meta.setAttribute('content', dark ? '#0d0f12' : '#f6f4ef');
})();
