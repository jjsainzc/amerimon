import { Component, HostListener, OnDestroy, OnInit, effect, inject, signal } from '@angular/core';
import { ListenLiveService } from '../../core/services/listenlive.service';
import { ListenLiveSessionsService } from '../../core/services/listenlive-sessions.service';
import { ListenLiveSession } from '../../core/models/listenlive-session.model';
import { AuthService } from '../../core/services/auth.service';
import { LoadingIndicatorComponent } from '../../shared/loading-indicator/loading-indicator.component';
import { MarkdownViewerComponent } from '../../shared/markdown-viewer/markdown-viewer.component';
import { UtcToLocalPipe } from '../../core/pipes/utc-to-local.pipe';
import { GeoIpLinkComponent } from '../../shared/geoip-link/geoip-link.component';
import { ConfirmacionDialogComponent } from '../../shared/confirmacion-dialog/confirmacion-dialog.component';

@Component({
  selector: 'app-monitor-call',
  standalone: true,
  imports: [UtcToLocalPipe, LoadingIndicatorComponent, MarkdownViewerComponent, ConfirmacionDialogComponent, GeoIpLinkComponent],
  templateUrl: './monitorcall.component.html',
  styleUrl: './monitorcall.component.css'
})
export class MonitorCallComponent implements OnInit, OnDestroy {
  protected listenService = inject(ListenLiveService);
  private readonly sessionsService = inject(ListenLiveSessionsService);
  private readonly authService = inject(AuthService);

  protected readonly ext = signal('nodo');
  protected readonly esAdmin = this.authService.esAdmin;
  protected readonly sesiones = signal<ListenLiveSession[]>([]);
  protected readonly cargandoSesiones = signal(false);
  protected readonly errorSesiones = signal<string | null>(null);
  protected readonly manualAbierto = signal(false);

  private readonly sessionChanges = effect(() => {
    this.listenService.sessionRevision();
    void this.cargarSesiones();
  });

  /** Comprueba la conexión y carga las sesiones activas al montar la vista. */
  ngOnInit(): void {
    void this.listenService.checkAsteriskConnection();
  }

  /** Actualiza el reporte de sesiones y vuelve a comprobar Asterisk. */
  actualizarMonitor(): void {
    void this.cargarSesiones();
    void this.listenService.checkAsteriskConnection();
  }

  /** Comprueba que la extensión contenga solo caracteres admitidos y mida hasta 32. */
  protected extValida(): boolean {
    return /^[0-9A-Za-z_]{1,32}$/.test(this.ext());
  }

  /** Inicia la escucha de la extensión actual si su formato es válido. */
  listen() {
    if (this.extValida()) {
      void this.iniciarEscucha(this.ext());
    }
  }

  /** Selecciona la extensión especial del nodo y comienza a escucharla. */
  listenNodo() {
    this.ext.set('nodo');
    void this.iniciarEscucha('nodo');
  }

  /** Detiene la escucha local y actualiza el listado de sesiones activas. */
  async stop(): Promise<void> {
    await this.listenService.stopListening();
    await this.cargarSesiones();
  }

  protected readonly sesionPendiente = signal<ListenLiveSession | null>(null);

  /** Guarda la sesión elegida como desconexión pendiente. */
  protected pedirDesconexion(sesion: ListenLiveSession): void {
    this.sesionPendiente.set(sesion);
  }

  /** Descarta la solicitud de desconexión de la sesión. */
  protected cancelarDesconexion(): void {
    this.sesionPendiente.set(null);
  }

  /** Desconecta la sesión confirmada y vuelve a consultar las sesiones activas. */
  protected async confirmarDesconexion(): Promise<void> {
    const sesion = this.sesionPendiente();
    this.sesionPendiente.set(null);
    if (!sesion) {
      return;
    }
    try {
      await this.sessionsService.desconectar(sesion.listenliveid);
    } catch {
      this.errorSesiones.set('No se pudo desconectar la sesión.');
    }
    await this.cargarSesiones();
  }

  /** Obtiene las escuchas activas y excluye la sesión del usuario actual. */
  async cargarSesiones(): Promise<void> {
    this.cargandoSesiones.set(true);
    this.errorSesiones.set(null);
    try {
      const sesiones = await this.sessionsService.listar();
      const currentUserId = this.authService.getCurrentUserId();
      this.sesiones.set(sesiones.filter(sesion =>
        !sesion.fecha_fin && (currentUserId === null || sesion.userid !== currentUserId)
      ));
    } catch {
      this.errorSesiones.set('No se pudo cargar el reporte de escuchas.');
    } finally {
      this.cargandoSesiones.set(false);
    }
  }

  /** Abre el manual de ayuda de la pantalla. */
  protected abrirManual(): void {
    this.manualAbierto.set(true);
  }

  /** Cierra el manual de ayuda. */
  protected cerrarManual(): void {
    this.manualAbierto.set(false);
  }

  /** Cierra el manual cuando se pulsa Escape en el documento. */
  @HostListener('document:keydown.escape')
  cerrarManualConEscape(): void {
    this.cerrarManual();
  }

  /** Detiene cualquier escucha al destruir el componente. */
  ngOnDestroy(): void {
    void this.listenService.stopListening();
  }

  /** Inicia una escucha y actualiza el reporte para reflejarla. */
  private async iniciarEscucha(ext: string): Promise<void> {
    await this.listenService.startListening(ext);
    await this.cargarSesiones();
  }
}
