// Applies the persisted colour theme before first paint (external file so the
// strict Content-Security-Policy "script-src 'self'" needs no exceptions).
try {
  var t = localStorage.getItem('qtf.theme')
  if (t === 'dark' || (!t && window.matchMedia('(prefers-color-scheme: dark)').matches)) {
    document.documentElement.classList.add('app-dark')
  }
} catch (e) {}
