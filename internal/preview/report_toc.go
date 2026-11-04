package preview

// Build the outline from rendered H1 headings so report chapters and
// Goldmark's existing (including duplicate-title) anchors stay in sync.
const reportTOCScript = `
(function installReportTOC() {
  var main = document.querySelector("main");
  var headings = Array.prototype.slice.call(main.querySelectorAll("h1"));
  if (!headings.length) return;
  var zh = (document.documentElement.lang || "").toLowerCase().indexOf("zh") === 0;
  var nav = document.createElement("nav");
  nav.className = "report-toc";
  nav.setAttribute("aria-label", zh ? "目录" : "Contents");
  var title = document.createElement("div");
  title.className = "report-toc-title";
  title.textContent = zh ? "目录" : "Contents";
  nav.appendChild(title);
  var list = document.createElement("ol");
  headings.forEach(function (heading, index) {
    if (!heading.id) {
      var id = "codrax-heading-" + index;
      while (document.getElementById(id)) id += "-";
      heading.id = id;
    }
    var item = document.createElement("li");
    var link = document.createElement("a");
    link.href = "#" + encodeURIComponent(heading.id);
    link.textContent = heading.textContent;
    item.appendChild(link);
    list.appendChild(item);
  });
  nav.appendChild(list);
  document.body.insertBefore(nav, main);
  document.body.classList.add("has-report-toc");

  // Audit budgets can hide legacy H3+ headings. Open their region before
  // native fragment navigation, including when opening a bookmarked report.
  function revealTarget() {
    var id;
    try { id = decodeURIComponent(location.hash.slice(1)); } catch (_) { return; }
    var target = document.getElementById(id);
    if (!target || !main.contains(target)) return;
    var section = target.closest("section");
    if (section && target.closest(".trace-audit-overflow")) {
      var toggle = section.querySelector('.trace-audit-toggle[aria-expanded="false"]');
      if (toggle) toggle.click();
    }
    for (var parent = target.parentElement; parent && parent !== main; parent = parent.parentElement) {
      if (parent.tagName === "DETAILS") parent.open = true;
    }
    target.scrollIntoView();
  }
  nav.addEventListener("click", function (event) {
    var link = event.target.closest("a");
    if (!link || event.ctrlKey || event.metaKey || event.shiftKey || event.altKey) return;
    // Run after the browser updates the fragment; also handles a repeated click.
    setTimeout(revealTarget, 0);
  });
  window.addEventListener("hashchange", revealTarget);
  if (location.hash) revealTarget();
})();
`

const reportTOCCSS = `
.report-toc { position: sticky; top: 24px; align-self: start; max-height: calc(100vh - 48px); overflow-y: auto; padding: 16px; margin: 32px 0 32px 20px; border-right: 1px solid var(--line); font-size: 14px; line-height: 1.5; }
.report-toc-title { font-weight: 700; margin-bottom: 12px; }
.report-toc ol { list-style: none; padding: 0; margin: 0; }
.report-toc li { margin: 4px 0; }
.report-toc a { display: block; padding: 4px 6px; color: var(--muted); border-radius: 4px; overflow-wrap: anywhere; }
.report-toc a:hover, .report-toc a:focus-visible { color: var(--link); background: var(--code); }
main :is(h1, h2, h3, h4, h5, h6) { scroll-margin-top: 24px; }
body.has-report-toc { display: grid; grid-template-columns: 260px minmax(0, 980px); justify-content: center; align-items: start; }
@media (max-width: 1000px) {
  body.has-report-toc { display: block; }
  .report-toc { position: static; max-height: 240px; max-width: 940px; margin: 20px auto 0; padding: 12px 20px; border-right: 0; border-bottom: 1px solid var(--line); }
}
@media print {
  body.has-report-toc { display: block; }
  .report-toc { display: none; }
}
`
