import { Pipe, PipeTransform } from '@angular/core';

@Pipe({
  name: 'isIp',
  standalone: true
})
export class IsIpPipe implements PipeTransform {
  private readonly ipv4Regex = /^(?:(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.){3}(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)$/;
  private readonly ipv6Regex = /^(([0-9a-fA-F]{1,4}:){7}[0-9a-fA-F]{1,4}|([0-9a-fA-F]{1,4}:){1,7}:|([0-9a-fA-F]{1,4}:){1,6}:[0-9a-fA-F]{1,4}|([0-9a-fA-F]{1,4}:){1,5}(:[0-9a-fA-F]{1,4}){1,2}|([0-9a-fA-F]{1,4}:){1,4}(:[0-9a-fA-F]{1,4}){1,3}|([0-9a-fA-F]{1,4}:){1,3}(:[0-9a-fA-F]{1,4}){1,4}|([0-9a-fA-F]{1,4}:){1,2}(:[0-9a-fA-F]{1,4}){1,5}|[0-9a-fA-F]{1,4}:((:[0-9a-fA-F]{1,4}){1,6})|:((:[0-9a-fA-F]{1,4}){1,7}|:))$/;

  /** Acepta direcciones IPv4/IPv6 válidas salvo los rangos locales excluidos por este pipe. */
  transform(value: string | null | undefined): boolean {
    if (!value) return false;

    // 1. Validar formato básico de IP
    const isV4 = this.ipv4Regex.test(value);
    const isV6 = this.ipv6Regex.test(value);

    if (!isV4 && !isV6) return false;

    // 2. Si es IPv4, excluir rangos privados y reservados
    if (isV4) {
      if (this.isPrivateIPv4(value)) {
        return false;
      }
    }

    // 3. Si es IPv6, excluir localhost y redes locales/privadas
    if (isV6) {
      if (this.isPrivateIPv6(value)) {
        return false;
      }
    }

    return true;
  }

  /** Detecta redes privadas, loopback, enlace local y la dirección 0.0.0.0 en una IPv4 válida. */
  private isPrivateIPv4(ip: string): boolean {
    const parts = ip.split('.').map(Number);
    const [a, b] = parts;

    // 10.0.0.0/8
    if (a === 10) return true;
    // 172.16.0.0/12 (172.16.0.0 - 172.31.255.255)
    if (a === 172 && b >= 16 && b <= 31) return true;
    // 192.168.0.0/16
    if (a === 192 && b === 168) return true;
    // 127.0.0.0/8 (Loopback)
    if (a === 127) return true;
    // 169.254.0.0/16 (Link-local / APIPA)
    if (a === 169 && b === 254) return true;
    // 0.0.0.0 (Red por defecto)
    if (ip === '0.0.0.0') return true;

    return false;
  }

  /** Detecta loopback, direcciones de enlace local y direcciones locales únicas en una IPv6 válida. */
  private isPrivateIPv6(ip: string): boolean {
    const lowerIp = ip.toLowerCase();

    // ::1 (Loopback)
    if (lowerIp === '::1' || lowerIp === '0:0:0:0:0:0:0:1') return true;
    // fe80::/10 (Link-local)
    if (lowerIp.startsWith('fe8') || lowerIp.startsWith('fe9') || lowerIp.startsWith('fea') || lowerIp.startsWith('feb')) return true;
    // fc00::/7 (Unique local address - ULA)
    if (lowerIp.startsWith('fc') || lowerIp.startsWith('fd')) return true;

    return false;
  }
}
