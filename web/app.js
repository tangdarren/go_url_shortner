const form = document.querySelector('#shorten-form');
const input = document.querySelector('#url');
const submit = document.querySelector('#shorten-button');
const status = document.querySelector('#form-status');
const result = document.querySelector('#result');
const shortLink = document.querySelector('#short-link');
const copy = document.querySelector('#copy-button');
const list = document.querySelector('#links');
const listStatus = document.querySelector('#list-status');
const refresh = document.querySelector('#refresh-button');

async function loadLinks() {
  refresh.disabled = true;
  listStatus.textContent = 'Loading recent links…';
  try {
    const response = await fetch('/api/links');
    if (!response.ok) throw new Error('Could not load recent links. Use Refresh to try again.');
    const links = await response.json();
    list.replaceChildren();
    for (const link of links) {
      const row = document.createElement('li');
      row.className = 'link-row';
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
    }
    listStatus.textContent = links.length ? '' : 'No links yet. Your first short link starts above.';
  } catch (error) {
    listStatus.textContent = error.message;
  } finally {
    refresh.disabled = false;
  }
}

form.addEventListener('submit', async (event) => {
  event.preventDefault();
  submit.disabled = true;
  submit.textContent = 'Shortening…';
  status.className = 'status';
  status.textContent = 'Creating your short link…';
  input.removeAttribute('aria-invalid');
  result.hidden = true;
  try {
    const response = await fetch('/api/shorten', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ url: input.value.trim() }),
    });
    const data = await response.json();
    if (!response.ok) {
      if (response.status === 400) input.setAttribute('aria-invalid', 'true');
      throw new Error(data.error || 'Could not create your link. Please try again.');
    }
    shortLink.href = data.short_url;
    shortLink.textContent = data.short_url;
    copy.textContent = 'Copy link';
    result.hidden = false;
    status.textContent = 'Success! Your short link is ready to share.';
    await loadLinks();
  } catch (error) {
    status.className = 'status error';
    status.textContent = error instanceof TypeError ? 'Could not reach the server. Please try again.' : error.message;
  } finally {
    submit.disabled = false;
    submit.textContent = 'Shorten ↗';
  }
});

copy.addEventListener('click', async () => {
  try {
    await navigator.clipboard.writeText(shortLink.textContent);
    copy.textContent = 'Copied!';
    status.textContent = 'Link copied to clipboard.';
  } catch {
    const selection = window.getSelection();
    const range = document.createRange();
    range.selectNodeContents(shortLink);
    selection.removeAllRanges();
    selection.addRange(range);
    status.textContent = 'Automatic copy is unavailable. The link is selected; use your device’s Copy command.';
  }
});
refresh.addEventListener('click', loadLinks);
window.addEventListener('pageshow', loadLinks);
