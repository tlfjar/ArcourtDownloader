// UI actions are serialized. A locally edited case immediately invalidates any
// pending snapshot; Go independently checks the preview generation on commands.
export class SnapshotGate {
  epoch = 0;
  revision = -1;
  invalidate() { return ++this.epoch; }
  accept(state, epoch) {
    if (epoch !== this.epoch || state.revision < this.revision) return false;
    this.revision = state.revision;
    return true;
  }
}

export function canDownload(s) {
  return !!s && !s.busy && !s.closing && !!s.preview && s.verified && s.selected > 0 && !!s.preferences.outputDirectory;
}

export function totals(c) {
  return `Saved ${c.Succeeded} · Skipped ${c.Skipped} · Failed ${c.Failed} · Unavailable ${c.Unavailable} · Canceled ${c.Canceled}`;
}

export function doneCount(c) { return c.Succeeded + c.Skipped + c.Failed + c.Unavailable + c.Canceled; }

const namingFallbackReasons = {
  image_only: 'scanned or image-only PDF',
  too_large: 'PDF too large for naming',
  encrypted: 'encrypted PDF',
  unreadable: 'no usable text',
  insufficient_context: 'title unclear',
  unsafe_result: 'model label could not be used',
  unsupported_result: 'model label did not match the title',
  length_exhaustion: 'label too long',
  configuration: 'naming setup incomplete',
  provider_auth: 'provider rejected the API key',
  provider_model: 'model unavailable',
  rate_limited: 'provider rate limit',
  provider_refusal: 'provider declined naming',
  provider_error: 'provider error',
  payload_limit: 'request too large',
  canceled: 'naming canceled',
  timeout: 'naming timed out',
};

export function namingDetail(n) {
  if (!n) return '';
  if (n.source === 'ai' && n.label) return `AI label: ${n.label}`;
  if (n.source === 'deterministic') {
    const detail = Object.hasOwn(namingFallbackReasons, n.reason) ? namingFallbackReasons[n.reason] : 'naming unavailable';
    return n.reason ? `Standard filename (${detail})` : 'Standard filename';
  }
  return 'Standard filename';
}

// Only provider/model combinations with a completed held-out naming-quality
// evaluation are presented as measured. Everything else is labeled
// experimental so an operator is not led to assume untested naming is proven.
export const namingEvaluatedModels = { openai: ['gpt-6-luna'] };

export function namingQualityNote(provider, model) {
  const evaluated = namingEvaluatedModels[provider] || [];
  if (evaluated.includes(String(model ?? '').trim())) {
    return `Naming quality for ${String(model).trim()} was measured against held-out synthetic filings and met this project's targets. Real court documents were not tested.`;
  }
  return evaluated.length
    ? `Experimental: naming quality for this model is unmeasured. Only ${evaluated.join(', ')} has been evaluated on this provider. An unusable label keeps the standard filename.`
    : 'Experimental: naming quality for this provider is unmeasured. Requests and responses are tested, but no live naming evaluation has run. An unusable label keeps the standard filename.';
}
