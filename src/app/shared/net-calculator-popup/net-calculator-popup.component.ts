import { Component, input, output, signal } from '@angular/core';
import { CommonModule } from '@angular/common';
import { FormsModule } from '@angular/forms';

@Component({
  selector: 'app-net-calculator-popup',
  standalone: true,
  imports: [CommonModule, FormsModule],
  templateUrl: './net-calculator-popup.component.html',
  styleUrl: './net-calculator-popup.component.css'
})
export class NetCalculatorPopupComponent {
  isOpen = input<boolean>(false);

  closeModal = output<void>();
  selectNetwork = output<string>();

  ipInput = signal<string>('192.168.1.10');
  cidrInput = signal<number>(24);

  networkResult = signal<{
    network: string;
    broadcast: string;
    netmask: string;
    totalHosts: number;
    error?: string;
  } | null>(null);

  close() {
    this.closeModal.emit();
  }

  calculateSubnet() {
    try {
      const ip = this.ipInput().trim();
      const cidr = Number(this.cidrInput());

      if (cidr < 0 || cidr > 32) {
        throw new Error('El prefijo CIDR debe estar entre 0 y 32.');
      }

      const ipParts = ip.split('.').map(Number);
      if (ipParts.length !== 4 || ipParts.some(p => isNaN(p) || p < 0 || p > 255)) {
        throw new Error('Dirección IP inválida (ej. 192.168.1.1).');
      }

      const ipInt = (ipParts[0] << 24) + (ipParts[1] << 16) + (ipParts[2] << 8) + ipParts[3];
      const maskInt = cidr === 0 ? 0 : (~0 << (32 - cidr)) >>> 0;
      const netInt = (ipInt & maskInt) >>> 0;
      const broadInt = (netInt | ~maskInt) >>> 0;

      const intToIp = (int: number) => [
        (int >>> 24) & 255,
        (int >>> 16) & 255,
        (int >>> 8) & 255,
        int & 255
      ].join('.');

      const totalHosts = cidr >= 31 ? (cidr === 32 ? 1 : 2) : Math.pow(2, 32 - cidr) - 2;

      this.networkResult.set({
        network: intToIp(netInt),
        broadcast: intToIp(broadInt),
        netmask: intToIp(maskInt),
        totalHosts: totalHosts > 0 ? totalHosts : 0
      });
    } catch (e: any) {
      this.networkResult.set({
        network: '',
        broadcast: '',
        netmask: '',
        totalHosts: 0,
        error: e.message
      });
    }
  }

  seleccionarRed() {
    const res = this.networkResult();
    if (res && !res.error) {
      this.selectNetwork.emit(`${res.network}/${this.cidrInput()}`);
    }
    this.closeModal.emit();
  }
}
