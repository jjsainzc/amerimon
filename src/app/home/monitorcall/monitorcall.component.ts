import { Component, HostListener, OnDestroy, OnInit, effect, inject, signal } from '@angular/core';
import { ListenLiveService } from '../../core/services/listenlive.service';
import { ListenLiveSessionsService } from '../../core/services/listenlive-sessions.service';
import { ListenLiveSession } from '../../core/models/listenlive-session.model';
import { AuthService } from '../../core/services/auth.service';
import { indicativoQrz } from '../../core/validators/callsign.validator';
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

  ngOnInit(): void {
    void this.listenService.checkAsteriskConnection();
  }

  actualizarMonitor(): void {
    void this.cargarSesiones();
    void this.listenService.checkAsteriskConnection();
  }

  protected extValida(): boolean {
    return /^[0-9A-Za-z_]{1,32}$/.test(this.ext());
  }

  protected urlQrz(callsign: string | null): string | null {
    const indicativo = indicativoQrz(callsign);
    return indicativo ? `https://www.qrz.com/db/${encodeURIComponent(indicativo)}` : null;
  }

  listen() {
    if (this.extValida()) {
      void this.iniciarEscucha(this.ext());
    }
  }

  listenNodo() {
    this.ext.set('nodo');
    void this.iniciarEscucha('nodo');
  }

  async stop(): Promise<void> {
    await this.listenService.stopListening();
    await this.cargarSesiones();
  }

  protected readonly sesionPendiente = signal<ListenLiveSession | null>(null);

  protected pedirDesconexion(sesion: ListenLiveSession): void {
    this.sesionPendiente.set(sesion);
  }

  protected cancelarDesconexion(): void {
    this.sesionPendiente.set(null);
  }

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

  protected abrirManual(): void {
    this.manualAbierto.set(true);
  }

  protected cerrarManual(): void {
    this.manualAbierto.set(false);
  }

  @HostListener('document:keydown.escape')
  cerrarManualConEscape(): void {
    this.cerrarManual();
  }

  ngOnDestroy(): void {
    void this.listenService.stopListening();
  }

  private async iniciarEscucha(ext: string): Promise<void> {
    await this.listenService.startListening(ext);
    await this.cargarSesiones();
  }
}
