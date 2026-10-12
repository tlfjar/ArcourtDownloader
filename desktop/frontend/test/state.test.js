import test from 'node:test';
import assert from 'node:assert/strict';
import { SnapshotGate, canDownload, totals, doneCount, namingDetail, namingQualityNote } from '../src/state.js';

test('late preview/poll cannot restore a previous case after typing', () => {
  const gate = new SnapshotGate();
  const old = gate.epoch;
  gate.invalidate();
  assert.equal(gate.accept({ revision: 12 }, old), false);
  assert.equal(gate.accept({ revision: 13 }, gate.epoch), true);
  assert.equal(gate.accept({ revision: 12 }, gate.epoch), false);
});

test('empty selection, unverified or changed case, cancellation and close disable download', () => {
  const s = { preview: {}, verified: true, selected: 2, preferences: { outputDirectory: 'C:\\PDF output' } };
  assert.equal(canDownload(s), true);
  for (const patch of [{ selected: 0 }, { verified: false }, { preview: null }, { busy: true, canceling: true }, { closing: true }, { preferences: {} }]) {
    assert.equal(canDownload({ ...s, ...patch }), false);
  }
});

test('display preserves separate service outcomes, including partial results', () => {
  const c = { Selected: 5, Succeeded: 1, Skipped: 1, Failed: 1, Unavailable: 1, Canceled: 1 };
  assert.equal(doneCount(c), 5);
  assert.equal(totals(c), 'Saved 1 · Skipped 1 · Failed 1 · Unavailable 1 · Canceled 1');
});

test('naming detail distinguishes AI labels and ordinary fallback', () => {
  assert.equal(namingDetail({ source: 'ai', label: 'Motion to Dismiss' }), 'AI label: Motion to Dismiss');
  assert.equal(namingDetail({ source: 'deterministic', reason: 'image_only' }), 'Standard filename (scanned or image-only PDF)');
  assert.equal(namingDetail({ source: 'deterministic', reason: 'unsupported_result' }), 'Standard filename (model label did not match the title)');
  assert.equal(namingDetail({ source: 'deterministic', reason: 'unexpected' }), 'Standard filename (naming unavailable)');
  assert.equal(namingDetail({ source: 'deterministic', reason: 'constructor' }), 'Standard filename (naming unavailable)');
  assert.equal(namingDetail(null), '');
});

test('only the evaluated provider and model are presented as measured', () => {
  assert.match(namingQualityNote('openai', 'gpt-6-luna'), /^Naming quality for gpt-6-luna was measured/);
  assert.match(namingQualityNote('openai', '  gpt-6-luna  '), /^Naming quality for gpt-6-luna was measured/);
  assert.match(namingQualityNote('openai', 'gpt-4.1-mini'), /^Experimental: naming quality for this model is unmeasured/);
  for (const provider of ['xai', 'anthropic', 'google']) {
    assert.match(namingQualityNote(provider, 'any-model'), /^Experimental: naming quality for this provider is unmeasured/);
  }
  assert.match(namingQualityNote('openai', ''), /^Experimental/);
  assert.match(namingQualityNote('', ''), /^Experimental/);
});