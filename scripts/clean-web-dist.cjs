const fs = require('node:fs');
const path = require('node:path');

const dist = path.resolve(__dirname, '../internal/web/dist');
if (!fs.lstatSync(dist).isDirectory()) {
  throw new Error('internal/web/dist debe ser un directorio, no un enlace simbólico.');
}

for (const entry of fs.readdirSync(dist)) {
  if (entry !== '.gitkeep') {
    fs.rmSync(path.join(dist, entry), { recursive: true, force: false });
  }
}

fs.writeFileSync(path.join(dist, '.gitkeep'), '');
console.log('Frontend generado eliminado; internal/web/dist/.gitkeep conservado.');
