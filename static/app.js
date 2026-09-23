function initSectionTracking() {
  const links = [...document.querySelectorAll('.site-nav a[href^="#"]')];
  if (!links.length || !("IntersectionObserver" in window)) return;

  const sections = links
    .map((link) => document.querySelector(link.getAttribute("href")))
    .filter(Boolean);

  const observer = new IntersectionObserver((entries) => {
    const visible = entries
      .filter((entry) => entry.isIntersecting)
      .sort((a, b) => b.intersectionRatio - a.intersectionRatio)[0];

    if (!visible) return;
    document.body.dataset.activeSection = visible.target.id;
    links.forEach((link) => {
      if (link.getAttribute("href") === `#${visible.target.id}`) link.setAttribute("aria-current", "page");
      else link.removeAttribute("aria-current");
    });
  }, { rootMargin: "-25% 0px -55%", threshold: [0, 0.1, 0.3] });

  sections.forEach((section) => observer.observe(section));
}

initSectionTracking();

if (new URLSearchParams(window.location.search).get("contact") === "sent") {
  const status = document.querySelector("#contact-status");
  if (status) status.textContent = "Thanks. Your message has been sent.";
}
