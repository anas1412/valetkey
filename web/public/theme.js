// Follow the system light/dark setting, live. A file (not inline) so the CSP can forbid inline scripts.
(() => {
  const q = matchMedia("(prefers-color-scheme: dark)");
  const apply = () => document.documentElement.classList.toggle("dark", q.matches);
  apply();
  q.addEventListener("change", apply);
})();
