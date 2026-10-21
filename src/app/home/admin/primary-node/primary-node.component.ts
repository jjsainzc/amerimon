import { HttpErrorResponse } from '@angular/common/http';
import { Component, DestroyRef, OnInit, inject, signal } from '@angular/core';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { FormsModule } from '@angular/forms';
import { timeout } from 'rxjs';
import { NodeAccessDocument, NodeAccessEntry, PrimaryNodeService, isNodeAccessDocument } from '../../../core/services/primary-node.service';
import { AdminVolverComponent } from '../../../shared/admin-volver/admin-volver.component';
import { PasswordFieldComponent } from '../../../shared/password-field/password-field.component';

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
