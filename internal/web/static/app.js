// Bhagirath dashboard behaviour: live countdowns, maps, small conveniences.
// No framework; everything degrades to plain server-rendered HTML.
(function () {
  "use strict";
  var body = document.body;
  var serverNow = Date.parse(body.getAttribute("data-now") || "");
  var skew = isNaN(serverNow) ? 0 : serverNow - Date.now(); // demo clock offset

  function fmt(ms) {
    var neg = ms < 0, s = Math.abs(ms) / 1000;
    var d = Math.floor(s / 86400), h = Math.floor((s % 86400) / 3600), m = Math.floor((s % 3600) / 60);
    var out = d > 0 ? d + "d " + h + "h" : h > 0 ? h + "h " + m + "m" : m + "m " + Math.floor(s % 60) + "s";
    return neg ? "overdue by " + out : out + " left";
  }
  function tick() {
    var now = Date.now() + skew;
    document.querySelectorAll("[data-deadline]").forEach(function (el) {
      var left = Date.parse(el.getAttribute("data-deadline")) - now;
      el.textContent = fmt(left);
      el.classList.toggle("overdue", left < 0);
      el.classList.toggle("soon", left >= 0 && left < 6 * 3600 * 1000);
    });
  }
  tick();
  setInterval(tick, 1000);

  // Print buttons (inline handlers are blocked by the CSP).
  document.querySelectorAll("[data-print]").forEach(function (b) { b.addEventListener("click", function () { window.print(); }); });

  // Confirm destructive or outward-facing actions.
  document.addEventListener("submit", function (e) {
    var msg = e.target.getAttribute("data-confirm");
    if (msg && !window.confirm(msg)) e.preventDefault();
  });

  // "Use my location" on the public report form.
  var gps = document.getElementById("use-gps");
  if (gps && navigator.geolocation) {
    gps.hidden = false;
    gps.addEventListener("click", function () {
      gps.disabled = true;
      navigator.geolocation.getCurrentPosition(function (p) {
        document.getElementById("lat").value = p.coords.latitude.toFixed(6);
        document.getElementById("lon").value = p.coords.longitude.toFixed(6);
        gps.textContent = "✓ " + p.coords.latitude.toFixed(4) + ", " + p.coords.longitude.toFixed(4);
      }, function () { gps.disabled = false; }, { enableHighAccuracy: true, timeout: 10000 });
    });
  }

  // Maps (Leaflet is loaded only on pages that need it).
  var colors = { watch: "#d18a00", warning: "#e0661b", critical: "#c62828", "": "#2f855a" };
  var pinColors = { open: "#0d6b8c", escalated: "#0d6b8c", claimed_fixed: "#d18a00", verified_fixed: "#2f855a", reopened: "#c62828", invalid: "#999" };
  function drawWards(map, wards, bounds, link) {
    // Focus on wards with an active alert (most severe first); show all otherwise.
    var rank = { critical: 3, warning: 2, watch: 1 }, top = 0;
    wards.forEach(function (w) { top = Math.max(top, rank[w.Level] || 0); });
    wards.forEach(function (w) {
      var ring = (w.Polygon && w.Polygon[0] || []).map(function (p) { return [p.lat, p.lon]; });
      if (!ring.length) return;
      var focus = top === 0 || (rank[w.Level] || 0) === top;
      var poly = L.polygon(ring, { color: colors[w.Level || ""], weight: 2, fillOpacity: w.Level ? 0.18 : 0.06 }).addTo(map);
      var label = "<b>" + esc(w.Name) + "</b><br>" + (w.Level ? w.Level.toUpperCase() + " alert" : "No active alert");
      if (link && w.AlertID) label += '<br><a href="/app/alerts/' + w.AlertID + '">Open alert →</a>';
      poly.bindPopup(label);
      if (focus) ring.forEach(function (p) { bounds.push(p); });
    });
  }
  function esc(s) { return String(s).replace(/[&<>"']/g, function (c) { return { "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c]; }); }
  function baseMap(el) {
    var map = L.map(el, { scrollWheelZoom: false });
    L.tileLayer("https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png", { maxZoom: 19, attribution: "© OpenStreetMap contributors" }).addTo(map);
    return map;
  }
  var mapEl = document.getElementById("map");
  if (mapEl && window.L) {
    var map = baseMap(mapEl);
    fetch("/api/v1/map", { credentials: "same-origin" }).then(function (r) { return r.json(); }).then(function (d) {
      var bounds = [];
      drawWards(map, d.wards || [], bounds, true);
      (d.complaints || []).forEach(function (c) {
        L.circleMarker([c.Lat, c.Lon], { radius: 6, color: "#fff", weight: 1.5, fillColor: pinColors[c.Status] || "#0d6b8c", fillOpacity: 0.95 })
          .bindPopup('<a href="/app/complaints/' + esc(c.Ref) + '"><b>' + esc(c.Ref) + "</b></a><br>" + esc(c.Issue) + "<br>" + esc(c.Status.replace(/_/g, " ")) + " · " + esc(c.Channel.replace(/_/g, " ")))
          .addTo(map);
      });
      if (bounds.length) map.fitBounds(bounds, { padding: [20, 20], maxZoom: 16 });
    });
  }
  // Static maps with inline data (evidence pack, complaint page).
  document.querySelectorAll("[data-pins]").forEach(function (el) {
    if (!window.L) return;
    var pins = JSON.parse(el.getAttribute("data-pins") || "[]") || [];
    var ring = JSON.parse(el.getAttribute("data-ring") || "[]") || [];
    var m = baseMap(el), bounds = [];
    if (ring.length) {
      var latlngs = ring.map(function (p) { return [p.lat, p.lon]; });
      L.polygon(latlngs, { color: "#0d6b8c", weight: 2, fillOpacity: 0.05 }).addTo(m);
      bounds = bounds.concat(latlngs);
    }
    pins.forEach(function (p) {
      L.circleMarker([p.Lat, p.Lon], { radius: 7, color: "#fff", weight: 1.5, fillColor: "#c62828", fillOpacity: 0.95 }).bindPopup(esc(p.Ref + " · " + p.Issue)).addTo(m);
      bounds.push([p.Lat, p.Lon]);
    });
    if (bounds.length) m.fitBounds(bounds, { padding: [24, 24], maxZoom: 17 });
  });
  // Click-to-pick a point on the complaint page.
  var pick = document.getElementById("pickmap");
  if (pick && window.L) {
    var pm = baseMap(pick), mk = null;
    var lat0 = parseFloat(pick.getAttribute("data-lat")), lon0 = parseFloat(pick.getAttribute("data-lon"));
    pm.setView([isNaN(lat0) ? 22.72 : lat0, isNaN(lon0) ? 75.86 : lon0], 16);
    pm.on("click", function (e) {
      document.getElementById("lat").value = e.latlng.lat.toFixed(6);
      document.getElementById("lon").value = e.latlng.lng.toFixed(6);
      if (mk) mk.setLatLng(e.latlng); else mk = L.marker(e.latlng).addTo(pm);
    });
  }
})();
