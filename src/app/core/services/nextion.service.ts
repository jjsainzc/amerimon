import { HttpClient } from '@angular/common/http';
import { Injectable, inject } from '@angular/core';
import { Observable } from 'rxjs';
import { environment } from '../environment';

export interface NextionSettings {
  enabled: boolean;
  port: string;
  baud_rate: number;
  interface: string;
  favorite_node: string;
}

export interface NextionNetworkInterface {
  name: string;
  label: string;
  ipv4?: string;
}

export interface NextionPort {
  path: string;
  label: string;
  manufacturer?: string;
  product?: string;
  vendor_id?: string;
  product_id?: string;
  serial?: string;
}

export interface NextionModel {
  name: string;
  model: string;
  language: string;
  preview_url: string;
  source_available: boolean;
}

export interface NextionRuntime {
  running: boolean;
  port: string;
  local_node?: string;
  error?: string;
}

export interface NextionStatus {
  settings: NextionSettings;
  ports: NextionPort[];
  models: NextionModel[];
  runtime: NextionRuntime;
  interfaces: NextionNetworkInterface[];
  current_ipv4: string;
  local_node: string;
  ami_configured: boolean;
}

@Injectable({ providedIn: 'root' })
export class NextionService {
  private readonly http = inject(HttpClient);
  private readonly url = `${environment.apiUrl}/nextion`;

  obtener(): Observable<NextionStatus> {
    return this.http.get<NextionStatus>(this.url);
  }

  guardar(settings: NextionSettings): Observable<NextionStatus> {
    return this.http.put<NextionStatus>(this.url, settings);
  }

  probarPuerto(port: string, baudRate: number): Observable<{ status: string }> {
    return this.http.post<{ status: string }>(`${this.url}/test-port`, { port, baud_rate: baudRate });
  }

  descargar(model: string, extension: 'tft' | 'hmi'): Observable<Blob> {
    return this.http.get(`${this.url}/files/${encodeURIComponent(model)}/${extension}`, { responseType: 'blob' });
  }
}
