import { HttpErrorResponse } from '@angular/common/http';
import { Component, DestroyRef, OnInit, inject, signal } from '@angular/core';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { FormsModule } from '@angular/forms';
import { timeout } from 'rxjs';
import { NodeAccessDocument, NodeAccessEntry, PrimaryAMIConfig, PrimaryNodeService, isNodeAccessDocument } from '../../../core/services/primary-node.service';
import { AdminVolverComponent } from '../../../shared/admin-volver/admin-volver.component';
import { PasswordFieldComponent } from '../../../shared/password-field/password-field.component';

export function calcularRedIPv4(value: string): string | null {
  const [address, maskText, extra] = value.trim().split('/');
  if (!address || extra !== undefined) { return null; }
  const octets = address.split('.').map(part => /^\d{1,3}$/.test(part) ? Number(part) : NaN);
  if (octets.length !== 4 || octets.some(part => !Number.isInteger(part) || part < 0 || part > 255)) { return null; }
  const addressValue = octets.reduce((total, part) => (total << 8n) | BigInt(part), 0n);
  let prefix: number;
  if (maskText === undefined || maskText === '') {
    prefix = 32;
  } else if (/^\d{1,2}$/.test(maskText)) {
    prefix = Number(maskText);
    if (prefix > 32) { return null; }
  } else {
    const maskOctets = maskText.split('.').map(part => /^\d{1,3}$/.test(part) ? Number(part) : NaN);
    if (maskOctets.length !== 4 || maskOctets.some(part => !Number.isInteger(part) || part < 0 || part > 255)) { return null; }
    const maskValue = maskOctets.reduce((total, part) => (total << 8n) | BigInt(part), 0n);
    const inverse = 0xffff_ffffn ^ maskValue;
    if ((inverse & (inverse + 1n)) !== 0n) { return null; }
    prefix = maskValue.toString(2).replace(/0/g, '').length;
  }
  const maskValue = prefix === 0 ? 0n : (0xffff_ffffn << BigInt(32 - prefix)) & 0xffff_ffffn;
  const network = addressValue & maskValue;
  const normalized = [24n, 16n, 8n, 0n].map(shift => Number((network >> shift) & 255n)).join('.');
  return `${normalized}/${prefix}`;
}

@Component({
  selector: 'app-primary-node',
  standalone: true,
  imports: [FormsModule, AdminVolverComponent, PasswordFieldComponent],
  templateUrl: './primary-node.component.html',
  styleUrl: './primary-node.component.css'
})
export class PrimaryNodeComponent implements OnInit {
  private readonly service = inject(PrimaryNodeService);
  private readonly destroyRef = inject(DestroyRef);

  readonly actual = signal('');
  readonly ruta = signal('');
  readonly passwordConfigurada = signal(false);
  readonly cargando = signal(false);
  readonly guardando = signal(false);
  readonly error = signal('');
  readonly aviso = signal('');
  readonly confirmando = signal(false);
  readonly entradas = signal<NodeAccessEntry[]>([]);
  readonly nodoAcceso = signal('');
  readonly cargandoAcceso = signal(false);
  readonly guardandoAcceso = signal(false);
  readonly errorAcceso = signal('');
  readonly avisoAcceso = signal('');
  readonly vaciadoPendiente = signal<{ tipo: NodeAccessEntry['type']; nodo: string } | null>(null);
  readonly importacionPendiente = signal<{ documento: NodeAccessDocument; nodo: string } | null>(null);
  readonly leyendoArchivo = signal(false);
  readonly amiConfig = signal<PrimaryAMIConfig | null>(null);
  readonly cargandoAMI = signal(false);
  readonly guardandoAMI = signal(false);
  readonly errorAMI = signal('');
  readonly avisoAMI = signal('');
  readonly amiSecret = signal('');
  readonly amiBindAddress = signal('');
  readonly permitsNuevas = signal('');
  readonly deniesNuevas = signal('');
  readonly tipoRed = signal<'permit' | 'deny'>('permit');
  readonly entradaRed = signal('');
  readonly redCalculada = signal<string | null>(null);
  numero = '';
  indicativo = '';
  password = '';
  private passwordOriginal = '';
  tipo: NodeAccessEntry['type'] = 'allowlist';
  remoto = '';
  motivo = '';

  ngOnInit(): void {
    this.cargar();
    this.cargarAcceso();
    this.cargarAMI();
  }

  cargarAMI(): void {
    if (this.cargandoAMI() || this.guardandoAMI()) { return; }
    this.cargandoAMI.set(true);
    this.errorAMI.set('');
    this.service.obtenerAMI().pipe(timeout(10_000), takeUntilDestroyed(this.destroyRef)).subscribe({
      next: response => {
        const config: PrimaryAMIConfig = {
          ...response,
          permit: Array.isArray(response.permit) ? response.permit : [],
          deny: Array.isArray(response.deny) ? response.deny : []
        };
        this.amiConfig.set(config);
        this.amiSecret.set(config.secret);
        this.amiBindAddress.set(config.bind_address);
        this.cargandoAMI.set(false);
      },
      error: (err: HttpErrorResponse) => {
        this.cargandoAMI.set(false);
        this.errorAMI.set(err.error?.message || 'No se pudo cargar la configuración AMI de manager.conf.');
      }
    });
  }

  calcularRed(): void {
    this.redCalculada.set(calcularRedIPv4(this.entradaRed()));
  }

  agregarRed(): void {
    const red = this.redCalculada();
    if (!red) { return; }
    const destination = this.tipoRed() === 'permit' ? this.permitsNuevas : this.deniesNuevas;
    destination.update(value => [...value.split(/\r?\n/).filter(Boolean), red].join('\n'));
    this.entradaRed.set('');
    this.redCalculada.set(null);
  }

  guardarAMI(): void {
    if (this.guardandoAMI() || this.cargandoAMI() || !this.amiConfig()) { return; }
    this.guardandoAMI.set(true);
    this.errorAMI.set('');
    this.avisoAMI.set('');
    const splitNetworks = (value: string) => value.split(/\r?\n/).map(network => network.trim()).filter(Boolean);
    const current = this.amiConfig();
    if (!current) { this.guardandoAMI.set(false); return; }
    this.service.guardarAMI({
      ...current,
      secret: this.amiSecret(),
      bind_address: this.amiBindAddress(),
      permit: current.permit.concat(splitNetworks(this.permitsNuevas())),
      deny: current.deny.concat(splitNetworks(this.deniesNuevas())),
    }).subscribe({
      next: response => {
        this.guardandoAMI.set(false);
        this.avisoAMI.set(response.message);
        this.permitsNuevas.set('');
        this.deniesNuevas.set('');
        this.cargarAMI();
      },
      error: (err: HttpErrorResponse) => {
        this.guardandoAMI.set(false);
        this.errorAMI.set(err.error?.message || 'No se pudo guardar la configuración AMI. No se confirmó ningún cambio.');
      }
    });
  }

  cargar(): void {
    if (this.cargando() || this.guardando()) { return; }
    this.cargando.set(true);
    this.error.set('');
    this.service.obtener().pipe(timeout(10_000), takeUntilDestroyed(this.destroyRef)).subscribe({
      next: config => {
        this.actual.set(config.node);
        this.ruta.set(config.path);
        this.passwordConfigurada.set(config.password_set);
        this.numero = config.node;
        this.indicativo = config.callsign;
        this.password = config.password;
        this.passwordOriginal = config.password;
        this.cargando.set(false);
      },
      error: (err: HttpErrorResponse) => {
        this.cargando.set(false);
        this.error.set(err.error?.message || 'No se pudo cargar la configuración del nodo principal.');
      }
    });
  }

  guardar(): void {
    if (this.guardando() || this.guardandoAcceso() || !this.actual()) { return; }
    this.guardando.set(true);
    this.confirmando.set(false);
    this.error.set('');
    this.aviso.set('');
    // No cancelar un guardado por timeout: podría completarse en el servidor.
    const password = this.password === this.passwordOriginal ? '' : this.password;
    this.service.guardar(this.actual(), this.numero, this.indicativo, password).subscribe({
      next: respuesta => {
        this.guardando.set(false);
        this.password = '';
        this.aviso.set(respuesta.message);
        this.cargar();
        this.cargarAcceso();
      },
      error: (err: HttpErrorResponse) => {
        this.guardando.set(false);
        this.password = '';
        this.error.set(err.error?.message || 'No se pudo confirmar el guardado. Recarga la configuración antes de reintentar.');
      }
    });
  }

  cargarAcceso(): void {
    if (this.cargandoAcceso() || this.guardandoAcceso()) { return; }
    this.cargandoAcceso.set(true);
    this.errorAcceso.set('');
    this.service.listarAcceso().pipe(timeout(15_000), takeUntilDestroyed(this.destroyRef)).subscribe({
      next: respuesta => {
        this.nodoAcceso.set(respuesta.node);
        this.entradas.set(respuesta.entries);
        this.cargandoAcceso.set(false);
      },
      error: (err: HttpErrorResponse) => {
        this.cargandoAcceso.set(false);
        this.errorAcceso.set(err.error?.message || 'No se pudieron cargar las listas de acceso. Comprueba la conexión AMI.');
      }
    });
  }

  cambiarAcceso(entry: NodeAccessEntry, eliminar = false): void {
    if (this.guardandoAcceso() || this.guardando() || !this.nodoAcceso()) { return; }
    this.guardandoAcceso.set(true);
    this.errorAcceso.set('');
    this.avisoAcceso.set('');
    const solicitud = eliminar
      ? this.service.eliminarAcceso(this.nodoAcceso(), entry)
      : this.service.guardarAcceso(this.nodoAcceso(), entry);
    solicitud.subscribe({
      next: () => {
        this.guardandoAcceso.set(false);
        this.avisoAcceso.set('Lista de acceso actualizada en Asterisk.');
        if (!eliminar) { this.remoto = ''; this.motivo = ''; }
        this.cargarAcceso();
      },
      error: (err: HttpErrorResponse) => {
        this.guardandoAcceso.set(false);
        this.errorAcceso.set(err.error?.message || 'No se pudo confirmar el cambio. Recarga la lista antes de reintentar.');
      }
    });
  }

  solicitarVaciado(tipo: NodeAccessEntry['type']): void {
    if (this.guardandoAcceso() || this.guardando() || this.cargandoAcceso() || !this.nodoAcceso()) { return; }
    this.vaciadoPendiente.set({ tipo, nodo: this.nodoAcceso() });
  }

  vaciarAcceso(): void {
    const pendiente = this.vaciadoPendiente();
    if (!pendiente || this.guardandoAcceso() || this.guardando() || this.cargandoAcceso()) { return; }
    this.vaciadoPendiente.set(null);
    this.guardandoAcceso.set(true);
    this.errorAcceso.set('');
    this.avisoAcceso.set('');
    this.service.vaciarAcceso(pendiente.nodo, pendiente.tipo).subscribe({
      next: () => {
        this.guardandoAcceso.set(false);
        this.avisoAcceso.set(`Lista de ${pendiente.tipo === 'allowlist' ? 'permitidos' : 'denegados'} vaciada en Asterisk.`);
        this.cargarAcceso();
      },
      error: (err: HttpErrorResponse) => {
        this.guardandoAcceso.set(false);
        this.errorAcceso.set(err.error?.message || 'No se pudo confirmar el vaciado. Recarga la lista antes de reintentar.');
      }
    });
  }

  exportarAcceso(): void {
    if (this.guardandoAcceso() || this.guardando() || this.cargandoAcceso() || this.leyendoArchivo()) { return; }
    this.guardandoAcceso.set(true);
    this.errorAcceso.set('');
    this.avisoAcceso.set('');
    this.service.exportarAcceso().pipe(timeout(15_000), takeUntilDestroyed(this.destroyRef)).subscribe({
      next: documento => {
        const blob = new Blob([JSON.stringify(documento, null, 2)], { type: 'application/json' });
        const url = URL.createObjectURL(blob);
        const enlace = document.createElement('a');
        enlace.href = url;
        enlace.download = `listas-acceso-${documento.node}.json`;
        enlace.click();
        setTimeout(() => URL.revokeObjectURL(url), 1000);
        this.guardandoAcceso.set(false);
        this.avisoAcceso.set('Archivo JSON de ambas listas exportado.');
      },
      error: (err: HttpErrorResponse) => {
        this.guardandoAcceso.set(false);
        this.errorAcceso.set(err.error?.message || 'No se pudieron exportar las listas.');
      }
    });
  }

  async seleccionarImportacion(event: Event): Promise<void> {
    if (!(event.target instanceof HTMLInputElement)) { return; }
    const input = event.target;
    const archivo = input.files?.[0];
    input.value = '';
    if (!archivo || this.guardandoAcceso() || this.guardando() || this.cargandoAcceso() || this.leyendoArchivo()) { return; }
    this.importacionPendiente.set(null);
    this.errorAcceso.set('');
    this.avisoAcceso.set('');
    if (archivo.size > 1024 * 1024) {
      this.errorAcceso.set('El archivo JSON no puede superar 1 MiB.');
      return;
    }
    const nodo = this.nodoAcceso();
    this.leyendoArchivo.set(true);
    try {
      const documento: unknown = JSON.parse(await archivo.text());
      if (!isNodeAccessDocument(documento)) {
        this.errorAcceso.set('Formato inválido. Selecciona un archivo JSON exportado por Amerimon, versión 1, con hasta 1000 entradas.');
        return;
      }
      this.importacionPendiente.set({ documento, nodo });
    } catch (err: unknown) {
      this.errorAcceso.set(err instanceof SyntaxError ? 'El archivo no contiene JSON válido.' : 'No se pudo leer el archivo seleccionado.');
    } finally {
      this.leyendoArchivo.set(false);
    }
  }

  importarAcceso(): void {
    const pendiente = this.importacionPendiente();
    if (!pendiente || this.guardandoAcceso() || this.guardando() || this.cargandoAcceso() || this.leyendoArchivo()) { return; }
    this.importacionPendiente.set(null);
    this.guardandoAcceso.set(true);
    this.errorAcceso.set('');
    this.avisoAcceso.set('');
    this.service.importarAcceso(pendiente.nodo, pendiente.documento).subscribe({
      next: respuesta => {
        this.guardandoAcceso.set(false);
        this.avisoAcceso.set(`${respuesta.imported} entradas importadas. Se conservaron las demás entradas existentes.`);
        this.cargarAcceso();
      },
      error: (err: HttpErrorResponse) => {
        this.guardandoAcceso.set(false);
        this.errorAcceso.set(err.error?.message || 'No se pudo confirmar la importación. Recarga y revisa las listas antes de reintentar.');
      }
    });
  }
}
