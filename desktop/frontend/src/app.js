import { SnapshotGate, canDownload, totals, doneCount, namingDetail } from './state.js';

const $ = id => document.getElementById(id);
const api = window.go.main.Desktop;
const gate = new SnapshotGate();
api.BuildInfo().then(info => {
  $('about-version').textContent = `About: ${info.version} · source ${info.commit}${info.release ? '' : ' · development, unsigned'}`;
}).catch(() => { $('about-version').textContent = 'Version unavailable'; });
let state = null, pending = 0, queue = Promise.resolve(), settingsDirty = false;
let documentKey = '', resultKey = '';
const namingRecipients = {
  openai: ['OpenAI', 'https://api.openai.com/v1/responses'],
  xai: ['xAI', 'https://api.x.ai/v1/chat/completions'],
  anthropic: ['Anthropic', 'https://api.anthropic.com/v1/messages'],
  google: ['Google', 'https://generativelanguage.googleapis.com/v1beta/models/{model}:generateContent'],
};

function updateNamingDisclosure() {
  const recipient = namingRecipients[$('naming-provider').value];
  $('naming-disclosure').textContent = recipient
    ? `Direct recipient: ${recipient[0]} at ${recipient[1]}. No intermediary is configured. A different recipient requires fresh consent.`
    : 'Choose a provider to review the direct recipient before consenting.';
}

function showError(message) {
  $('error').textContent = String(message);
  $('error').hidden = !message;
}

async function refresh() {
  const epoch = gate.epoch;
  const s = await api.Snapshot();
  if (!gate.accept(s, epoch)) return;
  state = s;
  render();
}

function act(action) {
  gate.invalidate();
  pending++;
  renderControls();
  queue = queue.then(async () => {
    try { showError(''); await action(); }
    catch (err) { showError(err || 'Desktop connection failed. Restart the application.'); }
    finally { pending--; if (!pending) await refresh(); else renderControls(); }
  }).catch(() => { showError('Desktop connection failed. Restart the application.'); });
  return queue;
}

function renderControls() {
  const busy = !state || state.busy || state.closing || pending > 0;
  $('preview').disabled = busy || !$('case-number').value.trim();
  $('download').disabled = busy || !canDownload(state);
  $('cancel').disabled = !state?.busy || state.canceling || state.closing;
  for (const id of ['verified', 'select-all', 'clear', 'save-settings', 'choose-folder']) $(id).disabled = busy;
  const savedProvider = state?.preferences.namingProvider;
  const providerIsSaved = !!savedProvider && savedProvider === $('naming-provider').value;
  $('save-credential').disabled = busy || !providerIsSaved || !$('naming-key').value.trim();
  $('remove-credential').disabled = busy || !providerIsSaved || state?.namingCredentialStatus !== 'configured';
  for (const input of $('documents').querySelectorAll('input')) input.disabled = busy;
  $('open-folder').disabled = !state?.preferences.outputDirectory && !state?.result?.directory;
}

function cell(row, text) { const td = row.insertCell(); td.textContent = text; return td; }

function render() {
  if (!state) return;
  const s = state;
  $('status').dataset.phase = s.phase;
  $('status').textContent = s.closing ? 'Closing safely; waiting for browser and file cleanup…' : s.message || ({ idle: 'Enter a case number, then preview.', previewing: 'Loading the case preview…', downloading: 'Downloading selected documents…', ready: 'Preview ready. Check the header and select documents.' }[s.phase] || '');
  $('output-path').textContent = s.preferences.outputDirectory || 'No output folder chosen.';
  $('browser-info').textContent = `Browser: ${s.browser}`;
  $('diagnostic').textContent = s.diagnostic || (['error', 'partial', 'canceled'].includes(s.phase) ? s.message : '');
  if (!settingsDirty) {
    $('template').value = s.preferences.caseURLTemplate;
    $('browser').value = s.preferences.browserOverride;
    $('output-setting').value = s.preferences.outputDirectory;
    $('naming-enabled').checked = !!s.preferences.namingEnabled;
    $('naming-provider').value = s.preferences.namingProvider || '';
    $('naming-model').value = s.preferences.namingModel || '';
    $('naming-consent').checked = !!s.preferences.namingConsentRecipient && s.preferences.namingConsentRecipient === namingRecipients[s.preferences.namingProvider]?.[1];
  }
  updateNamingDisclosure();
  const credentialProvider = $('naming-provider').value;
  $('credential-status').textContent = credentialProvider !== s.preferences.namingProvider
    ? 'Save the selected provider before managing its key.'
    : !credentialProvider ? 'Choose and save a provider.'
    : s.namingCredentialStatus === 'configured' ? 'API key configured for this provider.'
    : s.namingCredentialStatus === 'unavailable' ? 'Windows secure credential storage is unavailable.'
    : 'No API key saved for this provider.';
  $('naming-notice').textContent = s.namingNotice || '';
  $('preview-area').hidden = !s.preview;
  if (s.preview) {
    const p = s.preview;
    $('header-title').textContent = `${p.number} · ${p.title || 'Title not provided'}`;
    $('case-details').textContent = `County: ${p.county || 'Not provided'} · Judge: ${p.judge || 'Not provided'}`;
    $('parties').textContent = p.parties.join(' · ');
    $('verified').checked = s.verified;
    $('selection-count').textContent = `${s.selected} of ${p.documents.length} selected`;
    $('empty').hidden = p.documents.length !== 0;
    const key = JSON.stringify([s.generation, p.documents.map(d => [d.id, d.date, d.description]), p.warnings]);
    if (key !== documentKey) {
      documentKey = key;
      $('warnings').replaceChildren(...p.warnings.map(message => { const el = document.createElement('p'); el.className = 'warning'; el.textContent = message; return el; }));
      $('documents').replaceChildren();
      for (const d of p.documents) {
        const row = $('documents').insertRow();
        const input = document.createElement('input'); input.type = 'checkbox'; input.checked = d.selected; input.dataset.id = d.id;
        input.setAttribute('aria-label', `Select ${d.date} ${d.description}`);
        input.addEventListener('change', () => selectFromDOM(false));
        cell(row, '').append(input); cell(row, d.date || 'Not provided'); cell(row, d.description || 'Document');
      }
    }
    for (const d of p.documents) {
      const input = $('documents').querySelector(`input[data-id="${d.id}"]`);
      if (input) input.checked = d.selected;
    }
  } else { documentKey = ''; $('documents').replaceChildren(); }
  $('progress-area').hidden = !s.busy;
  const c = s.progress;
  if (s.phase === 'downloading') { $('progress').max = Math.max(1, c.Selected); $('progress').value = doneCount(c); }
  else $('progress').removeAttribute('value');
  $('progress-text').textContent = `${doneCount(c)} / ${c.Selected} complete · ${totals(c)}${s.bytes ? ` · Current transfer: ${s.bytes.toLocaleString()} bytes` : ''}`;
  $('results').hidden = !s.result;
  if (s.result) {
    $('totals').textContent = totals(s.result.counts);
    $('result-path').textContent = s.result.directory || s.preferences.outputDirectory;
    const key = JSON.stringify(s.result);
    if (key !== resultKey) {
      resultKey = key; $('result-documents').replaceChildren();
      for (const d of s.result.documents) {
        const row = $('result-documents').insertRow();
        cell(row, d.outcome + (d.saved && d.outcome !== 'succeeded' ? ' (PDF saved)' : ''));
        cell(row, [d.description, d.filename].filter(Boolean).join(' — '));
        const ordinaryDetail = d.error || (d.skip_reason === 'verified_existing' ? 'Existing file verified' : d.skip_reason || 'Saved');
        cell(row, [ordinaryDetail, namingDetail(d.naming)].filter(Boolean).join(' · '));
      }
    }
  } else resultKey = '';
  renderControls();
}

function selectFromDOM(all) {
  const ids = [...$('documents').querySelectorAll('input:checked')].map(x => x.dataset.id);
  const generation = state.generation, verified = $('verified').checked;
  act(() => api.SetSelection(generation, ids, verified, all));
}

$('case-number').addEventListener('input', () => {
  const value = $('case-number').value;
  // Invalidate locally before the binding round trip can return an old preview.
  if (state) { state = { ...state, preview: null, result: null, selected: 0, verified: false }; render(); }
  act(() => api.SetCase(value));
});
$('case-form').addEventListener('submit', event => { event.preventDefault(); act(() => api.PreviewCase()); });
$('cancel').addEventListener('click', () => act(() => api.Cancel()));
$('verified').addEventListener('change', () => selectFromDOM(state.all));
$('select-all').addEventListener('click', () => { for (const input of $('documents').querySelectorAll('input')) input.checked = true; selectFromDOM(true); });
$('clear').addEventListener('click', () => { for (const input of $('documents').querySelectorAll('input')) input.checked = false; selectFromDOM(false); });
$('download').addEventListener('click', () => { const gen = state.generation; act(() => api.Download(gen)); });
$('choose-folder').addEventListener('click', () => act(async () => {
  const folder = await api.ChooseFolder();
  if (folder) $('output-setting').value = folder;
}));
$('open-folder').addEventListener('click', () => act(() => api.OpenOutputFolder()));
$('settings-toggle').addEventListener('click', () => { $('settings').hidden = !$('settings').hidden; $('settings-toggle').setAttribute('aria-expanded', String(!$('settings').hidden)); });
$('settings-form').addEventListener('input', () => { settingsDirty = true; });
$('naming-provider').addEventListener('change', () => { $('naming-consent').checked = false; $('naming-key').value = ''; settingsDirty = true; updateNamingDisclosure(); renderControls(); });
$('naming-key').addEventListener('input', renderControls);
$('settings-form').addEventListener('submit', event => {
  event.preventDefault();
  const provider = $('naming-provider').value;
  const p = { caseURLTemplate: $('template').value, browserOverride: $('browser').value, outputDirectory: $('output-setting').value,
    namingEnabled: $('naming-enabled').checked, namingProvider: provider, namingModel: $('naming-model').value,
    namingConsentRecipient: $('naming-consent').checked ? namingRecipients[provider]?.[1] || '' : '' };
  act(async () => { await api.SavePreferences(p); settingsDirty = false; });
});
$('credential-form').addEventListener('submit', event => {
  event.preventDefault();
  const provider = $('naming-provider').value;
  const key = $('naming-key').value;
  $('naming-key').value = '';
  act(() => api.SaveNamingCredential(provider, key));
});
$('remove-credential').addEventListener('click', () => act(() => api.RemoveNamingCredential($('naming-provider').value)));

try { await refresh(); if (!state.preferences.caseURLTemplate) $('settings-toggle').click(); }
catch { showError('Desktop connection failed. Restart the application.'); }
// Polling an authoritative snapshot avoids depending on delivery of terminal
// progress events. Only one poll is outstanding, and actions invalidate it.
async function poll() {
  try { if (!pending) await refresh(); }
  catch { showError('Desktop connection failed. Restart the application.'); }
  setTimeout(poll, 200);
}
setTimeout(poll, 200);
