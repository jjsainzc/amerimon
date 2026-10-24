import { HttpErrorResponse } from '@angular/common/http';
import { Component, HostListener, OnInit, computed, inject, signal } from '@angular/core';
import { CopyValueDirective } from '../../../shared/copy-value/copy-value.directive';
import { FormBuilder, ReactiveFormsModule, Validators } from '@angular/forms';
import {
  AllstarlinkConnectionHistory,
  AllstarlinkConnectionHistoryFilters
} from '../../../core/models/allstarlink.model';
import { AllstarlinkService } from '../../../core/services/allstarlink.service';
import { indicativoQrz } from '../../../core/validators/callsign.validator';
import { AdminVolverComponent } from '../../../shared/admin-volver/admin-volver.component';
import { PrintButtonComponent } from '../../../shared/print-button/print-button.component';
import { ScrollTopComponent } from '../../../shared/scroll-top/scroll-top.component';
import { UtcToLocalPipe } from '../../../core/pipes/utc-to-local.pipe';
import { IsIpPipe } from '../../../core/pipes/is-ip.pipe';
import { GeoIpLinkComponent } from '../../../shared/geoip-link/geoip-link.component';
import { TablePaginationComponent } from '../../../shared/table-pagination/table-pagination.component';

type CampoOrden = 'local_node' | 'remote_node' | 'remote_ip' | 'remote_callsign' | 'remote_description' | 'connection_type' | 'first_seen_at' | 'disconnected_at' | 'duration';

@Component({
  selector: 'app-allstarlink-historial',
  standalone: true,
  imports: [UtcToLocalPipe, ReactiveFormsModule, AdminVolverComponent, PrintButtonComponent, ScrollTopComponent, IsIpPipe, GeoIpLinkComponent, TablePaginationComponent, CopyValueDirective],
  templateUrl: './allstarlink-historial.component.html',
  styleUrl: './allstarlink-historial.component.css'
})
export class AllstarlinkHistorialComponent implements OnInit {
  private readonly fb = inject(FormBuilder);
  private readonly service = inject(AllstarlinkService);

  readonly filtroForm = this.fb.nonNullable.group({
    local_node: '',
    remote_node: '',
    from: '',
    to: '',
    limit: ['500', [Validators.required, Validators.min(1), Validators.max(5000)]]
  });
  readonly conexiones = signal<AllstarlinkConnectionHistory[]>([]);
  readonly cargando = signal(false);
  readonly consultado = signal(false);
  readonly error = signal('');
  readonly busqueda = signal('');
  readonly ordenCampo = signal<CampoOrden>('first_seen_at');
  readonly ordenDireccion = signal<'asc' | 'desc'>('desc');
  readonly imprimiendo = signal(false);
  readonly pagina = signal(1);
  readonly itemsPorPagina = signal(25);
  /** Filtra, agrupa y ordena el historial según la búsqueda y los criterios activos. */
  readonly conexionesFiltradas = computed(() => {
    const consulta = this.busqueda().trim().toLocaleLowerCase();
    const coincidencias = consulta
      ? this.conexiones().filter(conexion => [
        conexion.local_node,
        conexion.remote_node,
        conexion.remote_ip,
        conexion.remote_callsign,
        conexion.remote_description,
        conexion.connection_type,
        conexion.first_seen_at,
        conexion.disconnected_at,
        this.duracionConexion(conexion)
      ].some(valor => String(valor ?? '').toLocaleLowerCase().includes(consulta)))
      : this.conexiones();
    const campo = this.ordenCampo();
    const direccion = this.ordenDireccion() === 'asc' ? 1 : -1;
    const agruparPorNodo = new Set(coincidencias.map(conexion => conexion.local_node)).size > 1;

    return [...coincidencias].sort((a, b) => {
      if (agruparPorNodo && campo !== 'local_node') {
        const grupo = a.local_node.localeCompare(b.local_node, undefined, { numeric: true });
        if (grupo) { return grupo; }
      }

      if (campo === 'duration') {
        const duracionA = this.duracionMs(a);
        const duracionB = this.duracionMs(b);
        if (duracionA === null || duracionB === null) {
          return duracionA === duracionB ? a.id - b.id : duracionA === null ? 1 : -1;
        }
        return (duracionA - duracionB) * direccion || a.id - b.id;
      }

      const valorA = String(a[campo] ?? '');
      const valorB = String(b[campo] ?? '');
      if (campo === 'disconnected_at' && Boolean(valorA) !== Boolean(valorB)) {
        return valorA ? -1 : 1;
      }

      const comparacion = campo === 'first_seen_at' || campo === 'disconnected_at'
        ? Date.parse(valorA) - Date.parse(valorB)
        : valorA.localeCompare(valorB, undefined, { numeric: true, sensitivity: 'base' });
      return (Number.isNaN(comparacion) ? 0 : comparacion) * direccion || a.id - b.id;
    });
  });
  /** Indica si los resultados visibles pertenecen a más de un nodo local. */
  readonly hayVariosNodosLocales = computed(() =>
    new Set(this.conexionesFiltradas().map(conexion => conexion.local_node)).size > 1
  );
  /** Calcula el número de páginas disponibles para los resultados filtrados. */
  readonly totalPaginas = computed(() => Math.max(1, Math.ceil(this.conexionesFiltradas().length / this.itemsPorPagina())));
  /** Devuelve la página actual, o todos los resultados durante la impresión. */
  readonly conexionesPaginadas = computed(() => {
    if (this.imprimiendo()) { return this.conexionesFiltradas(); }
    const inicio = (this.pagina() - 1) * this.itemsPorPagina();
    return this.conexionesFiltradas().slice(inicio, inicio + this.itemsPorPagina());
  });
  /** Calcula la posición inicial del tramo mostrado, adaptada a impresión. */
  readonly desdeRegistro = computed(() => this.conexionesFiltradas().length
    ? this.imprimiendo() ? 1 : (this.pagina() - 1) * this.itemsPorPagina() + 1
    : 0);
  /** Calcula la posición final del tramo mostrado o el total al imprimir. */
  readonly hastaRegistro = computed(() => this.imprimiendo()
    ? this.conexionesFiltradas().length
    : Math.min(this.pagina() * this.itemsPorPagina(), this.conexionesFiltradas().length));

  /** Ejecuta la primera consulta del historial al iniciar el componente. */
  ngOnInit(): void {
    this.cargar();
  }

  /** Valida filtros y fechas, consulta conexiones cerradas y actualiza la vista. */
  cargar(): void {
    this.filtroForm.markAllAsTouched();
    if (this.filtroForm.invalid) { return; }

    const filtro = this.filtroForm.getRawValue();
    if (filtro.from && filtro.to && filtro.from > filtro.to) {
      this.error.set('La fecha inicial no puede ser posterior a la fecha final.');
      return;
    }

    const parametros: AllstarlinkConnectionHistoryFilters = {
      local_node: filtro.local_node.trim() || undefined,
      remote_node: filtro.remote_node.trim() || undefined,
      status: 'disconnected',
      from: filtro.from || undefined,
      to: filtro.to || undefined,
      limit: Number(filtro.limit)
    };

    this.error.set('');
    this.cargando.set(true);
    this.pagina.set(1);
    this.busqueda.set('');
    this.service.listarHistorialConexiones(parametros).subscribe({
      next: respuesta => {
        this.cargando.set(false);
        this.consultado.set(true);
        this.conexiones.set(Array.isArray(respuesta?.data)
          ? respuesta.data.filter(conexion => !!conexion.disconnected_at)
          : []);
        if (respuesta?.status !== 'success') {
          this.error.set(respuesta?.message || 'No se pudo consultar el historial de conexiones.');
        }
      },
      error: (err: HttpErrorResponse) => {
        this.cargando.set(false);
        this.consultado.set(true);
        this.conexiones.set([]);
        this.error.set(this.mensajeError(err));
      }
    });
  }

  /** Restablece los filtros a sus valores iniciales y vuelve a consultar. */
  limpiar(): void {
    this.filtroForm.reset({
      local_node: '',
      remote_node: '',
      from: '',
      to: '',
      limit: '500'
    });
    this.cargar();
  }

  /** Genera el enlace a estadísticas para números válidos que no sean EchoLink. */
  urlEstadisticasNodo(nodo: string, tipoConexion?: string): string | null {
    const numero = nodo.trim();
    return tipoConexion?.toLowerCase() !== 'echolink' && /^\d{4,7}$/.test(numero)
      ? `https://stats.allstarlink.org/stats/${numero}`
      : null;
  }

  /** Genera el enlace QRZ a partir de un indicativo normalizado. */
  urlQrz(callsign?: string): string | null {
    const indicativo = indicativoQrz(callsign);
    return indicativo ? `https://www.qrz.com/db/${encodeURIComponent(indicativo)}` : null;
  }

  /** Traduce el tipo de conexión para su presentación en la tabla. */
  etiquetaTipoConexion(tipo?: string): string {
    switch (tipo?.trim().toLowerCase()) {
      case 'iax': return 'IAX';
      case 'iaxrpt': return 'IAXRPT';
      case 'echolink': return 'EchoLink';
      default: return tipo || '—';
    }
  }

  /** Devuelve la clase visual asociada al protocolo de conexión. */
  claseTipoConexion(tipo?: string): string {
    switch (tipo?.trim().toLowerCase()) {
      case 'iax': return 'modo-simplex';
      case 'iaxrpt': return 'modo-iaxrpt';
      case 'echolink': return 'modo-duplex';
      default: return 'modo-otro';
    }
  }

  /** Formatea la duración transcurrida entre conexión y desconexión. */
  duracionConexion(conexion: AllstarlinkConnectionHistory): string {
    const duracion = this.duracionMs(conexion);
    if (duracion === null) { return '—'; }

    let segundos = Math.floor(duracion / 1000);
    const dias = Math.floor(segundos / 86_400);
    segundos %= 86_400;
    const horas = Math.floor(segundos / 3_600);
    segundos %= 3_600;
    const minutos = Math.floor(segundos / 60);
    segundos %= 60;
    const partes: string[] = [];
    if (dias) { partes.push(`${dias} d`); }
    if (horas) { partes.push(`${horas} h`); }
    if (minutos) { partes.push(`${minutos} min`); }
    if (segundos || partes.length === 0) { partes.push(`${segundos} s`); }
    return partes.join(' ');
  }

  /** Aplica el texto de búsqueda y regresa a la primera página. */
  buscar(valor: string): void {
    this.busqueda.set(valor);
    this.pagina.set(1);
  }

  /** Actualiza el tamaño de página y reinicia la paginación. */
  cambiarItemsPorPagina(valor: string): void {
    this.itemsPorPagina.set(Number(valor));
    this.pagina.set(1);
  }

  /** Expande los resultados para que la vista de impresión incluya todos. */
  prepararImpresion(): void {
    this.imprimiendo.set(true);
  }

  /** Restaura la paginación normal cuando termina la impresión. */
  @HostListener('window:afterprint')
  restaurarVista(): void {
    this.imprimiendo.set(false);
  }

  /** Cambia el campo de orden o invierte su dirección si ya estaba seleccionado. */
  ordenarPor(campo: CampoOrden): void {
    if (this.ordenCampo() === campo) {
      this.ordenDireccion.update(direccion => direccion === 'asc' ? 'desc' : 'asc');
    } else {
      this.ordenCampo.set(campo);
      this.ordenDireccion.set(campo === 'first_seen_at' || campo === 'disconnected_at' ? 'desc' : 'asc');
    }
    this.pagina.set(1);
  }

  /** Devuelve el indicador textual de dirección para el encabezado activo. */
  indicadorOrden(campo: CampoOrden): string {
    if (this.ordenCampo() !== campo) { return ''; }
    return this.ordenDireccion() === 'asc' ? ' ▲' : ' ▼';
  }

  /** Informa la dirección accesible del orden o que la columna no está ordenada. */
  atributoOrden(campo: CampoOrden): 'ascending' | 'descending' | 'none' {
    if (this.ordenCampo() !== campo) { return 'none'; }
    return this.ordenDireccion() === 'asc' ? 'ascending' : 'descending';
  }

  /** Retrocede una página sin permitir índices inferiores a uno. */
  paginaAnterior(): void {
    this.pagina.update(actual => Math.max(1, actual - 1));
  }

  /** Avanza una página sin exceder el total disponible. */
  paginaSiguiente(): void {
    this.pagina.update(actual => Math.min(this.totalPaginas(), actual + 1));
  }

  /** Calcula la duración en milisegundos o null si faltan fechas válidas. */
  private duracionMs(conexion: AllstarlinkConnectionHistory): number | null {
    if (!conexion.disconnected_at) { return null; }
    const inicio = Date.parse(conexion.first_seen_at);
    const fin = Date.parse(conexion.disconnected_at);
    if (!Number.isFinite(inicio) || !Number.isFinite(fin)) { return null; }
    return Math.max(0, fin - inicio);
  }

  /** Convierte errores HTTP frecuentes en mensajes comprensibles para la vista. */
  private mensajeError(err: HttpErrorResponse): string {
    if (err.status === 0) { return 'No se pudo conectar con el servidor.'; }
    if (err.status === 401) { return 'Tu sesión expiró. Vuelve a iniciar sesión.'; }
    if (err.status === 403) { return 'No tienes privilegios de administrador.'; }
    return err.error?.message || `No se pudo cargar el historial (${err.status}).`;
  }
}
