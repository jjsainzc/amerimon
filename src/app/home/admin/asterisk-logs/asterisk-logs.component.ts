import { HttpErrorResponse } from '@angular/common/http';
import { Component, computed, inject, signal } from '@angular/core';
import { DatePipe } from '@angular/common';
import { AsteriskLogService, AsteriskOperationLog } from '../../../core/services/asterisk-log.service';
import { AdminVolverComponent } from '../../../shared/admin-volver/admin-volver.component';
import { TablePaginationComponent } from '../../../shared/table-pagination/table-pagination.component';

@Component({
  selector: 'app-asterisk-logs',
  standalone: true,
  imports: [DatePipe, AdminVolverComponent, TablePaginationComponent],
  templateUrl: './asterisk-logs.component.html',
  styleUrl: './asterisk-logs.component.css',
})
export class AsteriskLogsComponent {
  private readonly service = inject(AsteriskLogService);

  readonly registros = signal<AsteriskOperationLog[]>([]);
  readonly cargando = signal(false);
  readonly error = signal('');
  readonly busqueda = signal('');
  readonly pagina = signal(1);
  readonly itemsPorPagina = signal(25);
  readonly filtrados = computed(() => {
    const query = this.busqueda().trim().toLocaleLowerCase();
    if (!query) { return this.registros(); }
    return this.registros().filter(item => [
      item.action, item.local_node, item.remote_node, item.mode, item.command,
      item.result, item.details,
    ].some(value => value.toLocaleLowerCase().includes(query)));
  });
  readonly totalPaginas = computed(() => Math.max(1, Math.ceil(this.filtrados().length / this.itemsPorPagina())));
  readonly paginados = computed(() => {
    const offset = (this.pagina() - 1) * this.itemsPorPagina();
    return this.filtrados().slice(offset, offset + this.itemsPorPagina());
  });

  constructor() {
    this.cargar();
  }

  cargar(): void {
    this.cargando.set(true);
    this.error.set('');
    this.service.listar().subscribe({
      next: response => {
        this.registros.set(response.data ?? []);
        this.pagina.set(1);
        this.cargando.set(false);
      },
      error: (err: HttpErrorResponse) => {
        this.cargando.set(false);
        this.registros.set([]);
        this.error.set(err.error?.message ?? `No se pudo consultar el registro (${err.status}).`);
      },
    });
  }

  buscar(value: string): void {
    this.busqueda.set(value);
    this.pagina.set(1);
  }

  cambiarTamano(value: string): void {
    this.itemsPorPagina.set(Number(value));
    this.pagina.set(1);
  }

  etiquetaResultado(result: string): string {
    switch (result) {
      case 'success': return 'Correcto';
      case 'ami_error': return 'Error AMI';
      case 'not_confirmed': return 'Sin confirmar';
      case 'configuration_error': return 'Configuración';
      case 'invalid_request': return 'Solicitud inválida';
      default: return result;
    }
  }
}
