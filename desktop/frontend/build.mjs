import { mkdir, copyFile, rm } from 'node:fs/promises';
// No bundler or third-party runtime: embed exactly the reviewed local assets.
// Stale files are otherwise picked up by go:embed all:frontend/dist.
await rm(new URL('dist/', import.meta.url), { recursive: true, force: true });
await mkdir(new URL('dist/', import.meta.url), { recursive: true });
for (const name of ['index.html', 'style.css', 'app.js', 'state.js']) {
  await copyFile(new URL(`src/${name}`, import.meta.url), new URL(`dist/${name}`, import.meta.url));
}
