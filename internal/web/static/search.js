// Clear button for the search box: wipes the whole query in one click and keeps
// focus in the field. The button hides while the field is empty.
(function () {
  "use strict";
  var input = document.getElementById("search-input");
  var clear = document.getElementById("search-clear");
  if (!input || !clear) {
    return;
  }
  function sync() {
    clear.hidden = input.value.length === 0;
  }
  input.addEventListener("input", sync);
  clear.addEventListener("click", function () {
    input.value = "";
    sync();
    input.focus();
  });
  sync();
})();
