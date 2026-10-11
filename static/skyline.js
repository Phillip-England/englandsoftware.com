(() => {
  const canvas = document.querySelector(".tulsa-skyline-canvas");
  const hero = document.querySelector(".chapter-hero");
  if (!canvas || !hero) return;
  const context = canvas.getContext("2d", { alpha: false });
  if (!context) return;

  // Tower positions follow a west-facing daylight view of downtown Tulsa.
  const buildings = [
    { x: 217, y: 365, w: 91, h: 85, cols: 8, rows: 7, color: "#afa898" },
    { x: 266, y: 220, w: 75, h: 230, cols: 9, rows: 21, color: "#d3ceb9", kind: "midcontinent" },
    { x: 354, y: 235, w: 64, h: 215, cols: 9, rows: 20, color: "#a6a9a8", kind: "firstplace" },
    { x: 391, y: 302, w: 52, h: 148, cols: 6, rows: 13, color: "#c2ad91", kind: "philtower" },
    { x: 426, y: 145, w: 116, h: 305, cols: 22, rows: 29, color: "#aeb9c2", kind: "bok" },
    { x: 550, y: 345, w: 66, h: 105, cols: 7, rows: 9, color: "#d2c6ac" },
    { x: 610, y: 290, w: 158, h: 160, cols: 15, rows: 14, color: "#414e59", kind: "dark-office" },
    { x: 326, y: 323, w: 104, h: 127, cols: 11, rows: 11, color: "#779695", kind: "glass-office" }
  ];
  const hash = (a, b) => {
    const n = Math.sin(a * 127.1 + b * 311.7) * 43758.5453;
    return n - Math.floor(n);
  };
  const windows = buildings.map((building, index) => {
    const points = [];
    for (let row = 0; row < building.rows; row++) {
      for (let col = 0; col < building.cols; col++) {
        if (hash(row + index * 19, col) < .42) continue;
        points.push({
          x: building.x + (col + .5) * building.w / building.cols,
          y: building.y + (row + .7) * building.h / building.rows,
          color: hash(col + index, row) > .62 ? "#ffd9a2" : "#a9d5f5",
          phase: hash(row + index * 11, col) * Math.PI * 2,
          width: building.kind === "bok" ? 1.8 : 3
        });
      }
    }
    return points;
  });
  const motion = window.matchMedia("(prefers-reduced-motion: reduce)");
  let width = 0;
  let height = 0;
  let cityScale = 1;
  let cityLeft = 0;
  let cityTop = 0;
  let frame = 0;
  let lastFrame = 0;
  let elapsed = 0;
  const shouldAnimate = () => !document.hidden && !motion.matches;

  function resize() {
    const rect = hero.getBoundingClientRect();
    width = rect.width;
    height = rect.height;
    const density = Math.min(window.devicePixelRatio || 1, 2);
    canvas.width = Math.round(width * density);
    canvas.height = Math.round(height * density);
    context.setTransform(density, 0, 0, density, 0, 0);
    const compact = width <= 720;
    cityScale = compact ? width / 1000 : Math.min(width / 1000, Math.max(.45, (height - 455) / 355));
    cityLeft = compact ? 0 : (width - 1000 * cityScale) / 2;
    cityTop = height - 500 * cityScale;
    draw(elapsed);
  }

  function drawSky(seconds) {
    const sky = context.createLinearGradient(0, 0, 0, height);
    sky.addColorStop(0, "#07142d");
    sky.addColorStop(.52, "#17345d");
    sky.addColorStop(1, "#335478");
    context.fillStyle = sky;
    context.fillRect(0, 0, width, height);

    const glow = context.createRadialGradient(width * .48, height * .8, 0, width * .48, height * .8, width * .65);
    glow.addColorStop(0, "#96b4d12c");
    glow.addColorStop(1, "#96b4d100");
    context.fillStyle = glow;
    context.fillRect(0, 0, width, height);

    // Sparse stars brighten and dim without drawing attention from the city.
    for (let star = 0; star < 42; star++) {
      const x = hash(star, 40) * width;
      const y = hash(star, 41) * height * .62;
      context.globalAlpha = motion.matches ? .36 : .25 + .17 * Math.sin(seconds * .6 + star);
      context.fillStyle = "#e2efff";
      context.fillRect(x, y, star % 7 ? 1.3 : 2, star % 7 ? 1.3 : 2);
    }
    context.globalAlpha = 1;

    // A pale moon sits away from the central headline.
    const moonX = width <= 720 ? width * .13 : width * .83;
    const moonY = height * .28;
    const moonRadius = Math.max(22, Math.min(width * .042, height * .052, 47));
    const halo = context.createRadialGradient(moonX, moonY, moonRadius * .45, moonX, moonY, moonRadius * 3.2);
    halo.addColorStop(0, "#dcecff8a");
    halo.addColorStop(.35, "#b7d7ff4d");
    halo.addColorStop(1, "#b7d7ff00");
    context.fillStyle = halo;
    context.fillRect(moonX - moonRadius * 3.2, moonY - moonRadius * 3.2, moonRadius * 6.4, moonRadius * 6.4);
    const moon = context.createRadialGradient(moonX - moonRadius * .2, moonY - moonRadius * .2, 0, moonX, moonY, moonRadius);
    moon.addColorStop(0, "#f9fcff");
    moon.addColorStop(.78, "#e8f2fb");
    moon.addColorStop(1, "#cbddeb");
    context.beginPath();
    context.arc(moonX, moonY, moonRadius, 0, Math.PI * 2);
    context.fillStyle = moon;
    context.fill();
    context.fillStyle = "#9db8c72b";
    for (const [dx, dy, radius] of [[-.3, -.22, .13], [.3, .08, .18], [-.08, .32, .11]]) {
      context.beginPath();
      context.arc(moonX + dx * moonRadius, moonY + dy * moonRadius, radius * moonRadius, 0, Math.PI * 2);
      context.fill();
    }

    // Moonlit cloud banks drift at different speeds.
    for (let index = 0; index < 7; index++) {
      const span = width * (.17 + hash(index, 3) * .18);
      const x = ((hash(index, 4) * (width + span * 2) + seconds * (5 + index % 3 * 2)) % (width + span * 2)) - span;
      const y = height * (.14 + hash(index, 5) * .48);
      const rise = Math.max(16, span * .075);
      context.save();
      context.globalAlpha = .18 + hash(index, 6) * .1;
      context.fillStyle = "#b7cbe1";
      context.shadowColor = "#c5d9ed";
      context.shadowBlur = 16;
      context.beginPath();
      context.moveTo(x, y + rise * .35);
      context.bezierCurveTo(x + span * .07, y - rise * .2, x + span * .16, y - rise * .25, x + span * .24, y - rise * .12);
      context.bezierCurveTo(x + span * .32, y - rise * 1.1, x + span * .45, y - rise * 1.2, x + span * .55, y - rise * .55);
      context.bezierCurveTo(x + span * .7, y - rise * .9, x + span * .82, y - rise * .5, x + span * .86, y - rise * .08);
      context.bezierCurveTo(x + span * .97, y - rise * .02, x + span, y + rise * .35, x + span, y + rise * .35);
      context.bezierCurveTo(x + span * .7, y + rise * .75, x + span * .3, y + rise * .85, x, y + rise * .35);
      context.closePath();
      context.fill();
      context.restore();
    }
  }

  function drawBokCenter() {
    // The arena is a low, oval metal shell with a long, curving glass wall.
    // Its smaller, softer silhouette sits behind the street beside the office tower.
    context.save();
    context.globalAlpha = .8;
    context.fillStyle = "#43576b";
    context.fillRect(776, 416, 139, 30);
    context.fillStyle = "#718693";
    context.fillRect(776, 416, 139, 2);
    context.fillStyle = "#9bb0b687";
    for (let x = 785; x < 909; x += 12) context.fillRect(x, 429, 5, 2);
    context.translate(775, 420);
    context.scale(.65, .65);
    context.translate(-775, -450);
    context.translate(775, 0);
    context.scale(.84, 1);
    context.translate(-775, 0);
    const shell = context.createLinearGradient(772, 354, 1019, 456);
    shell.addColorStop(0, "#9badb7");
    shell.addColorStop(.43, "#d0d7d6");
    shell.addColorStop(1, "#667f91");
    context.beginPath();
    context.moveTo(778, 405);
    context.bezierCurveTo(780, 375, 825, 356, 886, 352);
    context.bezierCurveTo(957, 346, 1011, 364, 1023, 395);
    context.bezierCurveTo(1030, 414, 1013, 447, 985, 454);
    context.bezierCurveTo(923, 473, 825, 461, 792, 441);
    context.bezierCurveTo(780, 433, 775, 418, 778, 405);
    context.closePath();
    context.fillStyle = shell;
    context.fill();

    // The shallow oval roof is visible just above the tall outer metal band.
    context.beginPath();
    context.ellipse(898, 379, 112, 25, -.025, 0, Math.PI * 2);
    context.fillStyle = "#748893";
    context.fill();
    context.strokeStyle = "#d2dad9";
    context.lineWidth = 3;
    context.stroke();
    context.beginPath();
    context.ellipse(898, 381, 103, 19, -.025, Math.PI * 1.03, Math.PI * 1.96);
    context.strokeStyle = "#8fa5b1";
    context.lineWidth = 1.5;
    context.stroke();

    // A tapered silver ribbon crosses the roof and drops into the facade.
    context.beginPath();
    context.moveTo(804, 379);
    context.bezierCurveTo(844, 391, 918, 397, 983, 382);
    context.bezierCurveTo(999, 378, 1011, 373, 1018, 370);
    context.lineTo(1018, 385);
    context.bezierCurveTo(968, 418, 862, 414, 806, 396);
    context.closePath();
    context.fillStyle = "#b5c3ca";
    context.fill();

    // The sloping glass curtain wall wraps the public entrance side.
    context.beginPath();
    context.moveTo(792, 401);
    context.bezierCurveTo(858, 421, 957, 418, 1019, 384);
    context.bezierCurveTo(1027, 406, 1010, 437, 986, 445);
    context.bezierCurveTo(930, 461, 843, 452, 805, 432);
    context.closePath();
    const glass = context.createLinearGradient(795, 400, 1020, 450);
    glass.addColorStop(0, "#244b5d");
    glass.addColorStop(.5, "#507988");
    glass.addColorStop(1, "#193e58");
    context.fillStyle = glass;
    context.fill();
    context.save();
    context.clip();
    context.strokeStyle = "#bfd1d0aa";
    context.lineWidth = 1;
    for (let x = 792; x < 1032; x += 12) {
      context.beginPath();
      context.moveTo(x, 385);
      context.lineTo(x - 8, 465);
      context.stroke();
    }
    context.strokeStyle = "#a9c4c382";
    for (let y = 408; y < 455; y += 9) {
      context.beginPath();
      context.moveTo(788, y);
      context.bezierCurveTo(855, y + 12, 962, y + 10, 1030, y - 25);
      context.stroke();
    }
    context.restore();

    // The metal edge curls down at the near end, like the arena's sliver roofs.
    context.beginPath();
    context.moveTo(778, 403);
    context.bezierCurveTo(773, 421, 779, 436, 795, 446);
    context.bezierCurveTo(813, 455, 835, 457, 857, 457);
    context.lineTo(867, 448);
    context.bezierCurveTo(831, 445, 803, 428, 792, 401);
    context.closePath();
    context.fillStyle = "#adbcc4";
    context.fill();
    context.strokeStyle = "#e2e7e3a8";
    context.lineWidth = 2;
    context.stroke();
    context.beginPath();
    context.moveTo(792, 401);
    context.bezierCurveTo(858, 421, 957, 418, 1019, 384);
    context.strokeStyle = "#e4e6dd";
    context.lineWidth = 2.5;
    context.stroke();
    context.restore();
  }

  function drawRearTowers(seconds) {
    // University Club Tower: a round glass shaft, saucer-like cap, and spire.
    const clubGlass = context.createLinearGradient(565, 0, 633, 0);
    clubGlass.addColorStop(0, "#536b79");
    clubGlass.addColorStop(.4, "#91a7ae");
    clubGlass.addColorStop(1, "#3b566a");
    context.fillStyle = clubGlass;
    context.beginPath();
    context.moveTo(566, 225);
    context.quadraticCurveTo(600, 217, 634, 225);
    context.lineTo(634, 450);
    context.lineTo(566, 450);
    context.closePath();
    context.fill();
    context.save();
    context.beginPath();
    context.rect(566, 225, 68, 225);
    context.clip();
    context.strokeStyle = "#c8d8d39c";
    context.lineWidth = 1.3;
    for (let x = 573; x < 637; x += 9) {
      context.beginPath();
      context.moveTo(x, 224);
      context.lineTo(x, 450);
      context.stroke();
    }
    for (let y = 235; y < 450; y += 8) {
      context.strokeStyle = y % 3 ? "#1d405c9c" : "#d3dec88a";
      context.beginPath();
      context.moveTo(566, y);
      context.lineTo(634, y);
      context.stroke();
      for (let x = 573; x < 629; x += 18) {
        if (hash(x, y) > .68) {
          context.globalAlpha = .55 + .18 * Math.sin(seconds * .4 + x);
          context.fillStyle = "#f5d7a8";
          context.fillRect(x, y + 2, 4, 3);
        }
      }
    }
    context.restore();
    context.globalAlpha = 1;
    context.fillStyle = "#b5c4c8";
    context.beginPath();
    context.moveTo(559, 221);
    context.quadraticCurveTo(600, 203, 641, 221);
    context.quadraticCurveTo(600, 233, 559, 221);
    context.fill();
    context.fillStyle = "#d1dad8";
    context.fillRect(593, 193, 14, 16);
    context.strokeStyle = "#b9c9cc";
    context.lineWidth = 2;
    context.beginPath();
    context.moveTo(600, 193);
    context.lineTo(600, 151);
    context.stroke();
    context.lineWidth = 1.5;
    for (const x of [575, 625]) {
      context.beginPath();
      context.moveTo(x, 216);
      context.lineTo(600, 193);
      context.stroke();
    }

    // A narrow, pale rectangular tower is partly hidden by the dark office.
    const gray = context.createLinearGradient(688, 0, 738, 0);
    gray.addColorStop(0, "#91a5ae");
    gray.addColorStop(.65, "#c2cecc");
    gray.addColorStop(1, "#748d9b");
    context.fillStyle = gray;
    context.fillRect(688, 186, 50, 264);
    context.fillStyle = "#d8dfda";
    context.fillRect(686, 182, 54, 6);
    context.fillStyle = "#5a7487";
    context.fillRect(693, 192, 40, 7);
    context.fillStyle = "#577185a3";
    for (let x = 693; x < 738; x += 7) {
      context.fillRect(x, 201, 2, 249);
    }
    context.fillStyle = "#dce1da7a";
    for (let y = 207; y < 450; y += 10) {
      context.fillRect(690, y, 47, 1);
    }
  }

  function drawRoute66Landmarks(seconds) {
    // The Golden Driller stands to the far left, with his hand on an oil derrick.
    context.save();
    context.strokeStyle = "#78919b";
    context.lineWidth = 2;
    context.strokeRect(4, -94, 31, 91);
    for (let y = -91; y < -9; y += 17) {
      context.beginPath();
      context.moveTo(5, y);
      context.lineTo(34, y + 16);
      context.moveTo(34, y);
      context.lineTo(5, y + 16);
      context.stroke();
    }
    context.fillStyle = "#526978";
    context.fillRect(1, -96, 37, 5);
    context.fillRect(12, -104, 15, 8);
    context.fillStyle = "#ba9050";
    context.fillRect(12, -103, 15, 2);

    const gold = context.createLinearGradient(35, -120, 101, 0);
    gold.addColorStop(0, "#ffdf83");
    gold.addColorStop(.5, "#c99b45");
    gold.addColorStop(1, "#9b7337");
    context.fillStyle = gold;
    context.strokeStyle = "#7e5c2c";
    context.lineWidth = 1.5;
    context.beginPath();
    context.moveTo(50, -101);
    context.lineTo(38, -97);
    context.lineTo(29, -89);
    context.lineTo(20, -92);
    context.lineTo(18, -86);
    context.lineTo(29, -82);
    context.lineTo(43, -91);
    context.lineTo(47, -55);
    context.lineTo(43, -8);
    context.lineTo(39, -5);
    context.lineTo(39, -1);
    context.lineTo(55, -1);
    context.lineTo(61, -54);
    context.lineTo(79, -8);
    context.lineTo(75, -5);
    context.lineTo(75, -1);
    context.lineTo(94, -1);
    context.lineTo(90, -11);
    context.lineTo(73, -63);
    context.lineTo(76, -88);
    context.lineTo(88, -83);
    context.lineTo(95, -72);
    context.lineTo(82, -64);
    context.lineTo(78, -69);
    context.lineTo(89, -74);
    context.lineTo(85, -88);
    context.lineTo(75, -99);
    context.closePath();
    context.fill();
    context.stroke();
    context.beginPath();
    context.ellipse(62, -109, 12, 14, -.08, 0, Math.PI * 2);
    context.fill();
    context.stroke();
    context.beginPath();
    context.ellipse(62, -120, 16, 5, 0, 0, Math.PI * 2);
    context.fillStyle = "#d3a653";
    context.fill();
    context.fillRect(50, -126, 24, 7);
    context.strokeStyle = "#8e6831";
    context.beginPath();
    context.moveTo(54, -94);
    context.lineTo(61, -91);
    context.lineTo(69, -95);
    context.moveTo(53, -71);
    context.lineTo(71, -71);
    context.stroke();
    context.fillStyle = "#6b4c2b";
    context.font = "bold 7px Arial, sans-serif";
    context.textAlign = "center";
    context.fillText("TULSA", 63, -65);
    context.fillStyle = "#43566b";
    context.fillRect(0, -2, 100, 5);
    context.restore();

    // Meadow Gold's clock, open lattice, and yellow, red, and green neon.
    context.save();
    context.translate(117, 0);
    context.strokeStyle = "#8a9aa0";
    context.lineWidth = 1.3;
    for (const x of [4, 122]) {
      context.beginPath();
      context.moveTo(x, -80);
      context.lineTo(x, -14);
      context.stroke();
    }
    for (let y = -76; y < -18; y += 13) {
      context.beginPath();
      context.moveTo(5, y);
      context.lineTo(121, y + 13);
      context.moveTo(121, y);
      context.lineTo(5, y + 13);
      context.stroke();
    }
    context.fillStyle = "#a15443";
    context.fillRect(0, -15, 128, 15);
    context.fillStyle = "#492f31";
    for (const x of [13, 43, 73, 103]) {
      context.beginPath();
      context.arc(x, 0, 9, Math.PI, 0);
      context.fill();
    }
    context.strokeStyle = "#82ec71";
    context.shadowColor = "#66e866";
    context.shadowBlur = motion.matches ? 4 : 7 + Math.sin(seconds * 1.1) * 2;
    context.lineWidth = 2;
    context.strokeRect(1, -81, 126, 5);
    context.fillStyle = "#c83f46";
    context.fillRect(46, -109, 37, 30);
    context.beginPath();
    context.arc(64.5, -94, 13, 0, Math.PI * 2);
    context.fillStyle = "#e9e8d7";
    context.fill();
    context.fillStyle = "#cd5152";
    context.beginPath();
    context.arc(64.5, -94, 6, 0, Math.PI * 2);
    context.fill();
    context.strokeStyle = "#344657";
    context.lineWidth = 1.3;
    context.beginPath();
    context.moveTo(64.5, -94);
    context.lineTo(64.5, -103);
    context.moveTo(64.5, -94);
    context.lineTo(58, -89);
    context.stroke();
    context.textAlign = "center";
    context.fillStyle = "#ffe67a";
    context.shadowColor = "#ffd94a";
    context.shadowBlur = motion.matches ? 3 : 9;
    context.font = "bold 15px Arial, sans-serif";
    context.fillText("Meadow", 64, -60);
    context.font = "bold 23px Arial, sans-serif";
    context.fillText("Gold", 64, -39);
    context.shadowColor = "#ff6865";
    context.shadowBlur = motion.matches ? 2 : 6;
    context.fillStyle = "#b83f42";
    context.fillRect(4, -34, 120, 11);
    context.fillStyle = "#fff1dc";
    context.font = "bold 8px Arial, sans-serif";
    context.fillText("MILK · ICE CREAM", 64, -26);
    context.shadowColor = "#75ed68";
    context.fillStyle = "#266b4b";
    context.fillRect(1, -22, 126, 10);
    context.fillStyle = "#caff9a";
    context.font = "bold 7px Arial, sans-serif";
    context.fillText("BEATRICE FOODS CO.", 64, -14);
    context.restore();
  }

  function drawCity(seconds) {
    // Low outer blocks keep the panorama grounded when the screen is very wide.
    context.fillStyle = "#263e5a";
    for (let x = 0; x < width; x += 31) {
      context.fillRect(x, height - 62 - hash(x, 14) * 45, 33, 108);
    }
    context.save();
    context.translate(cityLeft, cityTop);
    context.scale(cityScale, cityScale);

    // The distant tree line and low roofs sit behind the tower cluster.
    context.fillStyle = "#25464e";
    context.fillRect(-30, 391, 1060, 24);
    context.fillStyle = "#344d68";
    for (let x = -30; x < 1030; x += 23) {
      context.fillRect(x, 392 + hash(x, 7) * 37, 25, 65);
    }
    // The Golden Driller and Meadow Gold sign sit where the distant towers were.
    context.save();
    context.translate(20, 420);
    context.scale(.66, .66);
    context.globalAlpha = .82;
    drawRoute66Landmarks(seconds);
    context.restore();
    drawRearTowers(seconds);
    drawBokCenter();
    buildings.forEach((building, index) => {
      const { x, y, w, h } = building;
      const facade = context.createLinearGradient(x, y, x + w, y);
      facade.addColorStop(0, building.color);
      facade.addColorStop(.72, building.color);
      facade.addColorStop(1, building.kind === "midcontinent" ? "#aaa994" : building.kind === "bok" ? "#dce2e3" : "#7895a6");
      context.fillStyle = facade;
      context.fillRect(x, y, w, h);
      context.fillStyle = "#d7e4e4a6";
      context.fillRect(x, y, w, 3);
      context.fillStyle = building.kind === "midcontinent" ? "#817f7566" : "#173c6647";
      context.fillRect(x + w - Math.max(5, w * .13), y + 3, Math.max(5, w * .13), h - 3);
      if (building.kind === "bok") {
        // The tall BOK tower has a bright concrete edge and close vertical ribs.
        context.fillStyle = "#e3e7e5";
        context.fillRect(x - 2, y - 5, w + 4, 5);
        context.fillStyle = "#526878";
        context.fillRect(x + 5, y + 5, w - 10, 9);
        context.fillStyle = "#e3e8e4a1";
        for (let rib = x + 5; rib < x + w - 5; rib += 5) {
          context.fillRect(rib, y + 15, 1.5, h - 15);
        }
      }
      if (building.kind === "firstplace") {
        context.fillStyle = "#e4e1d3b0";
        for (let row = y + 12; row < y + h; row += 9) context.fillRect(x + 2, row, w - 4, 1.5);
      }
      if (building.kind === "philtower") {
        // Philtower has a smaller, multicolored tiled crown.
        context.fillStyle = "#d5c7ad";
        context.fillRect(x + 5, y - 10, w - 10, 12);
        context.beginPath();
        context.moveTo(x + 9, y - 10);
        context.lineTo(x + 24, y - 41);
        context.lineTo(x + w - 24, y - 41);
        context.lineTo(x + w - 9, y - 10);
        context.closePath();
        context.fillStyle = "#357c75";
        context.fill();
        context.fillStyle = "#b46b5a";
        for (let stripe = 0; stripe < 4; stripe++) {
          context.fillRect(x + 17 + stripe * 9, y - 25, 3, 9);
        }
        context.fillStyle = "#337772";
        context.fillRect(x + w / 2 - 4, y - 49, 8, 9);
      }
      if (building.kind === "midcontinent") {
        // Mid-Continent's broad green copper mansard roof and pale terra-cotta tower.
        context.fillStyle = "#e6dfc9";
        context.fillRect(x - 3, y - 3, w + 6, 8);
        context.beginPath();
        context.moveTo(x - 2, y - 3);
        context.lineTo(x + 10, y - 40);
        context.lineTo(x + w - 10, y - 40);
        context.lineTo(x + w + 2, y - 3);
        context.closePath();
        context.fillStyle = "#158b75";
        context.fill();
        context.beginPath();
        context.moveTo(x + 10, y - 40);
        context.lineTo(x + 22, y - 37);
        context.lineTo(x + 10, y - 3);
        context.lineTo(x - 2, y - 3);
        context.closePath();
        context.fillStyle = "#58b994";
        context.fill();
        context.strokeStyle = "#a2dec1a6";
        context.lineWidth = 1;
        for (let seam = 1; seam < 7; seam++) {
          const roofX = x + seam * w / 7;
          context.beginPath();
          context.moveTo(roofX, y - 38);
          context.lineTo(roofX - 5, y - 4);
          context.stroke();
        }
        context.fillStyle = "#0d665b";
        context.fillRect(x + 10, y - 43, w - 20, 4);
        for (let post = x + 11; post < x + w - 8; post += 7) {
          context.fillRect(post, y - 48, 2, 6);
        }
        context.fillStyle = "#9d917b";
        for (let arch = 0; arch < 7; arch++) {
          context.fillRect(x + 8 + arch * 11, y + 9, 6, 10);
        }
        context.fillStyle = "#eee4cc";
        for (let pier = 0; pier < 8; pier++) {
          context.fillRect(x + pier * 11, y + 19, 3, h - 19);
        }
      }
      if (building.kind === "dark-office") {
        context.fillStyle = "#b6c3ca9c";
        for (let row = y + 10; row < y + h; row += 12) context.fillRect(x + 2, row, w - 4, 2);
      }
      if (building.kind === "glass-office") {
        context.strokeStyle = "#c7d9d0a1";
        context.lineWidth = 1;
        for (let col = x + 9; col < x + w; col += 11) {
          context.beginPath();
          context.moveTo(col, y);
          context.lineTo(col, y + h);
          context.stroke();
        }
      }
      if (!building.kind && w < 60) {
        context.fillStyle = "#d1dce087";
        context.fillRect(x + w * .5, y - 13, 2, 13);
      }
      if (w > 150) {
        context.fillStyle = "#d3dfe188";
        context.fillRect(x + 3, y + 7, w - 6, 4);
      }
      context.fillStyle = "#071b3870";
      context.fillRect(x, y, w, h);
      // Lit windows vary gently while each facade keeps its own grid.
      for (const light of windows[index]) {
        context.globalAlpha = motion.matches ? .68 : .55 + .27 * Math.sin(seconds * .65 + light.phase);
        context.fillStyle = light.color;
        context.fillRect(light.x, light.y, light.width, 3);
      }
      context.globalAlpha = 1;
    });

    // Parking decks and older low blocks overlap the tower bases.
    for (const [x, y, w, h, color] of [
      [0, 420, 224, 38, "#52637a"],
      [182, 428, 145, 30, "#61718a"],
      [429, 419, 177, 39, "#526581"]
    ]) {
      context.fillStyle = color;
      context.fillRect(x, y, w, h);
      context.fillStyle = "#9aafbb";
      context.fillRect(x, y, w, 4);
      context.fillStyle = "#233a57";
      for (let opening = x + 7; opening < x + w - 6; opening += 16) {
        context.fillRect(opening, y + 12, 9, 4);
        context.fillRect(opening, y + 24, 9, 4);
      }
    }

    // Small street traffic stays close to the base of the composition.
    context.fillStyle = "#172d49";
    context.fillRect(0, 446, 1000, 54);
    context.fillStyle = "#b4bacc4a";
    context.fillRect(0, 452, 1000, 2);
    for (let car = 0; car < 8; car++) {
      const x = ((car * 157 + seconds * (car % 2 ? 16 : -13)) % 1130 + 1130) % 1130 - 65;
      const y = 462 + hash(car, 18) * 21;
      const color = car % 2 ? "#ffd1a1" : "#ff7e82";
      context.fillStyle = color;
      context.shadowColor = color;
      context.shadowBlur = 9;
      context.fillRect(x, y, 5, 2);
    }
    context.shadowBlur = 0;

    // Tree crowns and low roofs define the bottom edge.
    context.fillStyle = "#102d34";
    for (let x = -20; x < 1040; x += 28) {
      const crown = 468 + hash(x, 21) * 18;
      context.beginPath();
      context.arc(x, crown, 13 + hash(x, 22) * 12, 0, Math.PI * 2);
      context.fill();
    }
    context.fillRect(0, 484, 1000, 16);
    context.restore();
  }

  function draw(seconds) {
    drawSky(seconds);
    drawCity(seconds);
  }

  function animate(now) {
    frame = 0;
    if (!shouldAnimate()) return;
    if (now - lastFrame >= 32) {
      lastFrame = now;
      elapsed = now / 1000;
      draw(elapsed);
    }
    frame = requestAnimationFrame(animate);
  }

  function updateMotion() {
    cancelAnimationFrame(frame);
    frame = 0;
    draw(elapsed);
    if (shouldAnimate()) frame = requestAnimationFrame(animate);
  }

  new ResizeObserver(resize).observe(hero);
  document.addEventListener("visibilitychange", updateMotion);
  motion.addEventListener("change", updateMotion);
  resize();
  updateMotion();
})();
