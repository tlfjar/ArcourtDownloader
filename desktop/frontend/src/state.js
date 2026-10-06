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
