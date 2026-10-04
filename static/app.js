const journey = document.querySelector(".journey-stage");
const chapters = [...document.querySelectorAll("main .chapter")];
const foregrounds = chapters.map(chapter => [...chapter.querySelectorAll(".chapter-inner, .side-note, .section-footnote, .outline-word")]);
const progress = document.querySelector(".chapter-progress");
const progressLabel = document.querySelector(".progress-label");
const progressFill = document.querySelector(".progress-fill");
const progressHint = progress.lastElementChild;
const navLinks = [...document.querySelectorAll(".site-nav a")];
const sceneMode = window.matchMedia("(min-width: 1051px) and (min-height: 768px) and (prefers-reduced-motion: no-preference)");
const motionMode = window.matchMedia("(prefers-reduced-motion: no-preference)");
const heroVideo = document.querySelector(".hero-video");
const heroVideoPlay = document.querySelector(".hero-video-play");
const clamp = value => Math.min(1, Math.max(0, value));
let scheduled = false;
let heroVideoPlayPending = false;
let heroVideoBlocked = false;

function heroIsVisible() {
  const rect = heroVideo.getBoundingClientRect();
  return rect.bottom > 0 && rect.top < window.innerHeight;
}

function startHeroVideo() {
  if (heroVideoPlayPending || !heroVideo.paused) return;
  heroVideoPlayPending = true;
  heroVideo.muted = true;
  const play = heroVideo.play();
  if (!play) {
    heroVideoPlayPending = false;
    return;
  }
  play.then(() => {
    heroVideoPlayPending = false;
    heroVideoBlocked = false;
    heroVideoPlay.hidden = true;
  }).catch(error => {
    heroVideoPlayPending = false;
    if (error.name === "AbortError" || error.name === "NotSupportedError") return;
    if (motionMode.matches && !document.hidden && heroIsVisible()) {
      heroVideoBlocked = true;
      heroVideoPlay.hidden = false;
    }
  });
}

// The coffee site's scenes advance in roughly 40vh of scrolling each.
journey.style.setProperty("--journey-height", `${100 + Math.max(0, chapters.length - 1) * 40}svh`);

function scenePosition() {
  const range = Math.max(1, journey.offsetHeight - window.innerHeight);
  const progress = clamp(-journey.getBoundingClientRect().top / range);
  return { progress, position: progress * (chapters.length - 1) };
}

function updatePagePosition() {
  scheduled = false;
  if (!chapters.length) return;

  const immersive = sceneMode.matches;
  document.documentElement.classList.toggle("has-chapter-scenes", immersive);
  document.documentElement.classList.toggle("has-scroll-fade", !immersive && motionMode.matches);
  let activeIndex = 0;
  let pageProgress;

  if (immersive) {
    const { progress: journeyProgress, position } = scenePosition();
    activeIndex = Math.min(chapters.length - 1, Math.round(position));
    pageProgress = journeyProgress;

    chapters.forEach((chapter, index) => {
      // Each full-screen scene crossfades in place, like the coffee site.
      const distance = Math.abs(position - index);
      const opacity = clamp((0.7 - distance) / 0.4);
      const foregroundOpacity = clamp((0.45 - distance) / 0.25);
      chapter.style.opacity = opacity.toFixed(3);
      chapter.querySelector(".chapter-inner").style.transform = `translate3d(0, ${Math.round((index - position) * 30)}px, 0)`;
      foregrounds[index].forEach(element => { element.style.opacity = foregroundOpacity.toFixed(3); });
      chapter.inert = index !== activeIndex;
      chapter.style.pointerEvents = index === activeIndex ? "auto" : "none";
    });
  } else {
    const marker = window.innerHeight * 0.46;
    chapters.forEach((chapter, index) => {
      const rect = chapter.getBoundingClientRect();
      if (rect.top <= marker) activeIndex = index;
      chapter.style.removeProperty("opacity");
      chapter.style.removeProperty("pointer-events");
      foregrounds[index].forEach(element => { element.style.removeProperty("opacity"); });
      const inner = chapter.querySelector(".chapter-inner");
      if (motionMode.matches) {
        const fadeDistance = Math.min(240, window.innerHeight * 0.3);
        const fixedBottom = document.querySelector(".offer-reel").getBoundingClientRect().bottom;
        const entering = clamp((window.innerHeight - rect.top) / fadeDistance);
        const leaving = clamp((rect.bottom - fixedBottom) / fadeDistance);
        const visibility = chapter.classList.contains("is-direct-target") || chapter.matches(":focus-within") ? 1 : Math.min(entering, leaving);
        inner.style.opacity = visibility.toFixed(3);
        inner.style.transform = `translate3d(0, ${Math.round((1 - visibility) * 26)}px, 0)`;
      } else {
        inner.style.removeProperty("opacity");
        inner.style.removeProperty("transform");
      }
      chapter.inert = false;
    });
    const scrollable = Math.max(1, document.documentElement.scrollHeight - window.innerHeight);
    pageProgress = clamp(window.scrollY / scrollable);
  }

  const current = chapters[activeIndex];
  if (heroVideo) {
    if (motionMode.matches && !document.hidden && heroIsVisible()) {
      if (heroVideoBlocked) heroVideoPlay.hidden = false;
      else startHeroVideo();
    } else {
      heroVideoPlay.hidden = true;
      if (!heroVideo.paused) heroVideo.pause();
    }
  }
  progressLabel.textContent = `${String(activeIndex + 1).padStart(2, "0")} / ${String(chapters.length).padStart(2, "0")}`;
  progressFill.style.width = `${(pageProgress * 100).toFixed(2)}%`;
  progressHint.hidden = activeIndex === chapters.length - 1;
  progress.classList.toggle("is-light", current.matches(".chapter-contact"));
  navLinks.forEach(link => {
    if (link.hash === `#${current.id}`) link.setAttribute("aria-current", "location");
    else link.removeAttribute("aria-current");
  });
}

function scheduleUpdate() {
  if (scheduled) return;
  scheduled = true;
  requestAnimationFrame(updatePagePosition);
}

function scrollToChapter(index, behavior = "smooth") {
  if (!sceneMode.matches) {
    chapters[index].scrollIntoView({ behavior });
    return;
  }
  const range = Math.max(0, journey.offsetHeight - window.innerHeight);
  const top = window.scrollY + journey.getBoundingClientRect().top + range * index / Math.max(1, chapters.length - 1);
  window.scrollTo({ top, behavior });
}

window.addEventListener("scroll", scheduleUpdate, { passive: true });
window.addEventListener("resize", scheduleUpdate);
sceneMode.addEventListener("change", scheduleUpdate);
motionMode.addEventListener("change", scheduleUpdate);
document.addEventListener("visibilitychange", scheduleUpdate);
if (heroVideo) {
  heroVideo.addEventListener("canplay", scheduleUpdate);
  heroVideo.addEventListener("playing", () => {
    heroVideoBlocked = false;
    heroVideoPlay.hidden = true;
  });
  heroVideoPlay.addEventListener("click", () => {
    heroVideoBlocked = false;
    heroVideoPlay.hidden = true;
    startHeroVideo();
  });
}

document.querySelectorAll('a[href^="#"]').forEach(link => {
  const index = chapters.findIndex(chapter => `#${chapter.id}` === link.getAttribute("href"));
  if (index < 0) return;
  link.addEventListener("click", event => {
    const directOffer = link.matches(".offer-reel, .deal-cta");
    if (!sceneMode.matches && !directOffer) return;
    event.preventDefault();
    history.pushState(null, "", `#${chapters[index].id}`);
    if (directOffer) {
      chapters[index].classList.add("is-direct-target");
      const inner = chapters[index].querySelector(".chapter-inner");
      const previousTransition = inner.style.transition;
      inner.style.transition = "none";
      inner.style.opacity = "1";
      inner.style.transform = "none";
      const root = document.documentElement;
      const previousBehavior = root.style.scrollBehavior;
      root.style.scrollBehavior = "auto";
      if (sceneMode.matches) scrollToChapter(index, "auto");
      else {
        const form = document.querySelector(".contact-form");
        const fixedBottom = document.querySelector(".offer-reel").getBoundingClientRect().bottom;
        const offset = fixedBottom + 16;
        window.scrollTo({ top: window.scrollY + form.getBoundingClientRect().top - offset, behavior: "auto" });
      }
      requestAnimationFrame(() => {
        root.style.scrollBehavior = previousBehavior;
        inner.style.transition = previousTransition;
      });
      setTimeout(() => {
        chapters[index].classList.remove("is-direct-target");
        scheduleUpdate();
      }, 900);
    } else {
      scrollToChapter(index);
    }
  });
});

window.addEventListener("popstate", () => {
  const index = chapters.findIndex(chapter => `#${chapter.id}` === window.location.hash);
  if (sceneMode.matches) scrollToChapter(Math.max(0, index), "auto");
});

scheduleUpdate();

if (new URLSearchParams(window.location.search).get("contact") === "sent") {
  const status = document.querySelector("#contact-status");
  if (status) status.textContent = "Thanks. Your message has been sent.";
  if (!window.location.hash) requestAnimationFrame(() => scrollToChapter(chapters.length - 1, "auto"));
} else if (window.location.hash && sceneMode.matches) {
  const index = chapters.findIndex(chapter => `#${chapter.id}` === window.location.hash);
  if (index >= 0) requestAnimationFrame(() => scrollToChapter(index, "auto"));
}
