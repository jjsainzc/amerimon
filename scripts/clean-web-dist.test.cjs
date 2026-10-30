const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const { spawnSync } = require('node:child_process');
const { test } = require('node:test');

function fixture(t) {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'amerimon-clean-dist-'));
  t.after(() => fs.rmSync(root, { recursive: true, force: true }));
  fs.mkdirSync(path.join(root, 'scripts'));
  fs.copyFileSync(__dirname + '/clean-web-dist.cjs', path.join(root, 'scripts/clean-web-dist.cjs'));
  fs.mkdirSync(path.join(root, 'internal/web'), { recursive: true });
  return {
    root,
    dist: path.join(root, 'internal/web/dist'),
    run: () => spawnSync(process.execPath, [path.join(root, 'scripts/clean-web-dist.cjs')], {
      cwd: os.tmpdir(),
      encoding: 'utf8',
    }),
  };
}

test('cleans generated files only, preserves .gitkeep and can run twice', (t) => {
  const { root, dist, run } = fixture(t);
  fs.mkdirSync(path.join(dist, 'assets'), { recursive: true });
  fs.writeFileSync(path.join(dist, 'index.html'), 'frontend');
  fs.writeFileSync(path.join(dist, 'chunk-old.js'), 'old');
  fs.writeFileSync(path.join(dist, 'assets/doc.txt'), 'asset');
  fs.writeFileSync(path.join(dist, '.gitkeep'), '');
  fs.mkdirSync(path.join(root, 'dist'));
  fs.writeFileSync(path.join(root, 'dist/amerimon.deb'), 'package');
  fs.writeFileSync(path.join(root, 'internal/web/web_test.go'), 'test');
  fs.symlinkSync(path.join(root, 'dist'), path.join(dist, 'external'));
  for (let i = 0; i < 2; i++) {
    const result = run();
    assert.equal(result.status, 0, result.stderr);
    assert.deepEqual(fs.readdirSync(dist), ['.gitkeep']);
    assert.equal(fs.readFileSync(path.join(root, 'dist/amerimon.deb'), 'utf8'), 'package');
    assert.equal(fs.readFileSync(path.join(root, 'internal/web/web_test.go'), 'utf8'), 'test');
  }
});

test('recreates the Go embed marker when the build did not leave it', (t) => {
  const { dist, run } = fixture(t);
  fs.mkdirSync(dist);
  fs.writeFileSync(path.join(dist, 'main.js'), 'frontend');
  const result = run();
  assert.equal(result.status, 0, result.stderr);
  assert.deepEqual(fs.readdirSync(dist), ['.gitkeep']);
});

test('refuses a symlink as the generated output directory', (t) => {
  const { root, dist, run } = fixture(t);
  const outside = path.join(root, 'outside');
  fs.mkdirSync(outside);
  fs.writeFileSync(path.join(outside, 'keep.txt'), 'keep');
  fs.symlinkSync(outside, dist);
  const result = run();
  assert.notEqual(result.status, 0);
  assert.equal(fs.readFileSync(path.join(outside, 'keep.txt'), 'utf8'), 'keep');
});

for (const status of [0, 1]) {
  test(`make release cleans the frontend only when packaging succeeds (exit ${status})`, (t) => {
    const { root, dist } = fixture(t);
    fs.copyFileSync(path.join(__dirname, '../Makefile'), path.join(root, 'Makefile'));
    fs.mkdirSync(dist);
    fs.writeFileSync(path.join(dist, 'main.js'), 'frontend');
    const bin = path.join(root, 'bin');
    fs.mkdirSync(bin);
    fs.writeFileSync(path.join(bin, 'goreleaser'), `#!/bin/sh\nexit ${status}\n`, { mode: 0o755 });
    const result = spawnSync('make', ['release'], {
      cwd: root,
      encoding: 'utf8',
      env: { ...process.env, PATH: bin + path.delimiter + process.env.PATH },
    });
    if (status === 0) {
      assert.equal(result.status, 0, result.stderr);
      assert.deepEqual(fs.readdirSync(dist), ['.gitkeep']);
    } else {
      assert.notEqual(result.status, 0);
      assert.equal(fs.readFileSync(path.join(dist, 'main.js'), 'utf8'), 'frontend');
    }
  });
}
