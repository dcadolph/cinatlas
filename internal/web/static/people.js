// people.js adds department filter pills above a people shelf. It reads the
// distinct known-for departments off the rendered cards and lets the viewer
// narrow the shelf to one, all on this device with nothing sent anywhere. The
// bar stays hidden when a shelf has fewer than two departments to choose from.

(function () {
  "use strict";

  var bars = document.querySelectorAll("[data-dept-filter]");

  bars.forEach(function (bar) {
    var section = bar.closest(".search-section");
    if (!section) {
      return;
    }
    var cards = Array.prototype.slice.call(section.querySelectorAll(".shelf-card[data-dept]"));
    var count = section.querySelector(".count");

    // depts collects the distinct non-empty departments in card order.
    var depts = [];
    cards.forEach(function (card) {
      var d = card.getAttribute("data-dept");
      if (d && depts.indexOf(d) === -1) {
        depts.push(d);
      }
    });
    if (depts.length < 2) {
      return;
    }

    // select shows only cards in dept, or every card when dept is empty, and
    // syncs the count and the active pill.
    function select(dept, pills) {
      var shown = 0;
      cards.forEach(function (card) {
        var match = !dept || card.getAttribute("data-dept") === dept;
        card.classList.toggle("hidden", !match);
        if (match) {
          shown += 1;
        }
      });
      if (count) {
        count.textContent = shown;
      }
      pills.forEach(function (p) {
        p.classList.toggle("active", p.getAttribute("data-dept") === dept);
      });
    }

    var pills = [];
    var labels = [""].concat(depts);
    labels.forEach(function (dept) {
      var pill = document.createElement("button");
      pill.type = "button";
      pill.className = "dept-pill";
      pill.setAttribute("data-dept", dept);
      pill.textContent = dept || "All";
      pill.addEventListener("click", function () {
        select(dept, pills);
      });
      bar.appendChild(pill);
      pills.push(pill);
    });
    select("", pills);
  });
})();
