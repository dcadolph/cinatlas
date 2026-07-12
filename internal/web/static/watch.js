// watch.js tags streaming providers the viewer already subscribes to. The list
// of owned services lives in localStorage on this device only; nothing is sent
// anywhere. A service matches a provider when its text appears in the provider
// name, case-insensitively, so "prime" matches "Amazon Prime Video".

(function () {
  "use strict";

  var STORAGE_KEY = "cinatlas.services";
  var root = document.querySelector("[data-watch]");
  if (!root) {
    return;
  }
  var input = root.querySelector("#my-services");
  var providers = root.querySelectorAll(".provider");

  // parse splits the raw input into lowercased, non-empty service tokens.
  function parse(raw) {
    return (raw || "")
      .split(",")
      .map(function (s) { return s.trim().toLowerCase(); })
      .filter(function (s) { return s.length > 0; });
  }

  // apply marks each provider owned when any token appears in its name.
  function apply(tokens) {
    providers.forEach(function (el) {
      var name = (el.getAttribute("data-provider") || "").toLowerCase();
      var owned = tokens.some(function (t) { return name.indexOf(t) !== -1; });
      el.classList.toggle("owned", owned);
    });
  }

  var saved = "";
  try {
    saved = window.localStorage.getItem(STORAGE_KEY) || "";
  } catch (e) {
    saved = "";
  }
  input.value = saved;
  apply(parse(saved));

  input.addEventListener("input", function () {
    var value = input.value;
    apply(parse(value));
    try {
      window.localStorage.setItem(STORAGE_KEY, value);
    } catch (e) {
      // Storage disabled or full. Tagging still works for this view.
    }
  });
})();
