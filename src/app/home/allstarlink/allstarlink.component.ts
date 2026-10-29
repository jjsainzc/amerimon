import { CommonModule } from '@angular/common';
import { CopyValueDirective } from '../../shared/copy-value/copy-value.directive';
import { Component, DestroyRef, OnDestroy, OnInit, computed, effect, inject, signal, untracked } from '@angular/core';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { FormsModule } from '@angular/forms';
import { DomSanitizer, SafeResourceUrl } from '@angular/platform-browser';
import { EMPTY, Observable, Subscription, concatMap, defer, expand, filter, from, map, of, switchMap, take, tap, timeout, timer, toArray } from 'rxjs';
import {
  AllstarlinkActivity,
  AllstarlinkGrupo,
  AllstarlinkNode,
  AllstarlinkResponse,
  AllstarlinkTabla,
  SupermonCommand,
  SupermonCommandRequest,
  SupermonCommandResult
} from '../../core/models/allstarlink.model';
import { AllstarlinkService } from '../../core/services/allstarlink.service';
import { AllstarlinkConnectionStateService } from '../../core/services/allstarlink-connection-state.service';
import { AllstarlinkViewStateService, CampoOrden } from '../../core/services/allstarlink-view-state.service';
import { AuthService } from '../../core/services/auth.service';
import { environment } from '../../core/environment';
import { indicativoQrz } from '../../core/validators/callsign.validator';
import { FavoritosPopupComponent } from '../../shared/favoritos-popup/favoritos-popup.component';
import { ScrollTopComponent } from '../../shared/scroll-top/scroll-top.component';
import { TranslatePipe } from '../../core/pipes/translate.pipe';
import { TranslationService } from '../../core/services/translation.service';
import { NoticePreferencesService } from '../../core/services/notice-preferences.service';
import { NoticePreferenceComponent } from '../../shared/notice-preference/notice-preference.component';

type SolicitudConexion = Readonly<SupermonCommandRequest & {
  remote_node: string;
  timeout_seconds: number;
}>;

@Component({
  selector: 'app-allstarlink',
  standalone: true,
  imports: [CommonModule, FormsModule, ScrollTopComponent, FavoritosPopupComponent, CopyValueDirective, TranslatePipe, NoticePreferenceComponent],
  templateUrl: './allstarlink.component.html',
  styleUrl: './allstarlink.component.css'
})
export class AllstarlinkComponent implements OnInit, OnDestroy {
  private readonly service = inject(AllstarlinkService);
  private readonly estadoConexion = inject(AllstarlinkConnectionStateService);
  private readonly estadoVista = inject(AllstarlinkViewStateService);
  private readonly auth = inject(AuthService);
  private readonly sanitizer = inject(DomSanitizer);
  private readonly destroyRef = inject(DestroyRef);
  private readonly noticePreferences = inject(NoticePreferencesService);
  readonly noMostrarConfirmacion = signal(false);

  // Para hacer translate dentro de los ts
  private readonly translation = inject(TranslationService);

  readonly nodos = signal<AllstarlinkNode[]>([]);
  readonly actividades = signal<AllstarlinkActivity[]>([]);
  readonly conexionesPendientes = signal<AllstarlinkNode[]>([]);
  readonly filtroBusqueda = this.estadoVista.filtroBusqueda;
  readonly servidorSeleccionado = this.estadoVista.servidorSeleccionado;
  readonly campoOrden = this.estadoVista.campoOrden;
  readonly direccionOrden = this.estadoVista.direccionOrden;
  readonly nodoPrincipal = signal('');
  /** Construye la URL segura del panel lsnodes o usa una página vacía sin nodo. */
  readonly lsnodesUrl = computed<SafeResourceUrl>(() => {
    const nodo = this.nodoPrincipal();
    return this.sanitizer.bypassSecurityTrustResourceUrl(nodo
      ? `${environment.apiUrl}/sm_lsnodes?format=html&node=${encodeURIComponent(nodo)}`
      : 'about:blank');
  });
  readonly lsnodesAbierto = signal(false);
  readonly tablasLocales = signal<AllstarlinkTabla[]>([]);
  readonly cargando = signal(false);
  readonly consultado = signal(false);
  readonly error = signal('');
  readonly erroresServidor = signal<Record<string, string>>({});
  readonly intervaloMs = this.estadoVista.intervaloMs;
  readonly esAdmin = this.auth.esAdmin;
  readonly panelAdminAbierto = this.estadoVista.panelAdminAbierto;
  readonly comandos = signal<SupermonCommand[]>([]);
  readonly cargandoComandos = signal(false);
  readonly errorComandos = signal('');
  readonly nodoLocal = this.estadoConexion.nodoLocal;
  readonly nodoRemoto = signal('');
  readonly nodoRemotoConexion = this.estadoConexion.nodoRemoto;
  readonly favoritosAbierto = signal(false);

  /** Selecciona el nodo favorito como destino de una conexión y cierra el selector. */
  elegirFavorito(numero: string): void {
    this.nodoRemotoConexion.set(numero);
    this.favoritosAbierto.set(false);
  }
  readonly modoConexion = this.estadoConexion.modo;
  readonly permanenteConexion = this.estadoConexion.permanente;
  readonly desconectarAntesConexion = this.estadoConexion.desconectarAntes;
  readonly timeoutConexion = this.estadoConexion.timeout;
  readonly conectando = signal(false);
  readonly desconectandoAntesConexion = signal(false);
  readonly resultadoConexion = signal<{ texto: string; error: boolean } | null>(null);
  readonly accionSeleccionada = signal('');
  readonly permanente = signal(false);
  readonly ejecutando = signal(false);
  readonly errorEjecucion = signal('');
  readonly resultado = signal<SupermonCommandResult | null>(null);
  readonly resultadoPopup = signal<{ titulo: string; comando: string; lineas: string[]; error: boolean; noticeKey: string } | null>(null);
  readonly confirmacionPendiente = signal<{ titulo: string; mensaje: string; confirmar: () => void; noticeKey: string } | null>(null);
  readonly desconexionEnCurso = signal<string | null>(null);
  readonly audioEnCurso = signal<string | null>(null);
  readonly resultadoDesconexion = signal<{ clave: string; texto: string; error: boolean } | null>(null);
  readonly tablasContraidas = this.estadoVista.tablasContraidas;
  private temporizador?: ReturnType<typeof setInterval>;
  private temporizadorRecargaDesconexion?: ReturnType<typeof setTimeout>;
  private recargaPendiente = false;
  private readonly consultasServidor = new Map<string, Subscription>();
  private readonly reintentosServidor = new Map<string, ReturnType<typeof setTimeout>>();
  private readonly recargasServidor = new Set<string>();
  private readonly desconexionesEnVerificacion = new Map<string, number>();
  /** Resuelve la operación administrativa elegida, si sigue disponible. */
  readonly comandoSeleccionado = computed(() =>
    this.comandos().find(comando => comando.id === this.accionSeleccionada()) ?? null
  );
  /** Determina si la operación elegida necesita confirmación explícita. */
  readonly requiereConfirmacion = computed(() => {
    const accion = this.accionSeleccionada();
    return !['core_version', 'core_uptime', 'node_status', 'link_status', 'iax_channels',
      'iax_netstats', 'iax_peers', 'iax_registry', 'rpt_channels', 'rpt_variables',
      'rpt_registrations'].includes(accion);
  });
  /** Agrupa las conexiones visibles por nodo local e incorpora las solicitudes pendientes. */
  readonly grupos = computed<AllstarlinkGrupo[]>(() => {
    const porNodoLocal = new Map<string, AllstarlinkNode[]>();
    const conexionesVisibles = [...this.nodos()];
    for (const pendiente of this.conexionesPendientes()) {
      const indice = conexionesVisibles.findIndex(nodo =>
        nodo.nodo_local === pendiente.nodo_local && nodo.nodo === pendiente.nodo &&
        nodo.protocolo?.trim().toLowerCase() === pendiente.protocolo?.trim().toLowerCase()
      );
      if (indice >= 0) {
        conexionesVisibles[indice] = {
          ...conexionesVisibles[indice],
          pendiente: true,
          permanente: pendiente.permanente
        };
      } else {
        conexionesVisibles.push(pendiente);
      }
    }

    for (const nodo of conexionesVisibles) {
      const nodoLocal = nodo.nodo_local || this.translation.translate('SIN_NODO_LOCAL');
      const conexiones = porNodoLocal.get(nodoLocal) ?? [];
      conexiones.push(nodo);
      porNodoLocal.set(nodoLocal, conexiones);
    }

    const nodosLocales = new Set([
      ...this.tablasLocales().map(tabla => tabla.nodo_local?.trim()).filter((nodo): nodo is string => !!nodo),
      ...porNodoLocal.keys()
    ]);
    const tablaPorNodo = new Map(this.tablasLocales().map(tabla => [tabla.nodo_local, tabla]));

    return Array.from(nodosLocales, nodoLocal => {
      const conexiones = porNodoLocal.get(nodoLocal) ?? [];
      const tabla = tablaPorNodo.get(nodoLocal);
      return {
        nodoLocal,
        detalle: [tabla?.descripcion, tabla?.ip, tabla?.hostname]
          .map(valor => valor?.trim()).filter(Boolean).join(' - '),
        nodos: conexiones,
        conectados: conexiones.filter(nodo => nodo.enlace?.trim().toLowerCase() === 'established').length,
        servidorConectado: tabla?.servidor_conectado ?? conexiones.length > 0
      };
    }).sort((a, b) =>
      Number(b.nodoLocal === this.nodoPrincipal()) - Number(a.nodoLocal === this.nodoPrincipal()) ||
      a.nodoLocal.localeCompare(b.nodoLocal, undefined, { numeric: true })
    );
  });
  /** Conserva la marca temporal más reciente por combinación de nodos local y remoto. */
  readonly actividadPorNodo = computed(() => {
    const actividadPorNodo = new Map<string, number>();
    for (const actividad of this.actividades()) {
      const clave = `${actividad.local_node.trim()}:${actividad.remote_node.trim()}`;
      const fecha = Date.parse(actividad.ultima_actividad);
      if (Number.isFinite(fecha) && fecha > (actividadPorNodo.get(clave) ?? Number.NEGATIVE_INFINITY)) {
        actividadPorNodo.set(clave, fecha);
      }
    }
    return actividadPorNodo;
  });
  /** Filtra nodos por texto y servidor, y los ordena según actividad o campo elegido. */
  readonly gruposFiltrados = computed(() => {
    const termino = this.normalizarTexto(this.filtroBusqueda().trim());
    const actividadPorNodo = this.actividadPorNodo();
    const servidorSeleccionado = this.servidorSeleccionado();
    return this.grupos()
      .filter(grupo => servidorSeleccionado === 'todos' || grupo.nodoLocal === servidorSeleccionado)
      .map(grupo => {
        const nodos = termino
          ? grupo.nodos.filter(nodo => [
            grupo.nodoLocal,
            nodo.protocolo,
            nodo.categoria,
            nodo.nodo,
            nodo.callsign,
            nodo.informacion,
            nodo.descripcion,
            nodo.contacto,
            nodo.ubicacion,
            nodo.enlace,
            nodo.direccion,
            nodo.conectado,
            nodo.modo
          ].some(valor => this.normalizarTexto(valor ?? '').includes(termino)))
          : grupo.nodos;
        const campo = this.campoOrden();
        const direccion = this.direccionOrden() === 'asc' ? 1 : -1;
        return {
          ...grupo,
          nodos: [...nodos].sort((a, b) => {
            const actividadA = actividadPorNodo.get(`${a.nodo_local.trim()}:${a.nodo.trim()}`);
            const actividadB = actividadPorNodo.get(`${b.nodo_local.trim()}:${b.nodo.trim()}`);
            const comparacion = campo === 'actividad'
              ? this.compararActividad(actividadA, actividadB, direccion)
              : this.valorOrden(a, campo).localeCompare(this.valorOrden(b, campo), undefined, { numeric: true, sensitivity: 'base' }) * direccion;
            return comparacion || this.compararActividad(actividadA, actividadB, -1);
          })
        };
      }).filter(grupo => !termino || grupo.nodos.length > 0);
  });
  /** Obtiene los nombres de servidor disponibles a partir de los grupos. */
  readonly servidoresDisponibles = computed(() => this.grupos().map(grupo => grupo.nodoLocal));
  /** Indica si hay más de un servidor para mostrar el selector. */
  readonly mostrarSelectorServidor = computed(() => this.servidoresDisponibles().length > 1);
  /** Indica si todos los servidores tienen la tabla contraída. */
  readonly todasLasTablasContraidas = computed(() =>
    this.grupos().length > 0 && this.grupos().every(grupo => this.tablaContraida(grupo.nodoLocal))
  );
  /** Reúne y ordena los nodos locales numéricos conocidos por las tablas y conexiones. */
  readonly nodosLocales = computed(() => Array.from(new Set(
    [
      ...this.tablasLocales().map(tabla => tabla.nodo_local?.trim()),
      ...this.nodos().map(nodo => nodo.nodo_local?.trim())
    ].filter((nodo): nodo is string => !!nodo && /^\d+$/.test(nodo))
  )).sort((a, b) => a.localeCompare(b, undefined, { numeric: true })));

  /** Carga los datos iniciales y programa su actualización periódica. */
  ngOnInit(): void {
    this.cargar();
    this.programarActualizacion();
  }

  // Los comandos de administración se piden cuando hay sesión de admin, incluso si inicia después.
  /** Solicita las operaciones administrativas cuando la sesión tiene permisos de admin. */
  private readonly cargaComandosAdmin = effect(() => {
    if (this.esAdmin()) { untracked(() => this.cargarComandos()); }
  });

  /** Cancela los temporizadores al destruir la vista. */
  ngOnDestroy(): void {
    if (this.temporizador) { clearInterval(this.temporizador); }
    if (this.temporizadorRecargaDesconexion) { clearTimeout(this.temporizadorRecargaDesconexion); }
    for (const temporizador of this.reintentosServidor.values()) { clearTimeout(temporizador); }
    for (const consulta of this.consultasServidor.values()) { consulta.unsubscribe(); }
  }

  /** Actualiza el intervalo de sondeo; un valor no numérico desactiva el temporizador. */
  cambiarIntervalo(valor: string): void {
    const intervalo = Number(valor);
    this.intervaloMs.set(Number.isFinite(intervalo) ? intervalo : 0);
    this.programarActualizacion();
  }

  /** Selecciona desde la cabecera el servidor que usará el formulario de conexión. */
  seleccionarServidorConexion(nodoLocal: string): void {
    this.nodoLocal.set(nodoLocal);
  }

  /** Indica si la tabla del servidor está contraída. */
  tablaContraida(nodoLocal: string): boolean {
    return this.tablasContraidas().has(nodoLocal);
  }

  /** Alterna la visibilidad de la tabla de un servidor sin ocultar su cabecera. */
  alternarTabla(nodoLocal: string): void {
    this.tablasContraidas.update(tablas => {
      const siguientes = new Set(tablas);
      if (siguientes.has(nodoLocal)) {
        siguientes.delete(nodoLocal);
      } else {
        siguientes.add(nodoLocal);
      }
      return siguientes;
    });
  }

  /** Contrae o expande todas las tablas sin afectar sus cabeceras. */
  alternarTodasLasTablas(): void {
    const grupos = this.grupos();
    if (grupos.length === 0) { return; }
    const contraer = !grupos.every(grupo => this.tablaContraida(grupo.nodoLocal));
    this.tablasContraidas.update(tablas => {
      const siguientes = new Set(tablas);
      for (const grupo of grupos) {
        if (contraer) {
          siguientes.add(grupo.nodoLocal);
        } else {
          siguientes.delete(grupo.nodoLocal);
        }
      }
      return siguientes;
    });
  }

  /** Quita diacríticos y normaliza el texto para búsquedas sin distinción de acentos. */
  private normalizarTexto(valor: string): string {
    return valor.normalize('NFD').replace(/[\u0300-\u036f]/g, '').toLocaleLowerCase();
  }

  /** Valida la forma admitida para identificar un nodo remoto. */
  private esNodoRemotoValido(valor: string): boolean {
    return /^[A-Za-z0-9][A-Za-z0-9._/-]*$/.test(valor);
  }

  /** Devuelve estadísticas externas para nodos numéricos públicos compatibles. */
  urlEstadisticasNodo(nodo: AllstarlinkNode): string | null {
    const numero = nodo.nodo?.trim();
    if (!numero || !/^\d{4,7}$/.test(numero) || this.categoriaNodo(nodo) === 'echolink') { return null; }
    return `https://stats.allstarlink.org/stats/${numero}`;
  }

  /** Genera un enlace QRZ cuando el indicativo puede normalizarse. */
  urlQrz(callsign?: string): string | null {
    const indicativo = indicativoQrz(callsign);
    return indicativo ? `https://www.qrz.com/db/${encodeURIComponent(indicativo)}` : null;
  }

  /** Elige el destino externo adecuado según la categoría del nodo. */
  urlNodo(nodo: AllstarlinkNode): string | null {
    const categoria = this.categoriaNodo(nodo);
    if (categoria === 'sip' || categoria === 'nodo_privado') { return null; }
    return categoria === 'iaxrpt' ? this.urlQrz(nodo.nodo) : this.urlEstadisticasNodo(nodo);
  }

  /** Obtiene el texto comparable de un nodo para el campo de orden solicitado. */
  private valorOrden(nodo: AllstarlinkNode, campo: CampoOrden): string {
    switch (campo) {
      case 'protocolo': return this.etiquetaProtocolo(nodo);
      case 'informacion': return nodo.informacion || nodo.descripcion || nodo.contacto || '';
      case 'actividad': return '';
      default: return nodo[campo] ?? '';
    }
  }

  /** Compara fechas de actividad y coloca las ausentes después de las disponibles. */
  private compararActividad(a: number | undefined, b: number | undefined, direccion: 1 | -1): number {
    if (a === b) { return 0; }
    if (a === undefined) { return 1; }
    if (b === undefined) { return -1; }
    return (a - b) * direccion;
  }

  /** Formatea en hora local la última actividad registrada para el nodo. */
  ultimaActividad(nodo: AllstarlinkNode): string {
    const fecha = this.actividadPorNodo().get(`${nodo.nodo_local.trim()}:${nodo.nodo.trim()}`);
    if (fecha === undefined) { return '—'; }
    const fechaLocal = new Date(fecha);
    const dosDigitos = (valor: number) => String(valor).padStart(2, '0');
    return `${fechaLocal.getFullYear()}-${dosDigitos(fechaLocal.getMonth() + 1)}-${dosDigitos(fechaLocal.getDate())} ${dosDigitos(fechaLocal.getHours())}:${dosDigitos(fechaLocal.getMinutes())}:${dosDigitos(fechaLocal.getSeconds())}`;
  }

  /** Selecciona el campo de orden o invierte la dirección del campo actual. */
  ordenarPor(campo: CampoOrden): void {
    if (this.campoOrden() === campo) {
      this.direccionOrden.update(direccion => direccion === 'asc' ? 'desc' : 'asc');
      return;
    }
    this.campoOrden.set(campo);
    this.direccionOrden.set(campo === 'actividad' ? 'desc' : 'asc');
  }

  /** Invierte la dirección de orden activa. */
  alternarDireccionOrden(): void {
    this.direccionOrden.update(direccion => direccion === 'asc' ? 'desc' : 'asc');
  }

  /** Alterna la visibilidad del panel de administración. */
  alternarPanelAdmin(): void {
    this.panelAdminAbierto.update(abierto => !abierto);
  }

  /** Abre el panel lsnodes únicamente para usuarios administradores. */
  abrirLsnodes(): void {
    if (this.esAdmin()) { this.lsnodesAbierto.set(true); }
  }

  /** Cierra el panel lsnodes. */
  cerrarLsnodes(): void {
    this.lsnodesAbierto.set(false);
  }

  /** Cambia la operación administrativa y limpia su resultado anterior. */
  cambiarAccion(valor: string): void {
    this.accionSeleccionada.set(valor);
    this.permanente.set(false);
    this.resultado.set(null);
    this.errorEjecucion.set('');
  }

  /** Recupera las operaciones disponibles para administración y selecciona la primera. */
  cargarComandos(): void {
    if (!this.esAdmin() || this.cargandoComandos()) { return; }
    this.cargandoComandos.set(true);
    this.errorComandos.set('');
    this.service.listarComandos().pipe(timeout({ each: 10_000 })).subscribe({
      next: respuesta => {
        const comandos = respuesta?.status === 'success'
          ? (respuesta.data ?? []).filter(comando => !['connect', 'monitor', 'localmonitor', 'disconnect'].includes(comando.id))
          : [];
        this.comandos.set(comandos);
        this.accionSeleccionada.set(comandos[0]?.id ?? '');
        if (respuesta?.status !== 'success') {
          this.errorComandos.set(respuesta?.message || this.translation.translate('NO_SE_PUDIERON_CARGAR_LAS_OPERACIONES'));
        }
        this.cargandoComandos.set(false);
      },
      error: err => {
        this.cargandoComandos.set(false);
        this.errorComandos.set(err?.name === 'TimeoutError'
          ? this.translation.translate('SERVIDOR_NO_RESPONDIO_A_TIEMPO')
          : err?.error?.message || this.translation.translate('NO_SE_PUDIERON_CARGAR_LAS_OPERACIONES'));
      }
    });
  }

  /** Valida, confirma y ejecuta la operación administrativa seleccionada. */
  ejecutarComando(): void {
    const comando = this.comandoSeleccionado();
    const localNode = this.nodoLocal().trim();
    const remoteNode = this.nodoRemoto().trim();
    if (!comando || !/^\d+$/.test(localNode) ||
        (comando.requires_remote_node && !this.esNodoRemotoValido(remoteNode))) {
      this.errorEjecucion.set(this.translation.translate('INGRESA_UN_NODO_LOCAL_NUMERICO_Y_UN_NODO_REMOTO_VALIDO'));
      this.mostrarPopup(this.translation.translate('OPERACION_NO_EJECUTADA'), '', [this.errorEjecucion()], true);
      return;
    }

    const request: SupermonCommandRequest = {
      local_node: localNode,
      ...(comando.requires_remote_node ? { remote_node: remoteNode } : {}),
      action: comando.id,
      permanent: comando.supports_permanent && comando.id !== 'disconnect' && this.permanente()
    };
    if (this.requiereConfirmacion()) {
      this.solicitarConfirmacion(
        `confirm.asterisk.command.${comando.id}`,
        this.translation.translate('CONFIRMAR_OPERACION'),
        this.translation.translate('SE_EJECUTARA_LA_OPERACION_EN_EL_NODO', {
          accion: comando.label,
          local: localNode,
          destino: remoteNode ? this.translation.translate('HACIA_EL_NODO', { remoto: remoteNode }) : ''
        }),
        () => this.ejecutarOperacion(comando, request)
      );
      return;
    }

    this.ejecutarOperacion(comando, request);
  }

  private ejecutarOperacion(comando: SupermonCommand, request: SupermonCommandRequest): void {
    if (!this.validarServidorOperacion(request.local_node)) { return; }
    this.ejecutando.set(true);
    this.errorEjecucion.set('');
    this.resultado.set(null);
    this.service.ejecutarComando(request).pipe(timeout({ each: 10_000 })).subscribe({
      next: respuesta => {
        this.ejecutando.set(false);
        if (respuesta?.status === 'success' && respuesta.data) {
          this.resultado.set(respuesta.data);
          this.mostrarPopup(comando.label, respuesta.data.command, respuesta.data.lines, false, `notice.asterisk.command.${comando.id}`);
        } else {
          this.errorEjecucion.set(respuesta?.message || this.translation.translate('NO_SE_PUDO_EJECUTAR_LA_OPERACION'));
          this.mostrarPopup(this.translation.translate('ERROR_DE_OPERACION'), '', [this.errorEjecucion()], true);
        }
      },
      error: err => {
        this.ejecutando.set(false);
        this.errorEjecucion.set(err?.name === 'TimeoutError'
          ? this.translation.translate('SERVIDOR_NO_RESPONDIO_A_TIEMPO')
          : err?.error?.message || this.translation.translate('NO_SE_PUDO_EJECUTAR_LA_OPERACION'));
        this.mostrarPopup(this.translation.translate('ERROR_DE_OPERACION'), '', [this.errorEjecucion()], true);
      }
    });
  }

  /** Solicita una conexión temporal o permanente tras validar y confirmar sus parámetros. */
  conectarNodo(): void {
    const local = this.nodoLocal().trim();
    const remoto = this.nodoRemotoConexion().trim();
    if (!this.esAdmin() || !/^\d+$/.test(local) || !this.esNodoRemotoValido(remoto)) {
      this.resultadoConexion.set({ texto: this.translation.translate('SELECCIONA_UN_SERVIDOR_E_INGRESA_UN_NODO_REMOTO_VALIDO'), error: true });
      this.mostrarPopup(this.translation.translate('CONEXION_NO_EJECUTADA'), '', [this.resultadoConexion()!.texto], true);
      return;
    }

    const modo = this.modoConexion();
    const modoLabel = modo === 'connect' ? 'Transceive' : modo === 'monitor' ? 'Monitor' : 'Local Monitor';
    const permanente = this.permanenteConexion();
    const timeoutSegundos = Math.min(300, Math.max(1, Math.floor(Number(this.timeoutConexion()) || 15)));
    this.timeoutConexion.set(timeoutSegundos);
    const tipoConexion = this.translation.translate(permanente ? 'TIPO_CONEXION_PERMANENTE' : 'TIPO_CONEXION_TEMPORAL');
    if (!this.validarServidorOperacion(local)) { return; }
    const request: SolicitudConexion = {
      local_node: local,
      remote_node: remoto,
      action: modo,
      permanent: permanente,
      timeout_seconds: timeoutSegundos,
      ...(this.desconectarAntesConexion() ? { disconnect_before_connect: true } : {})
    };
    this.solicitarConfirmacion(
      request.disconnect_before_connect ? 'confirm.asterisk.connect-replace-all' : 'confirm.asterisk.connect',
      this.translation.translate('CONFIRMAR_CONEXION'),
      this.translation.translate('CONFIRMACION_DE_CONEXION', {
        desconectar: request.disconnect_before_connect
          ? this.translation.translate('SE_DESCONECTARAN_TODOS_LOS_NODOS_DEL_SERVIDOR', { local }) + ' '
          : '',
        remoto,
        local,
        modo: this.translation.translate(modo === 'connect' ? 'TRANSCEIVE' : modo === 'monitor' ? 'MONITOR' : 'LOCAL_MONITOR'),
        tipo: tipoConexion
      }) + (request.disconnect_before_connect ? '' : ' ' + this.translation.translate('CONFIRMACION_CAMBIO_MODALIDAD_EXISTENTE')),
      () => this.ejecutarConexion(request, modoLabel)
    );
  }

  private ejecutarConexion(request: SolicitudConexion, modoLabel: string): void {
    if (!this.validarServidorOperacion(request.local_node)) { return; }
    const { local_node: local, remote_node: remoto, permanent: permanente, timeout_seconds: timeoutSegundos } = request;
    this.conectando.set(true);
    this.desconectandoAntesConexion.set(!!request.disconnect_before_connect);
    this.resultadoConexion.set(null);
    const preparacion = request.disconnect_before_connect ? this.desconectarNodosAntesDeConectar(local) : of(undefined);
    const { disconnect_before_connect: _desconectarAntes, ...solicitud } = request;
    preparacion.pipe(
      tap(() => {
        this.desconectandoAntesConexion.set(false);
        this.agregarConexionPendiente(local, remoto, modoLabel, permanente);
      }),
      switchMap(() => this.service.ejecutarComando(solicitud).pipe(
        timeout({ each: permanente ? 40_000 : timeoutSegundos * 1000 + 40_000 })
      )),
      takeUntilDestroyed(this.destroyRef)
    ).subscribe({
      next: respuesta => {
        this.conectando.set(false);
        this.desconectandoAntesConexion.set(false);
        if (respuesta?.status !== 'success') { this.quitarConexionPendiente(local, remoto); }
        this.programarRecargaConexiones();
        this.resultadoConexion.set(respuesta?.status === 'success' && respuesta.data
          ? { texto: [respuesta.data.command, ...respuesta.data.lines].filter(Boolean).join(' | '), error: false }
          : { texto: respuesta?.message || this.translation.translate('NO_SE_PUDO_CONECTAR_EL_NODO'), error: true });
        this.mostrarPopup(
          this.resultadoConexion()!.error ?  this.translation.translate('ERROR_DE_CONEXION') : this.translation.translate('CONEXION_EXITOSA'),
          respuesta?.data?.command ?? '',
          respuesta?.data?.lines ?? [this.resultadoConexion()!.texto],
          this.resultadoConexion()!.error,
          'notice.asterisk.connected'
        );
      },
      error: err => {
        this.conectando.set(false);
        this.desconectandoAntesConexion.set(false);
        this.quitarConexionPendiente(local, remoto);
        this.programarRecargaConexiones();
        this.resultadoConexion.set({
          texto: err?.name === 'TimeoutError'
            ? this.translation.translate('SERVIDOR_NO_RESPONDIO_A_TIEMPO')
            : err?.error?.message || err?.message || this.translation.translate('NO_SE_PUDO_CONECTAR_EL_NODO'),
          error: true
        });
        this.mostrarPopup(this.translation.translate('ERROR_DE_CONEXION'), '', [this.resultadoConexion()!.texto], true);
      }
    });
  }

  private nodosParaDesconectar(respuesta: AllstarlinkResponse, local: string): string[] {
    if (respuesta.status !== 'success' || !respuesta.data?.tablas?.some(tabla =>
      tabla.nodo_local === local && tabla.servidor_conectado
    ) || !Array.isArray(respuesta.data.resultados)) {
      throw new Error(respuesta.message || this.translation.translate('NO_SE_PUDO_CONSULTAR_LA_LISTA_DE_NODOS_DEL_SERVIDOR', { local }));
    }
    return [...new Set(respuesta.data.resultados
      .filter(nodo => nodo.nodo_local === local && nodo.protocolo?.trim().toLowerCase() !== 'sip')
      .map(nodo => nodo.nodo))];
  }

  private desconectarNodosAntesDeConectar(local: string): Observable<unknown> {
    const consultar = () => this.service.listarNodos(local).pipe(
      timeout({ each: 30_000 }),
      map(respuesta => this.nodosParaDesconectar(respuesta, local))
    );
    return consultar().pipe(
      switchMap(nodos => from(nodos).pipe(
        concatMap(remoto => this.service.ejecutarComando({
          local_node: local, remote_node: remoto, action: 'disconnect', permanent: false
        }).pipe(
          timeout({ each: 10_000 }),
          tap(respuesta => {
            if (respuesta.status !== 'success') {
              throw new Error(respuesta.message || this.translation.translate('NO_SE_PUDO_DESCONECTAR_EL_NODO_DEL_SERVIDOR', { remoto, local }));
            }
            this.quitarConexionPendiente(local, remoto);
          })
        )),
        toArray(),
        switchMap(() => defer(consultar).pipe(
          expand(restantes => restantes.length
            ? timer(500).pipe(switchMap(consultar))
            : EMPTY),
          filter(restantes => restantes.length === 0),
          take(1),
          timeout({ first: 15_000 })
        ))
      ))
    );
  }

  private validarServidorOperacion(local: string): boolean {
    if (this.esAdmin() && this.nodosLocales().includes(local)) { return true; }
    this.mostrarPopup( this.translation.translate('OPERACION_NO_EJECUTADA'), '', [
      this.esAdmin()
        ? this.translation.translate('EL_SERVIDOR_LOCAL_YA_NO_ESTA_DISPONIBLE', { local })
        :  this.translation.translate('SE_REQUIERE_SESION_ADMINISTRADOR')
    ], true);
    return false;
  }

  alternarAudio(nodo: AllstarlinkNode): void {
    const audio = nodo.audio;
    if (!this.esAdmin() || nodo.nodo_local !== this.nodoPrincipal() || !audio?.available ||
        !audio.connection_id || this.audioEnCurso() || nodo.pendiente || this.conectando() || this.desconexionEnCurso()) {
      return;
    }
    const muted = audio.muted !== true;
    const connectionId = audio.connection_id;
    this.solicitarConfirmacion(
      muted ? 'confirm.asterisk.mute' : 'confirm.asterisk.unmute',
      muted ? this.translation.translate('SILENCIAR_AUDIO_DE_ESTACION') : this.translation.translate('HABILITAR_AUDIO_DE_ESTACION'),
      muted
        ? this.translation.translate('IMPEDIR_QUE_EL_AUDIO_DEL_NODO_SE_RETRANSMITA')
        : this.translation.translate('PERMITIR_QUE_EL_AUDIO_DEL_NODO_SE_RETRANSMITA'),
      () => {
        if (!this.validarServidorOperacion(nodo.nodo_local) || this.audioEnCurso() || this.conectando() || this.desconexionEnCurso()) { return; }
        this.audioEnCurso.set(`${nodo.nodo_local}-${nodo.nodo}`);
        this.service.cambiarAudio(nodo.nodo_local, nodo.nodo, connectionId, muted).pipe(
          timeout({ each: 30_000 }), takeUntilDestroyed(this.destroyRef)
        ).subscribe({
          next: respuesta => {
            this.audioEnCurso.set(null);
            if (respuesta.status !== 'success' || respuesta.data?.muted !== muted ||
                respuesta.data.connection_id !== connectionId) {
              this.mostrarPopup(this.translation.translate('ERROR_DE_AUDIO'), '', [respuesta.message || this.translation.translate('ASTERISK_NO_CONFIRMO_ESTADO_DE_AUDIO')], true);
              this.programarRecargaConexiones();
              return;
            }
            const confirmado = respuesta.data;
            this.nodos.update(nodos => nodos.map(actual =>
              actual.nodo_local === nodo.nodo_local && actual.nodo === nodo.nodo &&
              actual.audio?.connection_id === connectionId
                ? { ...actual, audio: confirmado } : actual
            ));
            this.programarRecargaConexiones();
          },
          error: err => {
            this.audioEnCurso.set(null);
            this.nodos.update(nodos => nodos.map(actual =>
              actual.nodo_local === nodo.nodo_local && actual.nodo === nodo.nodo && actual.audio
                ? { ...actual, audio: { ...actual.audio, muted: null } } : actual
            ));
            this.mostrarPopup(this.translation.translate('ERROR_DE_AUDIO'), '', [
              err?.error?.message || this.translation.translate('ASTERISK_NO_CONFIRMO_EL_CAMBIO_DE_AUDIO')
            ], true);
            this.programarRecargaConexiones();
          }
        });
      }
    );
  }

  /** Solicita la desconexión de un nodo, espera confirmación y verifica el resultado. */
  desconectar(nodoLocal: string, nodoRemoto: string, confirmado = false): void {
    const local = nodoLocal.trim();
    const remoto = nodoRemoto.trim();
    const nodo = [...this.nodos(), ...this.conexionesPendientes()].find(item =>
      item.nodo_local === local && item.nodo === remoto && item.protocolo?.trim().toLowerCase() !== 'sip'
    );
    if (!this.esAdmin() || nodo?.protocolo?.trim().toLowerCase() === 'sip' || !/^\d+$/.test(local) || !this.esNodoRemotoValido(remoto)) { return; }

    const clave = `${local}-${remoto}`;
    if (this.desconexionEnCurso()) { return; }
    if (!confirmado) {
      // Para hacer translate dentro de los ts
      this.solicitarConfirmacion(
        'confirm.asterisk.disconnect',
        this.translation.translate('CONFIRMAR_DESCONEXION'),
        this.translation.translate('DESCONECTAR_EL_NODO_REMOTO_DEL NODO_LOCAL', { remoto, local }),
        () => this.desconectar(local, remoto, true)
      );
      return;
    }

    this.desconexionEnCurso.set(clave);
    this.resultadoDesconexion.set(null);
    this.service.ejecutarComando({
      local_node: local,
      remote_node: remoto,
      action: 'disconnect',
      permanent: false
    }).pipe(timeout({ each: 10_000 })).subscribe({
      next: respuesta => {
        if (respuesta?.status === 'success') {
          this.quitarConexionPendiente(local, remoto);
          this.desconexionesEnVerificacion.set(clave, 0);
          this.programarRecargaConexiones(1000);
        } else {
          this.desconexionEnCurso.set(null);
        }
        this.resultadoDesconexion.set(respuesta?.status === 'success' && respuesta.data
          ? { clave, texto: [respuesta.data.command, ...respuesta.data.lines].filter(Boolean).join(' | '), error: false }
          : { clave, texto: respuesta?.message || this.translation.translate('NO_SE_PUDO_DESCONECTAR_EL_NODO'), error: true });
        this.mostrarPopup(
          this.resultadoDesconexion()!.error ? this.translation.translate('ERROR_DE_DESCONEXION') : this.translation.translate('DESCONEXION_EXITOSA'),
          respuesta?.data?.command ?? '',
          respuesta?.data?.lines ?? [this.resultadoDesconexion()!.texto],
          this.resultadoDesconexion()!.error,
          'notice.asterisk.disconnected'
        );
      },
      error: err => {
        this.desconexionEnCurso.set(null);
        this.resultadoDesconexion.set({
          clave,
          texto: err?.name === 'TimeoutError'
            ? this.translation.translate('SERVIDOR_NO_RESPONDIO_A_TIEMPO')
            : err?.error?.message || this.translation.translate('NO_SE_PUDO_DESCONECTAR_EL_NODO'),
          error: true
        });
        this.mostrarPopup(this.translation.translate('ERROR_DE_DESCONEXION'), '', [this.resultadoDesconexion()!.texto], true);
      }
    });
  }

  /** Descarta el resultado emergente de la última operación. */
  cerrarPopup(): void {
    this.resultadoPopup.set(null);
  }

  /** Cancela el diálogo de confirmación sin ejecutar su acción. */
  cancelarConfirmacion(): void {
    this.confirmacionPendiente.set(null);
    this.noMostrarConfirmacion.set(false);
  }

  /** Ejecuta la acción asociada a la confirmación y cierra el diálogo. */
  confirmarAccion(): void {
    const confirmacion = this.confirmacionPendiente();
    this.confirmacionPendiente.set(null);
    if (confirmacion && this.noMostrarConfirmacion()) { this.noticePreferences.hide(confirmacion.noticeKey); }
    this.noMostrarConfirmacion.set(false);
    confirmacion?.confirmar();
  }

  /** Guarda en el estado la pregunta y la acción que requieren confirmación. */
  private solicitarConfirmacion(noticeKey: string, titulo: string, mensaje: string, confirmar: () => void): void {
    this.noMostrarConfirmacion.set(false);
    if (this.noticePreferences.isHidden(noticeKey)) {
      confirmar();
      return;
    }
    this.confirmacionPendiente.set({ titulo, mensaje, confirmar, noticeKey });
  }

  /** Publica el detalle de resultado que se presentará en el diálogo emergente. */
  private mostrarPopup(titulo: string, comando: string, lineas: string[], error = false, noticeKey = ''): void {
    if (!error && this.noticePreferences.isHidden(noticeKey)) { return; }
    this.resultadoPopup.set({ titulo, comando, lineas, error, noticeKey });
  }

  ocultarResultado(checked: boolean): void {
    const popup = this.resultadoPopup();
    if (checked && popup && !popup.error && popup.noticeKey) {
      this.noticePreferences.hide(popup.noticeKey);
      this.cerrarPopup();
    }
  }

  /** Asigna la clase visual correspondiente al modo de conexión. */
  claseModo(modo?: string): string {
    switch (modo?.trim().toLowerCase()) {
      case 'transceive': return 'modo-transceive';
      case 'simplex': return 'modo-simplex';
      case 'duplex': return 'modo-duplex';
      case 'monitor': return 'modo-monitor';
      case 'local monitor': return 'modo-duplex';
      default: return 'modo-otro';
    }
  }

  /** Abrevia los modos conocidos para la presentación compacta. */
  etiquetaModo(modo?: string): string {
    switch (modo?.trim().toLowerCase()) {
      case 'transceive': return 'T';
      case 'local monitor': return 'LM';
      case 'monitor': return 'M';
      default: return modo || '—';
    }
  }

  /** Oculta el estado establecido y conserva otros estados de enlace visibles. */
  etiquetaEnlace(enlace?: string): string {
    return enlace?.trim().toLowerCase() === 'established' ? '—' : enlace || '—';
  }

  /** Inserta o reemplaza la conexión local como pendiente hasta confirmarla en el sondeo. */
  private agregarConexionPendiente(local: string, remoto: string, modo: string, permanente: boolean): void {
    const categoria = remoto.length === 4
      ? 'nodo_privado'
      : remoto.startsWith('3') && remoto.length >= 5 ? 'echolink' : 'iax';
    const pendiente: AllstarlinkNode = {
      protocolo: 'iax',
      categoria,
      nodo_local: local,
      nodo: remoto,
      enlace: 'CONNECTING',
      direccion: 'OUT',
      modo,
      modo_codigo: modo === 'Transceive' ? 'T' : 'R',
      pendiente: true,
      permanente
    };
    this.conexionesPendientes.update(actuales => [
      ...actuales.filter(item => item.nodo_local !== local || item.nodo !== remoto),
      pendiente
    ]);
  }

  /** Retira de la lista local una conexión pendiente identificada por sus extremos. */
  private quitarConexionPendiente(local: string, remoto: string): void {
    this.conexionesPendientes.update(actuales =>
      actuales.filter(item => item.nodo_local !== local || item.nodo !== remoto)
    );
  }

  /** Quita solicitudes pendientes que ya aparecen establecidas en los datos del servidor. */
  private reconciliarConexionesPendientes(nodos: AllstarlinkNode[]): void {
    const establecidos = new Set(nodos
      .filter(nodo => nodo.enlace?.trim().toLowerCase() === 'established')
      .map(nodo => `${nodo.nodo_local}:${nodo.nodo}:${nodo.protocolo?.trim().toLowerCase() ?? ''}`));
    this.conexionesPendientes.update(actuales =>
      actuales.filter(nodo => !establecidos.has(`${nodo.nodo_local}:${nodo.nodo}:${nodo.protocolo?.trim().toLowerCase() ?? ''}`))
    );
  }

  /** Determina la categoría de un nodo usando su tipo, protocolo e información disponible. */
  categoriaNodo(nodo: AllstarlinkNode): string {
    const categoria = nodo.categoria?.trim().toLowerCase();
    const protocolo = nodo.protocolo?.trim().toLowerCase();
    const identificadorAlfanumerico = !!nodo.nodo && !/^\d+$/.test(nodo.nodo);
    if (protocolo === 'iax' && identificadorAlfanumerico && /iaxrpt|web txcvr|phone portal/i.test(nodo.informacion ?? '')) {
      return 'iaxrpt';
    }
    if (categoria) { return categoria; }

    if (protocolo === 'sip') { return 'sip'; }
    return nodo.nodo?.startsWith('3') && nodo.nodo.length >= 5 ? 'echolink' : 'iax';
  }

  /** Traduce la categoría detectada al nombre de protocolo mostrado en la tabla. */
  etiquetaProtocolo(nodo: AllstarlinkNode): string {
    switch (this.categoriaNodo(nodo)) {
      case 'iaxrpt': return 'IAXRPT';
      case 'echolink': return 'EchoLink';
      case 'nodo_privado': return this.translation.translate('PRIVADO');
      case 'sip': return 'SIP';
      default: return 'IAX';
    }
  }

  /** Devuelve la clase visual asociada a la categoría del nodo. */
  claseCategoria(nodo: AllstarlinkNode): string {
    switch (this.categoriaNodo(nodo)) {
      case 'iaxrpt': return 'modo-iaxrpt';
      case 'echolink': return 'modo-duplex';
      case 'nodo_privado': return 'modo-transceive';
      case 'sip': return 'modo-monitor';
      default: return 'modo-simplex';
    }
  }

  /** Descubre los servidores y consulta cada uno sin esperar a los demás. */
  cargar(forzar = false): void {
    if (this.cargando()) {
      if (forzar) { this.recargaPendiente = true; }
      return;
    }
    this.cargando.set(true);
    this.service.listarActividad().pipe(timeout({ each: 10_000 }), takeUntilDestroyed(this.destroyRef)).subscribe({
      next: respuesta => {
        if (respuesta?.status === 'success') {
          this.actividades.set(respuesta.data ?? []);
        }
      },
      error: () => undefined
    });
    this.service.listarServidores().pipe(timeout({ each: 10_000 }), takeUntilDestroyed(this.destroyRef)).subscribe({
      next: respuesta => {
        this.consultado.set(true);
        if (respuesta?.status === 'success') {
          this.nodoPrincipal.set(respuesta.data?.nodo_principal?.trim() ?? '');
          const tablas = respuesta.data?.tablas ?? [];
          const servidores = new Set(tablas.map(tabla => tabla.nodo_local));
          for (const [local, consulta] of this.consultasServidor) {
            if (!servidores.has(local)) {
              consulta.unsubscribe();
              this.consultasServidor.delete(local);
              this.recargasServidor.delete(local);
            }
          }
          for (const [local, temporizador] of this.reintentosServidor) {
            if (!servidores.has(local)) {
              clearTimeout(temporizador);
              this.reintentosServidor.delete(local);
            }
          }
          this.erroresServidor.update(errores => Object.fromEntries(
            Object.entries(errores).filter(([local]) => servidores.has(local))
          ));
          const anteriores = new Map(this.tablasLocales().map(tabla => [tabla.nodo_local, tabla]));
          this.tablasLocales.set(tablas.map(tabla => ({
            ...tabla,
            servidor_conectado: anteriores.get(tabla.nodo_local)?.servidor_conectado ?? tabla.servidor_conectado,
            nodos_disponibles: anteriores.get(tabla.nodo_local)?.nodos_disponibles ?? tabla.nodos_disponibles
          })));
          this.nodos.update(nodos => nodos.filter(nodo => servidores.has(nodo.nodo_local)));
          if (this.servidorSeleccionado() !== 'todos' &&
              !this.servidoresDisponibles().includes(this.servidorSeleccionado())) {
            this.servidorSeleccionado.set('todos');
          }
          if (!this.nodosLocales().includes(this.nodoLocal())) {
            this.nodoLocal.set(this.nodosLocales().includes(this.nodoPrincipal()) ? this.nodoPrincipal() : '');
          }
          this.error.set('');
          for (const tabla of tablas) { this.consultarServidor(tabla.nodo_local, forzar); }
        } else {
          this.limpiarDatosNodos();
          this.error.set(respuesta?.message || this.translation.translate('NO_SE_PUDO_CARGAR_EL_LISTADO_DE_NODOS'));
        }
        this.finalizarCarga();
        this.programarSondeoPermanente();
      },
      error: err => {
        this.limpiarDatosNodos();
        this.finalizarCarga();
        this.consultado.set(true);
        this.error.set(err?.name === 'TimeoutError'
          ? this.translation.translate('SERVIDOR_NO_RESPONDIO_A_TIEMPO')
          : err?.status === 0
            ? this.translation.translate('NO_SE_PUDO_CONECTAR_CON_EL_SERVIDOR')
            : err?.error?.message || this.translation.translate('NO_SE_PUDO_CARGAR_EL_LISTADO_DE_NODOS'));
        this.programarSondeoPermanente();
        this.revisarDesconexionesEnVerificacion();
      }
    });
  }

  private limpiarDatosNodos(): void {
    this.recargasServidor.clear();
    for (const consulta of this.consultasServidor.values()) { consulta.unsubscribe(); }
    this.consultasServidor.clear();
    for (const temporizador of this.reintentosServidor.values()) { clearTimeout(temporizador); }
    this.reintentosServidor.clear();
    this.tablasLocales.set([]);
    this.nodos.set([]);
    this.conexionesPendientes.set([]);
    this.erroresServidor.set({});
    this.nodoPrincipal.set('');
    this.nodoLocal.set('');
    this.servidorSeleccionado.set('todos');
  }

  private consultarServidor(local: string, forzar = false): void {
    if (this.consultasServidor.has(local)) {
      if (forzar) { this.recargasServidor.add(local); }
      return;
    }
    const reintento = this.reintentosServidor.get(local);
    if (reintento) {
      clearTimeout(reintento);
      this.reintentosServidor.delete(local);
    }
    const consulta = this.service.listarNodos(local).pipe(
      timeout({ each: 30_000 }), takeUntilDestroyed(this.destroyRef)
    ).subscribe({
      next: respuesta => {
        const tabla = respuesta.data?.tablas?.find(tabla => tabla.nodo_local === local);
        if (respuesta.status !== 'success' || !tabla?.servidor_conectado) {
          this.falloServidor(local, respuesta.message || this.translation.translate('NO_SE_PUDO_CONECTAR_CON_EL_SERVIDOR_DEL_NODO', { local }));
          return;
        }
        this.erroresServidor.update(errores => {
          const siguientes = { ...errores };
          delete siguientes[local];
          return siguientes;
        });
        this.tablasLocales.update(tablas => tablas.map(actual => actual.nodo_local === local ? tabla : actual));
        this.nodos.update(nodos => [
          ...nodos.filter(nodo => nodo.nodo_local !== local),
          ...(respuesta.data?.resultados ?? []).filter(nodo => nodo.nodo_local === local)
        ]);
        this.reconciliarConexionesPendientes(this.nodos());
        this.revisarDesconexionesEnVerificacion(this.nodos(), local);
      },
      error: err => {
        this.falloServidor(local, err?.name === 'TimeoutError'
          ? this.translation.translate('EL_SERVIDOR_DEL_NODO_NO_RESPONDIO_A_TIEMPO', { local })
          : err?.error?.message || this.translation.translate('NO_SE_PUDO_CONECTAR_CON_EL_SERVIDOR_DEL_NODO', { local }));
        this.finalizarConsultaServidor(local);
      },
      complete: () => this.finalizarConsultaServidor(local)
    });
    this.consultasServidor.set(local, consulta);
  }

  private falloServidor(local: string, mensaje: string): void {
    this.erroresServidor.update(errores => ({ ...errores, [local]: mensaje }));
    this.tablasLocales.update(tablas => tablas.map(tabla =>
      tabla.nodo_local === local ? { ...tabla, servidor_conectado: false } : tabla
    ));
    this.nodos.update(nodos => nodos.filter(nodo => nodo.nodo_local !== local));
    this.revisarDesconexionesEnVerificacion(undefined, local);
    this.reintentosServidor.set(local, setTimeout(() => {
      this.reintentosServidor.delete(local);
      this.consultarServidor(local);
    }, 5000));
  }

  private finalizarConsultaServidor(local: string): void {
    this.consultasServidor.delete(local);
    if (this.recargasServidor.delete(local)) { this.consultarServidor(local); }
  }

  /** Programa una nueva consulta y sustituye cualquier recarga diferida anterior. */
  private programarRecargaConexiones(retardoMs = 1500): void {
    if (this.temporizadorRecargaDesconexion) { clearTimeout(this.temporizadorRecargaDesconexion); }
    this.temporizadorRecargaDesconexion = setTimeout(() => {
      this.temporizadorRecargaDesconexion = undefined;
      this.cargar(true);
    }, retardoMs);
  }

  /** Mantiene sondeos periódicos mientras haya conexiones permanentes pendientes. */
  private programarSondeoPermanente(): void {
    if (this.conexionesPendientes().some(nodo => nodo.permanente)) {
      this.programarRecargaConexiones(5000);
    }
  }

  /** Comprueba la desconexión solicitada y reintenta la consulta hasta un límite. */
  private revisarDesconexionesEnVerificacion(nodos?: AllstarlinkNode[], nodoLocal?: string): void {
    if (this.desconexionesEnVerificacion.size === 0) { return; }
    let requiereOtraConsulta = false;
    for (const [clave, intentos] of this.desconexionesEnVerificacion) {
      const [local, ...partesRemoto] = clave.split('-');
      if (nodoLocal && local !== nodoLocal) { continue; }
      const remoto = partesRemoto.join('-');
      const sigueConectado = !nodos || nodos.some(nodo =>
        nodo.nodo_local === local && nodo.nodo === remoto && nodo.protocolo?.trim().toLowerCase() !== 'sip'
      );
      if (!sigueConectado) {
        this.desconexionesEnVerificacion.delete(clave);
        if (this.desconexionEnCurso() === clave) { this.desconexionEnCurso.set(null); }
        continue;
      }
      if (intentos >= 15) {
        this.desconexionesEnVerificacion.delete(clave);
        if (this.desconexionEnCurso() === clave) { this.desconexionEnCurso.set(null); }
        continue;
      }
      this.desconexionesEnVerificacion.set(clave, intentos + 1);
      requiereOtraConsulta = true;
    }
    if (requiereOtraConsulta) { this.programarRecargaConexiones(1500); }
  }

  /** Finaliza la consulta activa y procesa una recarga que se hubiera aplazado. */
  private finalizarCarga(): void {
    this.cargando.set(false);
    if (this.recargaPendiente) {
      this.recargaPendiente = false;
      this.cargar(true);
    }
  }

  /** Reinicia el temporizador de actualización con el intervalo configurado. */
  private programarActualizacion(): void {
    if (this.temporizador) { clearInterval(this.temporizador); }
    const intervalo = this.intervaloMs();
    if (intervalo > 0) {
      this.temporizador = setInterval(() => this.cargar(), intervalo);
    }
  }

}
