// Family fit profile builder. The profile lives client-side only: localStorage for
// the active profile, and a base64url payload in the p parameter for share links.
(function () {
  "use strict";

  var STORAGE_KEY = "cinatlas-fit-profile";
  var SERVICES_KEY = "cinatlas-fit-services";

  var membersEl = document.getElementById("members");
  var template = document.getElementById("member-template");
  var builder = document.getElementById("builder");
  var servicesEl = document.getElementById("services");

  function encodeProfile(profile) {
    var json = JSON.stringify(profile);
    return btoa(unescape(encodeURIComponent(json)))
      .replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
  }

  function decodeProfile(param) {
    try {
      var b64 = param.replace(/-/g, "+").replace(/_/g, "/");
      var json = decodeURIComponent(escape(atob(b64)));
      var profile = JSON.parse(json);
      return profile && profile.members && profile.members.length ? profile : null;
    } catch (err) {
      return null;
    }
  }

  function addMemberRow(member) {
    var node = template.content.cloneNode(true);
    var row = node.querySelector(".member");
    if (member) {
      row.querySelector(".member-name").value = member.name || "";
      row.querySelector(".member-ceiling").value = member.ceiling || "";
      (member.hard || []).forEach(function (key) {
        var box = row.querySelector('.hard-vetoes input[value="' + key + '"]');
        if (box) {
          box.checked = true;
          var cat = box.closest(".veto-cat");
          if (cat) cat.open = true;
        }
      });
      (member.soft || []).forEach(function (genre) {
        var box = row.querySelector('.soft-vetoes input[value="' + genre + '"]');
        if (box) box.checked = true;
      });
    }
    row.querySelector(".member-remove").addEventListener("click", function () {
      row.remove();
    });
    membersEl.appendChild(node);
  }

  function checkedValues(row, selector) {
    return Array.prototype.map.call(
      row.querySelectorAll(selector + " input:checked"),
      function (box) { return box.value; }
    );
  }

  function readProfile() {
    var members = Array.prototype.map.call(
      membersEl.querySelectorAll(".member"),
      function (row, i) {
        return {
          name: row.querySelector(".member-name").value.trim() || "Person " + (i + 1),
          ceiling: row.querySelector(".member-ceiling").value || undefined,
          hard: checkedValues(row, ".hard-vetoes"),
          soft: checkedValues(row, ".soft-vetoes")
        };
      }
    );
    return { v: 1, members: members };
  }

  function findFilms() {
    var profile = readProfile();
    if (!profile.members.length) {
      addMemberRow(null);
      return;
    }
    localStorage.setItem(STORAGE_KEY, JSON.stringify(profile));
    localStorage.setItem(SERVICES_KEY, servicesEl.value.trim());
    var params = new URLSearchParams();
    params.set("p", encodeProfile(profile));
    if (servicesEl.value.trim()) params.set("services", servicesEl.value.trim());
    window.location.href = "/fit?" + params.toString();
  }

  function copyShareLink() {
    var profile = readProfile();
    if (!profile.members.length) return;
    var params = new URLSearchParams();
    params.set("p", encodeProfile(profile));
    if (servicesEl.value.trim()) params.set("services", servicesEl.value.trim());
    var link = window.location.origin + "/fit?" + params.toString();
    navigator.clipboard.writeText(link).then(function () {
      var btn = document.getElementById("share");
      var label = btn.textContent;
      btn.textContent = "Copied";
      setTimeout(function () { btn.textContent = label; }, 1500);
    });
  }

  function initialProfile() {
    var fromLink = decodeProfile(builder.dataset.profile || "");
    if (fromLink) return fromLink;
    try {
      var stored = JSON.parse(localStorage.getItem(STORAGE_KEY) || "null");
      if (stored && stored.members && stored.members.length) return stored;
    } catch (err) { /* Fall through to a fresh profile. */ }
    return null;
  }

  var profile = initialProfile();
  if (profile) {
    profile.members.forEach(addMemberRow);
  } else {
    addMemberRow(null);
  }
  if (!servicesEl.value) {
    servicesEl.value = localStorage.getItem(SERVICES_KEY) || "";
  }

  document.getElementById("add-member").addEventListener("click", function () {
    addMemberRow(null);
  });
  document.getElementById("find").addEventListener("click", findFilms);
  document.getElementById("share").addEventListener("click", copyShareLink);
})();
