// hints.js rotates a search box placeholder through a house list of favorite
// actors so an empty box always suggests someone worth looking up. Focusing
// the box restores its own descriptive placeholder and stops the rotation so
// it never fights the person typing.

(function () {
  "use strict";

  var NAMES = [
    "Steve Carell",
    "Bryan Cranston",
    "Leonardo DiCaprio",
    "Christian Bale",
    "Natalie Portman",
    "Margot Robbie",
    "Cillian Murphy",
    "Emma Stone"
  ];

  var inputs = document.querySelectorAll("[data-hints]");
  Array.prototype.forEach.call(inputs, function (input) {
    var base = input.getAttribute("placeholder") || "";
    var i = 0;
    var timer = null;

    // tick advances the placeholder to the next favorite name.
    function tick() {
      input.setAttribute("placeholder", "Try “" + NAMES[i % NAMES.length] + "”");
      i++;
    }

    // start rotates from the current name and keeps cycling.
    function start() {
      stop();
      tick();
      timer = window.setInterval(tick, 2600);
    }

    // stop halts rotation and restores the box's own placeholder.
    function stop() {
      if (timer !== null) {
        window.clearInterval(timer);
        timer = null;
      }
      if (base) {
        input.setAttribute("placeholder", base);
      }
    }

    input.addEventListener("focus", stop);
    input.addEventListener("blur", function () {
      if (!input.value) {
        start();
      }
    });

    if (document.activeElement !== input) {
      start();
    }
  });
})();
