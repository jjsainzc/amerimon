import { HttpClient } from '@angular/common/http';
import { Injectable, inject } from '@angular/core';
import { environment } from '../environment';

export interface PrimaryNodeConfig {
  node: string;
  callsign: string;
  password_set: boolean;
  password: string;
  duplex: number;
  path: string;
}

export interface PrimaryAMIConfig {
  user: string;
  secret: string;
  secret_set: boolean;
  bind_address: string;
  permit: string[];
  deny: string[];
}

export interface NodeAccessEntry {
  type: 'allowlist' | 'denylist';
  node: string;
  reason: string;
}

export interface PublicCallsignEntry {
  type: 'whitelist' | 'blacklist';
  callsign: string;
  reason: string;
}

export interface NodeAccessDocument {
  format: 'amerimon-node-access';
  version: 1;
  node: string;
  entries: NodeAccessEntry[];
}

export function isNodeAccessDocument(value: unknown): value is NodeAccessDocument {
  if (typeof value !== 'object' || value === null ||
      !('format' in value) || value.format !== 'amerimon-node-access' ||
      !('version' in value) || value.version !== 1 ||
      !('node' in value) || typeof value.node !== 'string' || !/^[2-9][0-9]{3,5}$/.test(value.node) ||
      !('entries' in value) || !Array.isArray(value.entries) || value.entries.length > 1000) { return false; }
  return value.entries.every((entry: unknown) => typeof entry === 'object' && entry !== null &&
    'type' in entry && (entry.type === 'allowlist' || entry.type === 'denylist') &&
    'node' in entry && typeof entry.node === 'string' &&
    'reason' in entry && typeof entry.reason === 'string');
}

@Injectable({ providedIn: 'root' })
export class PrimaryNodeService {
  private readonly http = inject(HttpClient);
  private readonly url = `${environment.apiUrl}/primary-node`;

  obtener() {
    return this.http.get<PrimaryNodeConfig>(this.url);
  }

  obtenerAMI() {
    return this.http.get<PrimaryAMIConfig>(`${this.url}/ami`);
  }

  guardarAMI(config: PrimaryAMIConfig) {
    return this.http.put<{ status: string; message: string }>(`${this.url}/ami`, config);
  }

  guardar(current_node: string, node: string, callsign: string, password: string, duplex: number) {
    return this.http.put<{ message: string; node: string; restart_required: boolean }>(
      this.url, { current_node, node, callsign, password, duplex }
    );
  }

  listarAcceso() {
    return this.http.get<{ node: string; entries: NodeAccessEntry[] }>(`${this.url}/access`);
  }

  guardarAcceso(primary_node: string, entry: NodeAccessEntry) {
    return this.http.put(`${this.url}/access`, { primary_node, ...entry });
  }

  eliminarAcceso(primary_node: string, entry: NodeAccessEntry) {
    return this.http.delete(`${this.url}/access`, { body: { primary_node, ...entry } });
  }

  vaciarAcceso(primary_node: string, type: NodeAccessEntry['type']) {
    return this.http.post(`${this.url}/access/clear`, {
      primary_node, type, confirmation: `VACIAR ${type}`
    });
  }

  exportarAcceso() {
    return this.http.get<NodeAccessDocument>(`${this.url}/access/export`);
  }

  importarAcceso(primary_node: string, document: NodeAccessDocument) {
    return this.http.post<{ imported: number }>(`${this.url}/access/import`, { primary_node, document });
  }

  listarIndicativos() {
    return this.http.get<{ entries: PublicCallsignEntry[] }>(`${this.url}/public-access`);
  }

  guardarIndicativo(entry: PublicCallsignEntry) {
    return this.http.put(`${this.url}/public-access`, entry);
  }

  eliminarIndicativo(entry: PublicCallsignEntry) {
    return this.http.delete(`${this.url}/public-access`, { body: entry });
  }
}
