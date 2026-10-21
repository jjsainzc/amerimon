const fs = require('node:fs');
const path = require('node:path');

function prepareDebugEnv() {
  let user = process.env.AMI_USER?.trim();
  let secret = process.env.AMI_SECRET;

  if (!user || !secret) {
    const managerPath = '/etc/asterisk/manager.conf';
    const contents = fs.readFileSync(managerPath, 'utf8');
    const accounts = [];
    let account;
    for (const rawLine of contents.split(/\r?\n/)) {
      const line = rawLine.replace(/;.*$/, '').trim();
      if (!line || line.startsWith('#')) continue;
      const section = line.match(/^\[([^\]]+)\]\s*$/);
      if (section) {
        account = section[1] === 'general' ? undefined : { user: section[1].trim() };
        if (account) accounts.push(account);
      } else if (line.startsWith('[')) {
        account = undefined;
      } else if (account) {
        const setting = line.match(/^secret\s*=\s*(.*)$/i);
        if (setting) account.secret = setting[1].trim();
      }
    }
    const selected = accounts.find(account => account.secret && (!user || account.user === user));
    if (!selected) {
      throw new Error(`No se encontró una cuenta AMI con secret en ${managerPath}. Define AMI_USER y AMI_SECRET en el entorno de VS Code.`);
    }
    user = selected.user;
    secret = secret || selected.secret;
  }

  const encode = value => {
    if (/[\r\n]/.test(value)) {
      throw new Error('Las credenciales AMI no pueden contener saltos de línea.');
    }
    // El lector envFile de la extensión Go expande $; las comillas simples evitan convertir \n.
    return `'${value.replace(/\$/g, '\\$')}'`;
  };
  const contents = `AMI_USER=${encode(user)}\nAMI_SECRET=${encode(secret)}\n`;
  const destination = path.join(__dirname, '..', '.vscode', 'ami.env');
  fs.writeFileSync(destination, contents, { mode: 0o600 });
  fs.chmodSync(destination, 0o600);
  console.log('Credenciales AMI preparadas en .vscode/ami.env (solo lectura/escritura del propietario; excluido de Git).');
}

try {
  prepareDebugEnv();
} catch (error) {
  console.error(`No se pudo preparar AMI para depurar: ${error.message}`);
  process.exitCode = 1;
}
