// globe.js drives the cinatlas globe in three modes. Movie mode reads its pins
// from an inline JSON block. Person mode fetches every filming pin across a
// person's scoped filmography from /globe/pins and supports live filtering and
// a focused/all scope toggle. Landing mode shows a slowly spinning empty globe
// behind the "whose atlas?" prompt.

(function () {
  "use strict";

  var mount = document.querySelector("[data-globe]");
  if (!mount || typeof maplibregl === "undefined") {
    return;
  }
  var mode = mount.getAttribute("data-mode");

  var map = new maplibregl.Map({
    container: "map",
    style: {
      version: 8,
      sources: {
        carto: {
          type: "raster",
          tiles: [
            "https://a.basemaps.cartocdn.com/dark_nolabels/{z}/{x}/{y}@2x.png",
            "https://b.basemaps.cartocdn.com/dark_nolabels/{z}/{x}/{y}@2x.png",
            "https://c.basemaps.cartocdn.com/dark_nolabels/{z}/{x}/{y}@2x.png"
          ],
          tileSize: 256,
          attribution: "© OpenStreetMap contributors © CARTO"
        }
      },
      layers: [
        { id: "space", type: "background", paint: { "background-color": "#05060a" } },
        { id: "carto", type: "raster", source: "carto", paint: { "raster-opacity": 0.92 } }
      ]
    },
    center: [0, 20],
    zoom: 1.7,
    pitch: mode === "person" ? 12 : 0,
    attributionControl: { compact: true }
  });
  map.addControl(new maplibregl.NavigationControl({ visualizePitch: true }));

  map.on("style.load", function () {
    try {
      map.setProjection({ type: "globe" });
    } catch (e) {
      // Older engines fall back to the flat map.
    }
    try {
      // A thin gold-tinted atmosphere lifts the globe off the star field.
      map.setSky({
        "sky-color": "#0a1020",
        "sky-horizon-blend": 0.6,
        "horizon-color": "#c88a2a",
        "horizon-fog-blend": 0.5,
        "fog-color": "#05060a",
        "fog-ground-blend": 0.7,
        "atmosphere-blend": 0.9
      });
    } catch (e) {
      // Older engines skip the atmosphere gracefully.
    }
  });

  if (mode === "landing") {
    spinIdle();
    return;
  }
  if (mode === "movie") {
    map.on("load", function () { addPins(readMoviePins(), false); });
    return;
  }
  if (mode === "person") {
    setupPerson();
  }

  // readMoviePins parses the inline pin block written by the movie template.
  function readMoviePins() {
    var el = document.getElementById("movie-pins");
    if (!el) {
      return [];
    }
    try {
      return JSON.parse(el.textContent) || [];
    } catch (e) {
      return [];
    }
  }

  // setupPerson fetches the person's pins and wires the filter box.
  function setupPerson() {
    var person = mount.getAttribute("data-person");
    var scope = mount.getAttribute("data-scope") || "focused";
    var countEl = mount.querySelector(".globe-count");
    var filterEl = mount.querySelector(".globe-filter");
    var entries = [];

    var url = "/globe/pins?person=" + encodeURIComponent(person) + "&scope=" + encodeURIComponent(scope);
    fetch(url)
      .then(function (r) {
        if (!r.ok) {
          throw new Error("pins " + r.status);
        }
        return r.json();
      })
      .then(function (data) {
        entries = addPins(data.pins || [], true);
        report(countEl, data, entries.length);
        if (filterEl) {
          filterEl.addEventListener("input", function () {
            applyFilter(entries, filterEl.value, countEl, data);
          });
        }
      })
      .catch(function () {
        if (countEl) {
          countEl.textContent = "Could not load filming locations.";
        }
      });
  }

  // addPins drops a marker for every pin and returns filter entries pairing
  // each marker with its lowercased search text. When clickable is true a
  // marker click opens its film page.
  function addPins(pins, clickable) {
    var entries = [];
    var bounds = new maplibregl.LngLatBounds();
    pins.forEach(function (pin) {
      var popup = new maplibregl.Popup({ offset: 16, closeButton: false }).setDOMContent(buildPopup(pin));
      var el = document.createElement("div");
      el.className = "gpin" + (pin.source === "country" ? " gpin-coarse" : "");
      var dot = document.createElement("div");
      dot.className = "gpin-dot";
      el.appendChild(dot);
      var marker = new maplibregl.Marker({ element: el, anchor: "center" })
        .setLngLat([pin.lon, pin.lat])
        .setPopup(popup)
        .addTo(map);
      el.addEventListener("mouseenter", function () { el.classList.add("hot"); popup.addTo(map); });
      el.addEventListener("mouseleave", function () { el.classList.remove("hot"); popup.remove(); });
      if (clickable && pin.movieUrl) {
        el.addEventListener("click", function () { window.location = pin.movieUrl; });
      }
      bounds.extend([pin.lon, pin.lat]);
      var hay = ((pin.film || "") + " " + (pin.name || "") + " " + (pin.role || "")).toLowerCase();
      entries.push({ marker: marker, hay: hay });
    });
    frame(pins, bounds);
    return entries;
  }

  // buildPopup renders the hover card for one pin.
  function buildPopup(pin) {
    var box = document.createElement("div");
    box.className = "pin-popup";
    var name = document.createElement("b");
    name.textContent = pin.name;
    box.appendChild(name);
    if (pin.film) {
      var film = document.createElement("div");
      film.className = "film";
      var label = pin.film + (pin.year ? " (" + pin.year + ")" : "");
      if (pin.movieUrl) {
        var link = document.createElement("a");
        link.href = pin.movieUrl;
        link.textContent = label;
        film.appendChild(link);
      } else {
        film.textContent = label;
      }
      if (pin.role) {
        film.appendChild(document.createTextNode(" · " + pin.role));
      }
      box.appendChild(film);
    }
    var links = document.createElement("div");
    links.className = "links";
    [["Maps ↗", pin.maps], ["Earth ↗", pin.earth]].forEach(function (pair) {
      if (!pair[1]) {
        return;
      }
      var a = document.createElement("a");
      a.href = pair[1];
      a.target = "_blank";
      a.rel = "noopener";
      a.textContent = pair[0];
      links.appendChild(a);
    });
    box.appendChild(links);
    return box;
  }

  // applyFilter shows only markers whose text contains the query and updates
  // the count line.
  function applyFilter(entries, raw, countEl, data) {
    var q = (raw || "").trim().toLowerCase();
    var shown = 0;
    entries.forEach(function (entry) {
      var match = q === "" || entry.hay.indexOf(q) !== -1;
      entry.marker.getElement().style.display = match ? "" : "none";
      if (match) {
        shown++;
      }
    });
    report(countEl, data, shown);
  }

  // report writes the pin and film tallies into the count line.
  function report(countEl, data, shown) {
    if (!countEl) {
      return;
    }
    if (!data.pins || data.pins.length === 0) {
      countEl.textContent = "No pinned filming locations found.";
      return;
    }
    var msg = shown + " of " + data.pins.length + " pins · " +
      data.resolved + "/" + data.films + " films mapped";
    if (data.truncated) {
      msg += " · focused view";
    }
    countEl.textContent = msg;
  }

  // frame fits the map to the pins, or notes when there are none.
  function frame(pins, bounds) {
    if (pins.length > 1) {
      map.fitBounds(bounds, { padding: 90, maxZoom: 8, duration: 1600 });
    } else if (pins.length === 1) {
      map.flyTo({ center: [pins[0].lon, pins[0].lat], zoom: 5.5, duration: 1600 });
    } else if (mode === "movie") {
      var note = document.createElement("div");
      note.className = "empty-note";
      note.textContent = "No pinned coordinates for this film yet.";
      document.body.appendChild(note);
    }
  }

  // spinIdle rotates the empty landing globe slowly for atmosphere.
  function spinIdle() {
    var lon = 0;
    map.on("load", function () {
      setInterval(function () {
        if (map.isMoving()) {
          return;
        }
        lon = (lon + 1.2) % 360;
        map.easeTo({ center: [lon > 180 ? lon - 360 : lon, 20], duration: 900, easing: function (t) { return t; } });
      }, 900);
    });
  }
})();
