import test from 'node:test';
import assert from 'node:assert/strict';
import { SnapshotGate, canDownload, totals, doneCount, namingDetail } from '../src/state.js';

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
  assert.equal(namingDetail({ source: 'deterministic', reason: 'image_only' }), 'Standard filename (image only)');
  assert.equal(namingDetail(null), '');
});
