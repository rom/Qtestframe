// Applies the persisted colour theme before first paint (external file so the
// strict Content-Security-Policy "script-src 'self'" needs no exceptions).
// qtf.theme holds the light/dark scheme, qtf.decor a decorative theme id;
// both are written by src/stores/ui.ts.
try {
  var root = document.documentElement
  var t = localStorage.getItem('qtf.theme')
  if (t === 'dark' || (!t && window.matchMedia('(prefers-color-scheme: dark)').matches)) {
    root.classList.add('app-dark')
  }
  var d = localStorage.getItem('qtf.decor')
  if (d && /^[a-z_]{1,32}$/.test(d)) {
    root.setAttribute('data-theme', d)
  }
} catch (e) {}
