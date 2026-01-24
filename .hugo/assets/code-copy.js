(function () {
  document.querySelectorAll(".highlight").forEach(function (highlight) {
    var pre = highlight.querySelector("pre");
    if (!pre) return;

    var button = document.createElement("button");
    button.className = "code-copy-btn";
    button.setAttribute("aria-label", "Copy code");
    button.innerHTML = '<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="9" y="9" width="13" height="13" rx="2"/><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/></svg>';

    button.addEventListener("click", function () {
      var code = pre.querySelector("code");
      navigator.clipboard.writeText(code.textContent).then(function () {
        button.innerHTML = '<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="20 6 9 17 4 12"/></svg>';
        button.classList.add("copied");
        setTimeout(function () {
          button.innerHTML = '<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="9" y="9" width="13" height="13" rx="2"/><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/></svg>';
          button.classList.remove("copied");
        }, 2000);
      });
    });

    highlight.style.position = "relative";
    highlight.appendChild(button);
  });
})();
