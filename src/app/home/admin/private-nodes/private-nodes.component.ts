import { HttpErrorResponse } from '@angular/common/http';
import { Component, OnInit, inject, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { CopyValueDirective } from '../../../shared/copy-value/copy-value.directive';
import { AdminVolverComponent } from '../../../shared/admin-volver/admin-volver.component';
import { PrivateNodesService, PrivateRPTNode } from '../../../core/services/private-nodes.service';
import { TranslationService } from '../../../core/services/translation.service';
import { TranslatePipe } from '../../../core/pipes/translate.pipe';
import { FormChanges } from '../../../core/utils/form-changes';
import { ConfirmacionDialogComponent } from '../../../shared/confirmacion-dialog/confirmacion-dialog.component';

@Component({
  selector: 'app-private-nodes',
  standalone: true,
  imports: [FormsModule, AdminVolverComponent, CopyValueDirective, TranslatePipe, ConfirmacionDialogComponent],
  templateUrl: './private-nodes.component.html',
  styleUrl: './private-nodes.component.css'
})
export class PrivateNodesComponent implements OnInit {
  private readonly service = inject(PrivateNodesService);
  private readonly translation = inject(TranslationService);
  readonly cambios = new FormChanges();
  readonly cambiosRPT = new FormChanges();
  readonly cambiosServidores = new FormChanges();

  readonly lineas = signal<string[]>([]);
  readonly ruta = signal('');
  readonly escribible = signal(false);
  readonly cargando = signal(false);
  readonly guardando = signal(false);
  readonly error = signal('');
  readonly aviso = signal('');
  readonly rptNodes = signal<PrivateRPTNode[]>([]);
  readonly rptPath = signal('');
  readonly rptTemplateSettings = signal<Record<string, string>>({});
  readonly rptTemplateDescriptions = signal<Record<string, string>>({});
  readonly cargandoRPT = signal(false);
  readonly guardandoRPT = signal(false);
  readonly errorRPT = signal('');
  readonly avisoRPT = signal('');
  readonly editorAbierto = signal(false);
  readonly editandoNodo = signal(false);
  readonly nodoActual = signal<PrivateRPTNode>(this.nuevoNodoVacio());
  readonly parametroKey = signal('');
  readonly parametroValue = signal('');
  readonly parametroSeleccionado = signal('');
  readonly parametroPersonalizado = signal(false);
  readonly pendienteEliminar = signal<string | null>(null);

  /** Carga al iniciar el componente las líneas y permisos del archivo de nodos. */
  ngOnInit(): void {
    this.cargar();
    this.cargarRPTNodes();
  }

  cargarRPTNodes(): void {
    this.cargandoRPT.set(true);
    this.errorRPT.set('');
    this.service.obtenerRPT().subscribe({
      next: respuesta => {
        this.rptNodes.set(respuesta.nodes);
        this.cambiosServidores.reset(respuesta.nodes);
        this.rptPath.set(respuesta.path);
        this.rptTemplateSettings.set(respuesta.template_settings ?? {});
        this.rptTemplateDescriptions.set(respuesta.template_descriptions ?? {});
        this.cargandoRPT.set(false);
      },
      error: (error: HttpErrorResponse) => {
        this.cargandoRPT.set(false);
        this.errorRPT.set(error.error?.message ?? this.translation.translate('NO_SE_PUDO_CARGAR_NODOS_RPT'));
      }
    });
  }

  /** Lee el archivo y actualiza su ruta, permisos, contenido y estado de error. */
  cargar(): void {
    this.cargando.set(true);
    this.error.set('');
    this.service.obtener().subscribe({
      next: respuesta => {
        this.cargando.set(false);
        this.ruta.set(respuesta.path);
        this.escribible.set(respuesta.writable);
        this.lineas.set(respuesta.lines);
        this.cambios.reset(this.lineas());
      },
      error: (error: HttpErrorResponse) => {
        this.cargando.set(false);
        this.error.set(error.error?.message ?? this.translation.translate('NO_SE_PUDO_CARGAR_ARCHIVO_NODOS_PRIVADOS'));
      }
    });
  }

  /** Añade una línea vacía para introducir otro nodo privado. */
  agregar(): void {
    this.lineas.update(lineas => [...lineas, '']);
  }

  /** Sustituye el contenido de una línea sin alterar las demás. */
  actualizar(indice: number, valor: string): void {
    this.lineas.update(lineas => lineas.map((linea, actual) => actual === indice ? valor : linea));
  }

  /** Elimina la línea indicada de la configuración en edición. */
  quitar(indice: number): void {
    this.lineas.update(lineas => lineas.filter((_, actual) => actual !== indice));
  }

  /** Guarda las líneas editadas y vuelve a cargar el archivo confirmado. */
  guardar(): void {
    if (this.guardando() || this.cargando() || !this.cambios.changed(this.lineas())) { return; }
    this.error.set('');
    this.aviso.set('');
    this.guardando.set(true);
    this.service.guardar(this.lineas()).subscribe({
      next: () => {
        this.guardando.set(false);
        this.aviso.set(this.translation.translate('ARCHIVO_NODOS_PRIVADOS_GUARDADO'));
        this.cargar();
      },
      error: (error: HttpErrorResponse) => {
        this.guardando.set(false);
        this.error.set(error.error?.message ?? this.translation.translate('NO_SE_PUDO_GUARDAR_ARCHIVO_NODOS_PRIVADOS'));
      }
    });
  }

  abrirNuevoNodo(): void {
    const node = this.nuevoNodoVacio();
    this.nodoActual.set(node);
    this.editandoNodo.set(false);
    this.cambiosRPT.reset(node);
    this.parametroKey.set('');
    this.parametroValue.set('');
    this.parametroSeleccionado.set('');
    this.parametroPersonalizado.set(false);
    this.errorRPT.set('');
    this.editorAbierto.set(true);
  }

  editarNodo(node: PrivateRPTNode): void {
    const draft = { ...node, settings: { ...node.settings } };
    this.nodoActual.set(draft);
    this.editandoNodo.set(true);
    this.cambiosRPT.reset(draft);
    this.parametroKey.set('');
    this.parametroValue.set('');
    this.parametroSeleccionado.set('');
    this.parametroPersonalizado.set(false);
    this.errorRPT.set('');
    this.editorAbierto.set(true);
  }

  cerrarEditor(): void {
    if (!this.guardandoRPT()) { this.editorAbierto.set(false); }
  }

  actualizarNodo(cambios: Partial<PrivateRPTNode>): void {
    this.nodoActual.update(node => ({ ...node, ...cambios }));
  }

  opcionesNodo(): { key: string; value: string }[] {
    return Object.entries(this.nodoActual().settings).map(([key, value]) => ({ key, value }));
  }

  actualizarParametro(key: string, value: string): void {
    this.nodoActual.update(node => ({ ...node, settings: { ...node.settings, [key]: value } }));
  }

  parametrosDisponibles(): { key: string; value: string }[] {
    const configured = new Set(Object.keys(this.nodoActual().settings).map(key => key.toLowerCase()));
    return Object.entries(this.rptTemplateSettings())
      .filter(([key]) => key.toLowerCase() !== 'rxchannel' && !configured.has(key.toLowerCase()))
      .map(([key, value]) => ({ key, value }))
      .sort((a, b) => a.key.localeCompare(b.key));
  }

  seleccionarParametro(key: string): void {
    this.parametroSeleccionado.set(key);
    this.parametroPersonalizado.set(key === '__custom__');
    this.parametroKey.set('');
    this.parametroValue.set(key && key !== '__custom__' ? this.rptTemplateSettings()[key] ?? '' : '');
    this.errorRPT.set('');
  }

  descripcionParametro(): string {
    const key = this.parametroPersonalizado() ? this.parametroKey().trim().toLowerCase() : this.parametroSeleccionado();
    return this.rptTemplateDescriptions()[key] ?? '';
  }

  agregarParametro(): void {
    const key = this.parametroPersonalizado() ? this.parametroKey().trim() : this.parametroSeleccionado();
    const value = this.parametroValue().trim();
    const normalizedKey = key.toLowerCase();
    if (!/^[A-Za-z0-9_]+$/.test(key) || !value || normalizedKey === 'rxchannel' ||
        Object.keys(this.nodoActual().settings).some(existing => existing.toLowerCase() === normalizedKey)) {
      this.errorRPT.set(this.translation.translate('PARAMETRO_RPT_INVALIDO_O_DUPLICADO'));
      return;
    }
    this.actualizarParametro(normalizedKey, value);
    this.parametroKey.set('');
    this.parametroValue.set('');
    this.parametroSeleccionado.set('');
    this.parametroPersonalizado.set(false);
    this.errorRPT.set('');
  }

  guardarNodo(): void {
    const node = this.nodoActual();
    if (this.guardandoRPT() || !this.cambiosRPT.changed(node)) { return; }
    if (!/^[0-9]{4}$/.test(node.id) || !node.rxchannel.trim()) {
      this.errorRPT.set(this.translation.translate('NODO_RPT_VALIDACION'));
      return;
    }
    if (!this.editandoNodo() && this.rptNodes().some(existing => existing.id === node.id)) {
      this.errorRPT.set(this.translation.translate('NODO_RPT_DUPLICADO'));
      return;
    }
    const nodes = this.rptNodes().filter(existing => existing.id !== node.id);
    nodes.push({ ...node, rxchannel: node.rxchannel.trim(), settings: { ...node.settings } });
    this.persistirNodosRPT(nodes);
  }

  solicitarEliminarNodo(id: string): void {
    this.pendienteEliminar.set(id);
  }

  cambiarServidor(id: string, visible: boolean): void {
    this.rptNodes.update(nodes => nodes.map(node => node.id === id ? { ...node, show_as_server: visible } : node));
  }

  guardarServidores(): void {
    if (this.guardandoRPT() || this.cargandoRPT() || this.editorAbierto() ||
        !this.cambiosServidores.changed(this.rptNodes())) { return; }
    this.persistirNodosRPT(this.rptNodes());
  }

  cancelarEliminacionNodo(): void {
    if (!this.guardandoRPT()) { this.pendienteEliminar.set(null); }
  }

  confirmarEliminacionNodo(): void {
    const id = this.pendienteEliminar();
    if (!id || this.guardandoRPT()) { return; }
    this.persistirNodosRPT(this.rptNodes().filter(node => node.id !== id), true);
  }

  private persistirNodosRPT(nodes: PrivateRPTNode[], deleting = false): void {
    this.errorRPT.set('');
    this.avisoRPT.set('');
    this.guardandoRPT.set(true);
    this.service.guardarRPT(nodes).subscribe({
      next: () => {
        this.guardandoRPT.set(false);
        this.pendienteEliminar.set(null);
        this.editorAbierto.set(false);
        this.avisoRPT.set(this.translation.translate(deleting ? 'NODO_RPT_ELIMINADO' : 'NODOS_RPT_GUARDADOS'));
        this.cargarRPTNodes();
      },
      error: (error: HttpErrorResponse) => {
        this.guardandoRPT.set(false);
        if (deleting) { this.pendienteEliminar.set(null); }
        this.errorRPT.set(this.translation.translate(error.error?.message ?? 'NO_SE_PUDO_GUARDAR_NODOS_RPT'));
      }
    });
  }

  private nuevoNodoVacio(): PrivateRPTNode {
    return {
      id: '',
      show_as_server: false,
      rxchannel: 'USRP/127.0.0.1:34001:32001',
      settings: {
        duplex: '0',
        hangtime: '0',
        althangtime: '0',
        holdofftelem: '1',
        telemdefault: '0',
        telemdynamic: '0',
        linktolink: 'no',
        nounkeyct: '1',
        totime: '180000'
      }
    };
  }
}
