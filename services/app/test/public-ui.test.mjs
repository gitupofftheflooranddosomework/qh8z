import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const publicDir = path.join(root, 'public');
const appHtml = fs.readFileSync(path.join(publicDir, 'app.html'), 'utf8');
const appJs = fs.readFileSync(path.join(publicDir, 'app.js'), 'utf8');
const indexHtml = fs.readFileSync(path.join(publicDir, 'index.html'), 'utf8');

function idsIn(html) {
  return [...html.matchAll(/\sid=["']([^"']+)["']/g)].map(match => match[1]);
}

function localAssets(html) {
  return [...html.matchAll(/(?:src|href)=["']\/assets\/([^"'?#]+)[^"']*["']/g)].map(match => match[1]);
}

test('dashboard HTML has no duplicate IDs', () => {
  const ids = idsIn(appHtml);
  const duplicates = ids.filter((id, index) => ids.indexOf(id) !== index);
  assert.deepEqual([...new Set(duplicates)], []);
});

test('every direct dashboard selector references a real element ID', () => {
  const ids = new Set(idsIn(appHtml));
  const selectors = [...appJs.matchAll(/\$\(['"]#([^'"]+)['"]\)/g)].map(match => match[1]);
  const missing = [...new Set(selectors.filter(id => !ids.has(id)))];
  assert.deepEqual(missing, []);
});

test('dialog close targets exist', () => {
  const ids = new Set(idsIn(appHtml));
  const targets = [...appHtml.matchAll(/data-close-dialog=["']([^"']+)["']/g)].map(match => match[1]);
  assert.deepEqual([...new Set(targets.filter(id => !ids.has(id)))], []);
});

test('landing and dashboard local assets exist', () => {
  const assets = new Set([...localAssets(appHtml), ...localAssets(indexHtml)]);
  for (const asset of assets) {
    assert.equal(fs.existsSync(path.join(publicDir, asset)), true, `missing /assets/${asset}`);
  }
});

test('dashboard exposes the launch-v1 product sections', () => {
  for (const id of ['linksSection', 'billingPanel', 'developerPanel', 'accountPanel', 'adminPanel', 'bulkDialog', 'detailDialog', 'apiTokenDialog']) {
    assert.match(appHtml, new RegExp(`id=["']${id}["']`));
  }
});
