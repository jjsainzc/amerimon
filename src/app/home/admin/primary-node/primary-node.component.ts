import { HttpErrorResponse } from '@angular/common/http';
import { Component, DestroyRef, OnInit, inject, signal } from '@angular/core';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { FormsModule } from '@angular/forms';
import { CopyValueDirective } from '../../../shared/copy-value/copy-value.directive';
import { timeout } from 'rxjs';
import { NodeAccessDocument, NodeAccessEntry, PrimaryAMIConfig, PrimaryNodeService, isNodeAccessDocument } from '../../../core/services/primary-node.service';
import { AdminVolverComponent } from '../../../shared/admin-volver/admin-volver.component';
import { PasswordFieldComponent } from '../../../shared/password-field/password-field.component';
import { NetCalculatorPopupComponent } from '../../../shared/net-calculator-popup/net-calculator-popup.component';
import { ScrollTopComponent } from '../../../shared/scroll-top/scroll-top.component';
import { TranslatePipe } from '../../../core/pipes/translate.pipe';
import { TranslationService } from '../../../core/services/translation.service';
import { NoticePreferencesService } from '../../../core/services/notice-preferences.service';
import { NoticePreferenceComponent } from '../../../shared/notice-preference/notice-preference.component';
import { DismissibleNoticeComponent } from '../../../shared/dismissible-notice/dismissible-notice.component';
import { FormChanges } from '../../../core/utils/form-changes';



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
  imports: [FormsModule, AdminVolverComponent, PasswordFieldComponent,NetCalculatorPopupComponent, ScrollTopComponent, CopyValueDirective, TranslatePipe, NoticePreferenceComponent, DismissibleNoticeComponent],
  templateUrl: './primary-node.component.html',
  styleUrl: './primary-node.component.css'
})
export class PrimaryNodeComponent implements OnInit {
  private readonly service = inject(PrimaryNodeService);
  private readonly destroyRef = inject(DestroyRef);
  private readonly translation = inject(TranslationService);
  private readonly noticePreferences = inject(NoticePreferencesService);
  readonly noMostrarGuardado = signal(false);
  readonly noMostrarImportacion = signal(false);
  readonly noMostrarVaciado = signal(false);

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
  readonly popupCalculadoraAbierto = signal(false);

  numero = '';
  indicativo = '';
  password = '';
  private passwordOriginal = '';
  readonly cambios = new FormChanges();
  readonly cambiosAMI = new FormChanges();

  datosFormulario(): string[] {
    return [this.numero, this.indicativo, this.password];
  }

  datosAMI(): string[] {
    return [this.amiSecret(), this.amiBindAddress(), this.permitsNuevas().trim(), this.deniesNuevas().trim()];
  }
  tipo: NodeAccessEntry['type'] = 'allowlist';
  remoto = '';
  motivo = '';

  ngOnInit(): void {
    this.cargar();
    this.cargarAcceso();
    this.cargarAMI();
  }

  aplicarRedCalculada(resultado: string | null): void {
    if (resultado) {
      this.entradaRed.set(resultado);
      this.calcularRed(); // Opcional: actualiza la previsualización de red calculada de inmediato
    }
    this.popupCalculadoraAbierto.set(false);
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
        this.cambiosAMI.reset([config.secret, config.bind_address, '', '']);
        this.cargandoAMI.set(false);
      },
      error: (err: HttpErrorResponse) => {
        this.cargandoAMI.set(false);
        this.errorAMI.set(err.error?.message || this.translation.translate('PRIMARY_NODE_AMI_LOAD_FAILED'));
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
    if (this.guardandoAMI() || this.cargandoAMI() || !this.amiConfig() || !this.cambiosAMI.changed(this.datosAMI())) { return; }
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
        this.errorAMI.set(err.error?.message || this.translation.translate('PRIMARY_NODE_AMI_SAVE_UNCONFIRMED'));
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
        this.cambios.reset(this.datosFormulario());
        this.cargando.set(false);
      },
      error: (err: HttpErrorResponse) => {
        this.cargando.set(false);
        this.error.set(err.error?.message || this.translation.translate('PRIMARY_NODE_CONFIG_LOAD_FAILED'));
      }
    });
  }

  solicitarGuardado(): void {
    if (!this.cambios.changed(this.datosFormulario())) { return; }
    this.noMostrarGuardado.set(false);
    if (this.noticePreferences.isHidden('confirm.primary.save')) {
      this.guardar();
    } else {
      this.confirmando.set(true);
    }
  }

  guardar(): void {
    if (this.guardando() || this.guardandoAcceso() || !this.actual() || !this.cambios.changed(this.datosFormulario())) { return; }
    if (this.noMostrarGuardado()) { this.noticePreferences.hide('confirm.primary.save'); }
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
        this.error.set(err.error?.message || this.translation.translate('PRIMARY_NODE_SAVE_UNCONFIRMED'));
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
        this.errorAcceso.set(err.error?.message || this.translation.translate('PRIMARY_NODE_ACCESS_LOAD_FAILED'));
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
        this.avisoAcceso.set(this.translation.translate('PRIMARY_NODE_ACCESS_UPDATED'));
        if (!eliminar) { this.remoto = ''; this.motivo = ''; }
        this.cargarAcceso();
      },
      error: (err: HttpErrorResponse) => {
        this.guardandoAcceso.set(false);
        this.errorAcceso.set(err.error?.message || this.translation.translate('PRIMARY_NODE_ACCESS_CHANGE_UNCONFIRMED'));
      }
    });
  }

  solicitarVaciado(tipo: NodeAccessEntry['type']): void {
    if (this.guardandoAcceso() || this.guardando() || this.cargandoAcceso() || !this.nodoAcceso()) { return; }
    this.vaciadoPendiente.set({ tipo, nodo: this.nodoAcceso() });
    this.noMostrarVaciado.set(false);
    if (this.noticePreferences.isHidden(`confirm.primary.clear.${tipo}`)) { this.vaciarAcceso(); }
  }

  vaciarAcceso(): void {
    const pendiente = this.vaciadoPendiente();
    if (!pendiente || this.guardandoAcceso() || this.guardando() || this.cargandoAcceso()) { return; }
    if (this.noMostrarVaciado()) { this.noticePreferences.hide(`confirm.primary.clear.${pendiente.tipo}`); }
    this.vaciadoPendiente.set(null);
    this.guardandoAcceso.set(true);
    this.errorAcceso.set('');
    this.avisoAcceso.set('');
    this.service.vaciarAcceso(pendiente.nodo, pendiente.tipo).subscribe({
      next: () => {
        this.guardandoAcceso.set(false);
        this.avisoAcceso.set(this.translation.translate(
          pendiente.tipo === 'allowlist' ? 'PRIMARY_NODE_ALLOWED_LIST_CLEARED' : 'PRIMARY_NODE_BLOCKED_LIST_CLEARED'
        ));
        this.cargarAcceso();
      },
      error: (err: HttpErrorResponse) => {
        this.guardandoAcceso.set(false);
        this.errorAcceso.set(err.error?.message || this.translation.translate('PRIMARY_NODE_LIST_CLEAR_UNCONFIRMED'));
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
        this.avisoAcceso.set(this.translation.translate('PRIMARY_NODE_ACCESS_EXPORT_SUCCESS'));
      },
      error: (err: HttpErrorResponse) => {
        this.guardandoAcceso.set(false);
        this.errorAcceso.set(err.error?.message || this.translation.translate('PRIMARY_NODE_ACCESS_EXPORT_FAILED'));
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
      this.errorAcceso.set(this.translation.translate('PRIMARY_NODE_IMPORT_FILE_TOO_LARGE'));
      return;
    }
    const nodo = this.nodoAcceso();
    this.leyendoArchivo.set(true);
    try {
      const documento: unknown = JSON.parse(await archivo.text());
      if (!isNodeAccessDocument(documento)) {
        this.errorAcceso.set(this.translation.translate('PRIMARY_NODE_IMPORT_INVALID_FORMAT'));
        return;
      }
      this.importacionPendiente.set({ documento, nodo });
      this.noMostrarImportacion.set(false);
    } catch (err: unknown) {
      this.errorAcceso.set(this.translation.translate(
        err instanceof SyntaxError ? 'PRIMARY_NODE_IMPORT_INVALID_JSON' : 'PRIMARY_NODE_IMPORT_READ_FAILED'
      ));
    } finally {
      this.leyendoArchivo.set(false);
    }
    if (this.importacionPendiente() && this.noticePreferences.isHidden('confirm.primary.import')) { this.importarAcceso(); }
  }

  importarAcceso(): void {
    const pendiente = this.importacionPendiente();
    if (!pendiente || this.guardandoAcceso() || this.guardando() || this.cargandoAcceso() || this.leyendoArchivo()) { return; }
    if (this.noMostrarImportacion()) { this.noticePreferences.hide('confirm.primary.import'); }
    this.importacionPendiente.set(null);
    this.guardandoAcceso.set(true);
    this.errorAcceso.set('');
    this.avisoAcceso.set('');
    this.service.importarAcceso(pendiente.nodo, pendiente.documento).subscribe({
      next: respuesta => {
        this.guardandoAcceso.set(false);
        this.avisoAcceso.set(this.translation.translate('PRIMARY_NODE_IMPORT_SUCCESS', { cantidad: respuesta.imported }));
        this.cargarAcceso();
      },
      error: (err: HttpErrorResponse) => {
        this.guardandoAcceso.set(false);
        this.errorAcceso.set(err.error?.message || this.translation.translate('PRIMARY_NODE_IMPORT_UNCONFIRMED'));
      }
    });
  }
}
