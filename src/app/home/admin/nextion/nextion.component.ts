import { HttpErrorResponse } from '@angular/common/http';
import { Component, DestroyRef, HostListener, OnInit, inject, signal } from '@angular/core';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { FormsModule } from '@angular/forms';
import { forkJoin, timer } from 'rxjs';
import { Favorito } from '../../../core/models/favorito.model';
import { FavoritosService } from '../../../core/services/favoritos.service';
import { NextionModel, NextionNetworkInterface, NextionPort, NextionRuntime, NextionService, NextionSettings, NextionStatus } from '../../../core/services/nextion.service';
import { FormChanges } from '../../../core/utils/form-changes';
import { TranslatePipe } from '../../../core/pipes/translate.pipe';
import { AdminVolverComponent } from '../../../shared/admin-volver/admin-volver.component';

type HttpErrorBody = { message?: string };

@Component({
  selector: 'app-nextion',
  standalone: true,
  imports: [FormsModule, TranslatePipe, AdminVolverComponent],
  templateUrl: './nextion.component.html',
  styleUrl: './nextion.component.css',
})
export class NextionComponent implements OnInit {
  private readonly service = inject(NextionService);
  private readonly favoritosService = inject(FavoritosService);
  private readonly destroyRef = inject(DestroyRef);

  protected readonly cargando = signal(false);
  protected readonly guardando = signal(false);
  protected readonly probando = signal(false);
  protected readonly error = signal('');
  protected readonly aviso = signal('');
  protected readonly puertos = signal<NextionPort[]>([]);
  protected readonly interfaces = signal<NextionNetworkInterface[]>([]);
  protected readonly favoritos = signal<Favorito[]>([]);
  protected readonly modelos = signal<NextionModel[]>([]);
  protected readonly vistaPrevia = signal<NextionModel | null>(null);
  protected readonly runtime = signal<NextionRuntime>({ running: false, port: '' });
  protected readonly nodoLocal = signal('');
  protected readonly ipv4Actual = signal('--');
  protected readonly amiConfigurado = signal(false);
  protected readonly cambios = new FormChanges();

  protected habilitado = false;
  protected puerto = '';
  protected velocidad = 9600;
  protected interfaz = '';
  protected nodoFavorito = '';

  protected velocidades = [9600, 19200, 38400, 57600, 115200];

  ngOnInit(): void {
    this.cargar();
  }

  protected datosFormulario(): NextionSettings {
    return {
      enabled: this.habilitado,
      port: this.puerto.trim(),
      baud_rate: Number(this.velocidad),
      interface: this.interfaz.trim(),
      favorite_node: this.nodoFavorito.trim(),
    };
  }

  protected cargar(): void {
    if (this.cargando() || this.guardando() || this.probando()) { return; }
    this.cargando.set(true);
    this.error.set('');
    forkJoin({ status: this.service.obtener(), favoritos: this.favoritosService.listar() })
      .pipe(takeUntilDestroyed(this.destroyRef)).subscribe({
        next: ({ status, favoritos }) => {
          this.aplicarEstado(status);
          this.favoritos.set(favoritos);
          this.cargando.set(false);
        },
        error: (error: HttpErrorResponse) => {
          this.error.set(this.mensajeError(error, 'NEXTION_LOAD_ERROR'));
          this.cargando.set(false);
        },
      });
  }

  protected guardar(): void {
    if (this.guardando() || this.cargando() || !this.cambios.changed(this.datosFormulario())) { return; }
    this.guardando.set(true);
    this.error.set('');
    this.aviso.set('');
    this.service.guardar(this.datosFormulario()).pipe(takeUntilDestroyed(this.destroyRef)).subscribe({
      next: status => {
        this.aplicarEstado(status);
        this.guardando.set(false);
        this.aviso.set('NEXTION_SAVED');
        if (status.settings.enabled) {
          timer(1500).pipe(takeUntilDestroyed(this.destroyRef)).subscribe(() => this.cargar());
        }
      },
      error: (error: HttpErrorResponse) => {
        this.error.set(this.mensajeError(error, 'NEXTION_SAVE_ERROR'));
        this.guardando.set(false);
      },
    });
  }

  protected probarPuerto(): void {
    const port = this.puerto.trim();
    if (this.probando() || this.guardando() || !port) { return; }
    this.probando.set(true);
    this.error.set('');
    this.aviso.set('');
    this.service.probarPuerto(port, Number(this.velocidad)).pipe(takeUntilDestroyed(this.destroyRef)).subscribe({
      next: () => {
        this.aviso.set('NEXTION_PORT_OK');
        this.probando.set(false);
      },
      error: (error: HttpErrorResponse) => {
        this.error.set(this.mensajeError(error, 'NEXTION_PORT_ERROR'));
        this.probando.set(false);
      },
    });
  }

  protected descargar(model: NextionModel, extension: 'tft' | 'hmi'): void {
    if (this.guardando() || this.cargando()) { return; }
    this.error.set('');
    this.service.descargar(model.name, extension).pipe(takeUntilDestroyed(this.destroyRef)).subscribe({
      next: file => {
        const url = URL.createObjectURL(file);
        const anchor = document.createElement('a');
        anchor.href = url;
        anchor.download = `${model.name}.${extension === 'hmi' ? 'HMI' : 'tft'}`;
        anchor.click();
        setTimeout(() => URL.revokeObjectURL(url), 1000);
      },
      error: (error: HttpErrorResponse) => this.error.set(this.mensajeError(error, 'NEXTION_DOWNLOAD_ERROR')),
    });
  }

  protected favoritoLabel(favorito: Favorito): string {
    return favorito.descripcion ? `${favorito.numero} — ${favorito.descripcion}` : favorito.numero;
  }

  protected favoritoGuardado(numero: string): boolean {
    return this.favoritos().some(favorito => favorito.numero === numero);
  }

  protected interfazGuardada(name: string): boolean {
    return this.interfaces().some(interfaz => interfaz.name === name);
  }

  protected previewUrl(model: NextionModel): string {
    return new URL(model.preview_url, document.baseURI).toString();
  }

  protected abrirVistaPrevia(model: NextionModel): void {
    this.vistaPrevia.set(model);
  }

  protected cerrarVistaPrevia(): void {
    this.vistaPrevia.set(null);
  }

  @HostListener('document:keydown.escape')
  protected cerrarVistaPreviaConEscape(): void {
    if (this.vistaPrevia()) {
      this.cerrarVistaPrevia();
    }
  }

  private aplicarEstado(status: NextionStatus): void {
    this.habilitado = status.settings.enabled;
    this.puerto = status.settings.port;
    this.velocidad = status.settings.baud_rate;
    this.interfaz = status.settings.interface;
    this.nodoFavorito = status.settings.favorite_node;
    this.puertos.set(status.ports);
    this.interfaces.set(status.interfaces);
    this.modelos.set(status.models);
    this.runtime.set(status.runtime);
    this.nodoLocal.set(status.local_node);
    this.ipv4Actual.set(status.current_ipv4 || '--');
    this.amiConfigurado.set(status.ami_configured);
    this.cambios.reset(this.datosFormulario());
  }

  private mensajeError(error: HttpErrorResponse, fallback: string): string {
    const body = error.error as HttpErrorBody | null;
    return body?.message ?? fallback;
  }
}
