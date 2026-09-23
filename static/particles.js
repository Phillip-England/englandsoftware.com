const canvas = document.querySelector('#edge-particles');
const context = canvas?.getContext('2d');

if (context) {
  const reducedMotion = window.matchMedia('(prefers-reduced-motion: reduce)');
  const particles = [];
  let width = 0;
  let height = 0;
  let frame = 0;
  let running = false;

  function reset() {
    width = window.innerWidth;
    height = window.innerHeight;
    const ratio = Math.min(window.devicePixelRatio || 1, 2);
    canvas.width = Math.round(width * ratio);
    canvas.height = Math.round(height * ratio);
    context.setTransform(ratio, 0, 0, ratio, 0, 0);
    particles.length = 0;
    const count = Math.min(42, Math.max(20, Math.floor(width / 32)));
    for (let i = 0; i < count; i += 1) {
      particles.push({
        x: Math.random() * width,
        y: Math.random() * height,
        radius: 0.8 + Math.random() * 1.1,
        speed: 0.035 + Math.random() * 0.07,
        phase: Math.random() * Math.PI * 2,
      });
    }
    draw();
  }

  function draw() {
    context.clearRect(0, 0, width, height);
    for (const particle of particles) {
      const edge = Math.min(particle.x, width - particle.x) / Math.max(width, 1);
      const fade = Math.max(0.25, 1 - edge * 1.8);
      const alpha = fade * (0.09 + 0.035 * Math.sin(particle.phase));
      context.fillStyle = `rgba(63, 104, 232, ${alpha})`;
      context.beginPath();
      context.arc(particle.x, particle.y, particle.radius, 0, Math.PI * 2);
      context.fill();
    }
  }

  function animate() {
    if (document.hidden || reducedMotion.matches) { running = false; return; }
    frame += 1;
    for (const particle of particles) {
      particle.y -= particle.speed;
      particle.phase += 0.008;
      if (particle.y < -3) particle.y = height + 3;
    }
    if (frame % 2 === 0) draw();
    requestAnimationFrame(animate);
  }

  function start() {
    draw();
    if (!running && !document.hidden && !reducedMotion.matches) {
      running = true;
      requestAnimationFrame(animate);
    }
  }

  window.addEventListener('resize', reset, { passive: true });
  document.addEventListener('visibilitychange', start);
  reducedMotion.addEventListener('change', start);
  reset();
  start();
}
