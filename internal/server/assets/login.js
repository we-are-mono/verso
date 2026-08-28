// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

// The login hero's constellation is a true 3D scene: each device node sits at
// its own depth (z) around the router, which is fixed at the origin — the
// rotation centre — so it never leaves the middle of the mesh. The scene rotates
// toward the cursor; every node is re-projected each frame and its edge to the
// router redrawn, so the wiring always stays connected. External (not inline) to
// satisfy the strict script-src 'self' CSP; honours reduced-motion; a no-op on
// touch / no-mouse.
(function () {
  var svg = document.getElementById("verso-constellation");
  var mesh = document.getElementById("verso-mesh");
  if (!svg || !mesh) return;

  var NS = "http://www.w3.org/2000/svg";
  var CX = 220, CY = 160, F = 620; // centre (the router) and the projection focal length

  // Device nodes: [x, y, z, colour], centred on the router at the origin. z is
  // each node's own plane — some floated forward, some pushed back. Green nodes
  // read as active devices, the lone blue node on the left as the WAN uplink,
  // slate as idle.
  var nodes = [
    [-235,  25,  35, "#38bdf8"], // WAN uplink — the internet, far out on the longest line
    [  95, -80,  55, "#34d399"], // active
    [ 140,  50, -50, "#34d399"], // active
    [  60, 125,  45, "#34d399"], // active
    [ -85, -75, -45, "#cbd5e1"], // idle
    [  25, -120, 60, "#cbd5e1"], // idle
    [ 120,  -5,  50, "#34d399"], // active
    [ -70, 105,  55, "#cbd5e1"]  // idle
  ];

  // Build an edge + a dot per node (edges first, so dots paint on top).
  var els = nodes.map(function (n) {
    var line = document.createElementNS(NS, "line");
    line.setAttribute("x1", CX); line.setAttribute("y1", CY);
    line.setAttribute("stroke", "#e2e8f0"); line.setAttribute("stroke-width", "1.5");
    mesh.appendChild(line);
    return { line: line };
  });
  els.forEach(function (e, i) {
    var dot = document.createElementNS(NS, "circle");
    dot.setAttribute("fill", nodes[i][3]);
    mesh.appendChild(dot);
    e.dot = dot;
  });

  function clamp(v, lo, hi) { return v < lo ? lo : v > hi ? hi : v; }

  // Rotate every node by (ax around X, ay around Y), project to 2D, and lay out
  // its edge + dot. Closer nodes read larger and more solid.
  function place(ax, ay) {
    var cy = Math.cos(ay), sy = Math.sin(ay), cx = Math.cos(ax), sx = Math.sin(ax);
    for (var i = 0; i < els.length; i++) {
      var n = nodes[i];
      var x1 = n[0] * cy + n[2] * sy;
      var z1 = -n[0] * sy + n[2] * cy;
      var y1 = n[1] * cx - z1 * sx;
      var z2 = n[1] * sx + z1 * cx;
      var s = F / (F - z2);
      var px = CX + x1 * s, py = CY + y1 * s;
      var op = clamp(0.4 + (s - 0.85) * 1.8, 0.4, 1);
      var e = els[i];
      e.line.setAttribute("x2", px.toFixed(2));
      e.line.setAttribute("y2", py.toFixed(2));
      e.line.setAttribute("opacity", (op * 0.8).toFixed(2));
      e.dot.setAttribute("cx", px.toFixed(2));
      e.dot.setAttribute("cy", py.toFixed(2));
      e.dot.setAttribute("r", (4.6 * s).toFixed(2));
      e.dot.setAttribute("opacity", op.toFixed(2));
    }
  }

  place(0, 0); // resting pose

  if (window.matchMedia && window.matchMedia("(prefers-reduced-motion: reduce)").matches) return;

  var MAX = 0.55; // radians of tilt at the screen edges (~31°)
  var tx = 0, ty = 0, rx = 0, ry = 0, raf = 0;
  function frame() {
    rx += (tx - rx) * 0.1;
    ry += (ty - ry) * 0.1;
    place(rx, ry);
    if (Math.abs(tx - rx) > 0.0005 || Math.abs(ty - ry) > 0.0005) {
      raf = requestAnimationFrame(frame);
    } else {
      raf = 0;
    }
  }
  function nudge() { if (!raf) raf = requestAnimationFrame(frame); }
  window.addEventListener("mousemove", function (e) {
    ty = ((e.clientX / window.innerWidth) * 2 - 1) * MAX;   // yaw follows horizontal
    tx = -((e.clientY / window.innerHeight) * 2 - 1) * MAX; // pitch follows vertical
    nudge();
  });
  document.addEventListener("mouseleave", function () { tx = 0; ty = 0; nudge(); });
})();
