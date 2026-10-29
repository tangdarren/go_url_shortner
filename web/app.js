const form = document.querySelector('#shorten-form');
const input = document.querySelector('#url');
const submit = document.querySelector('#shorten-button');
const status = document.querySelector('#form-status');
const result = document.querySelector('#result');
const shortLink = document.querySelector('#short-link');
const copy = document.querySelector('#copy-button');
const copyLabel = document.querySelector('.copy-label');
const list = document.querySelector('#links');
const listStatus = document.querySelector('#list-status');
const refresh = document.querySelector('#refresh-button');

const submitLabel = submit.querySelector('.btn-label');

function setStatus(message, kind = '') {
  status.className = kind ? `status ${kind}` : 'status';
  status.textContent = message;
}

function isValidHttpUrl(value) {
  try {
    const parsed = new URL(value);
    return parsed.protocol === 'http:' || parsed.protocol === 'https:';
  } catch {
    return false;
  }
}

function validateInput() {
  const value = input.value.trim();
  if (!value) {
    input.removeAttribute('aria-invalid');
    return true;
  }
  const ok = isValidHttpUrl(value);
  input.setAttribute('aria-invalid', ok ? 'false' : 'true');
  return ok;
}

async function loadLinks() {
  refresh.disabled = true;
  listStatus.textContent = 'Loading recent links…';
  try {
    const response = await fetch('/api/links');
    if (!response.ok) throw new Error('Could not load recent links. Use Refresh to try again.');
    const links = await response.json();
    list.replaceChildren();
    links.forEach((link, index) => {
      const row = document.createElement('li');
      row.className = 'link-row';
      row.style.animationDelay = `${Math.min(index, 12) * 40}ms`;
      const details = document.createElement('div');
      details.className = 'link-details';
      const anchor = document.createElement('a');
      anchor.href = link.short_url;
      anchor.textContent = link.short_url;
      const original = document.createElement('span');
      original.className = 'original';
      original.textContent = link.original_url;
      const clicks = document.createElement('span');
      clicks.className = 'clicks';
      clicks.textContent = `${link.clicks} ${link.clicks === 1 ? 'click' : 'clicks'}`;
      details.append(anchor, original);
      row.append(details, clicks);
      list.append(row);
    });
    listStatus.textContent = links.length ? '' : 'No links yet. Your first short link starts above.';
  } catch (error) {
    listStatus.textContent = error.message;
  } finally {
    refresh.disabled = false;
  }
}

input.addEventListener('focus', () => {
  // Keep the field above the on-screen keyboard / browser chrome.
  requestAnimationFrame(() => {
    input.scrollIntoView({ block: 'center', inline: 'nearest', behavior: 'smooth' });
  });
});

input.addEventListener('blur', () => {
  if (!input.value.trim()) {
    input.removeAttribute('aria-invalid');
    return;
  }
  if (!validateInput()) {
    setStatus('Enter a full URL starting with http:// or https://', 'error');
  } else if (status.classList.contains('error')) {
    setStatus('');
  }
});

input.addEventListener('input', () => {
  if (input.getAttribute('aria-invalid') === 'true' && isValidHttpUrl(input.value.trim())) {
    input.setAttribute('aria-invalid', 'false');
    if (status.classList.contains('error')) setStatus('');
  }
});

form.addEventListener('submit', async (event) => {
  event.preventDefault();
  const value = input.value.trim();
  if (!isValidHttpUrl(value)) {
    input.setAttribute('aria-invalid', 'true');
    setStatus('Enter a full URL starting with http:// or https://', 'error');
    input.focus();
    return;
  }

  submit.disabled = true;
  submitLabel.textContent = 'Shortening…';
  setStatus('Creating your short link…');
  input.removeAttribute('aria-invalid');
  result.hidden = true;
  copy.classList.remove('copied');
  copyLabel.textContent = 'Copy link';

  try {
    const response = await fetch('/api/shorten', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ url: value }),
    });
    const data = await response.json();
    if (!response.ok) {
      if (response.status === 400) input.setAttribute('aria-invalid', 'true');
      throw new Error(data.error || 'Could not create your link. Please try again.');
    }
    shortLink.href = data.short_url;
    shortLink.textContent = data.short_url;
    result.hidden = false;
    setStatus('Success! Your short link is ready to share.', 'success');
    await loadLinks();
  } catch (error) {
    setStatus(
      error instanceof TypeError ? 'Could not reach the server. Please try again.' : error.message,
      'error',
    );
  } finally {
    submit.disabled = false;
    submitLabel.textContent = 'Shorten';
  }
});

copy.addEventListener('click', async () => {
  try {
    await navigator.clipboard.writeText(shortLink.textContent);
    copyLabel.textContent = 'Copied!';
    copy.classList.add('copied');
    setStatus('Link copied to clipboard.', 'success');
  } catch {
    const selection = window.getSelection();
    const range = document.createRange();
    range.selectNodeContents(shortLink);
    selection.removeAllRanges();
    selection.addRange(range);
    setStatus('Automatic copy is unavailable. The link is selected; use your device’s Copy command.');
  }
});

refresh.addEventListener('click', loadLinks);
window.addEventListener('pageshow', loadLinks);
