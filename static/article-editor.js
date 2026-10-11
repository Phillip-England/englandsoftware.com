(() => {
  const form = document.querySelector('.article-editor');
  const frame = document.querySelector('iframe[name="article-preview"]');
  const status = document.querySelector('[data-preview-status]');
  const button = document.querySelector('[data-preview-button]');
  if (!form || !frame || !status || !button) return;

  let timer;
  let controller;
  let revision = 0;

  async function updatePreview() {
    const currentRevision = revision;
    controller?.abort();
    controller = new AbortController();
    status.textContent = 'Updating preview…';

    try {
      const response = await fetch('/admin/articles/preview', {
        method: 'POST',
        body: new FormData(form),
        credentials: 'same-origin',
        signal: controller.signal,
      });
      if (response.redirected) throw new Error('Your session has expired. Sign in again to preview.');
      if (!response.ok) throw new Error((await response.text()).trim() || 'Preview could not be updated.');
      const html = await response.text();
      if (currentRevision !== revision) return;
      frame.srcdoc = html;
      status.textContent = 'Preview is up to date. Changes are not saved.';
    } catch (error) {
      if (error.name !== 'AbortError' && currentRevision === revision) {
        status.textContent = error.message || 'Preview could not be updated.';
      }
    }
  }

  function schedulePreview() {
    revision++;
    clearTimeout(timer);
    controller?.abort();
    status.textContent = 'Preview will update when you pause.';
    timer = setTimeout(updatePreview, 800);
  }

  form.addEventListener('input', schedulePreview);
  form.addEventListener('change', schedulePreview);
  form.addEventListener('submit', () => {
    clearTimeout(timer);
    controller?.abort();
  });
  button.addEventListener('click', (event) => {
    event.preventDefault();
    clearTimeout(timer);
    revision++;
    updatePreview();
  });

  updatePreview();
})();
