// Applies the saved theme (Auto / Light / Dark, per browser) before the first paint, so there is no flash.
// A plain file, not inline: the panel's Content-Security-Policy allows scripts from 'self' only.
(function () {
  try {
    var t = localStorage.getItem('dialcatz-den-theme');
    if (t === 'light' || t === 'dark') document.documentElement.setAttribute('data-theme', t);
  } catch (e) {}
})();
