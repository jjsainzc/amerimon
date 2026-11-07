import { DatePipe } from '@angular/common';
import { HttpErrorResponse } from '@angular/common/http';
import { Component, DestroyRef, OnInit, inject, signal } from '@angular/core';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { FormBuilder, ReactiveFormsModule, Validators } from '@angular/forms';
import { forkJoin } from 'rxjs';
import { NodeReconnection, NodeReconnectionInput, NodeReconnectionsService, ReconnectionAction } from '../../../core/services/node-reconnections.service';
import { TranslationService } from '../../../core/services/translation.service';
import { FormChanges } from '../../../core/utils/form-changes';
import { TranslatePipe } from '../../../core/pipes/translate.pipe';
import { AdminVolverComponent } from '../../../shared/admin-volver/admin-volver.component';
import { ConfirmacionDialogComponent } from '../../../shared/confirmacion-dialog/confirmacion-dialog.component';
import { CopyValueDirective } from '../../../shared/copy-value/copy-value.directive';
import { AllstarlinkService } from '../../../core/services/allstarlink.service';

@Component({
  selector: 'app-node-reconnections',
  imports: [ReactiveFormsModule, DatePipe, TranslatePipe, AdminVolverComponent, ConfirmacionDialogComponent, CopyValueDirective],
  templateUrl: './node-reconnections.component.html',
  styleUrl: './node-reconnections.component.css'
})
export class NodeReconnectionsComponent implements OnInit {
  private readonly service = inject(NodeReconnectionsService);
  private readonly allstarlink = inject(AllstarlinkService);
  private readonly translation = inject(TranslationService);
  private readonly destroyRef = inject(DestroyRef);
  private readonly fb = inject(FormBuilder);
  readonly entries = signal<NodeReconnection[]>([]);
  readonly sources = signal<{ value: string; label: string }[]>([]);
  readonly primary = signal('');
  readonly loading = signal(false);
  readonly busy = signal(false);
  readonly error = signal('');
  readonly notice = signal('');
  readonly editorOpen = signal(false);
  readonly editingID = signal<number | null>(null);
  readonly pendingDelete = signal<NodeReconnection | null>(null);
  readonly changes = new FormChanges();
  readonly actions: { value: ReconnectionAction; label: string }[] = [
    { value: 'connect', label: 'RECONNECT_TRANSCEIVER' },
    { value: 'monitor', label: 'RECONNECT_MONITOR' },
    { value: 'localmonitor', label: 'RECONNECT_LOCAL_MONITOR' }
  ];
  readonly form = this.fb.nonNullable.group({
    source_node: [''],
    node: ['', [Validators.required, Validators.pattern(/^[0-9]{1,10}$/)]],
    enabled: [false],
    interval_seconds: [60, [Validators.required, Validators.min(1), Validators.max(86400), Validators.pattern(/^[0-9]+$/)]],
    action: this.fb.nonNullable.control<ReconnectionAction>('connect'),
    permanent: [false]
  });

  ngOnInit(): void {
    this.load();
  }

  load(): void {
    if (this.loading() || this.busy()) { return; }
    this.loading.set(true);
    forkJoin({ list: this.service.listar(), servers: this.allstarlink.listarServidores() })
      .pipe(takeUntilDestroyed(this.destroyRef)).subscribe({
      next: ({ list, servers }) => {
        if (servers.status !== 'success' || !servers.data) {
          this.loading.set(false);
          this.sources.set([]);
          this.error.set(this.translation.translate('RECONNECT_DISCOVERY_ERROR'));
          return;
        }
        this.entries.set(list.entries);
        this.primary.set(servers.data.nodo_principal ?? '');
        this.sources.set((servers.data.tablas ?? []).map(table => ({
          value: table.nodo_local,
          label: table.descripcion ? `${table.nodo_local} — ${table.descripcion}` : table.nodo_local
        })));
        this.loading.set(false);
        this.error.set('');
      },
      error: (error: HttpErrorResponse) => {
        this.loading.set(false);
        this.sources.set([]);
        this.error.set(this.translation.translate('RECONNECT_LIST_LOAD_ERROR') + ' ' + this.errorMessage(error));
      }
    });
  }

  open(entry?: NodeReconnection): void {
    if (this.busy() || this.loading()) { return; }
    this.editingID.set(entry?.id ?? null);
    this.form.reset(entry ? {
      source_node: entry.source_node,
      node: entry.node, enabled: entry.enabled, interval_seconds: entry.interval_seconds,
      action: entry.action, permanent: entry.permanent
    } : { source_node: this.sources().some(source => source.value === this.primary()) ? this.primary() : '',
      node: '', enabled: false, interval_seconds: 60, action: 'connect', permanent: false });
    this.changes.reset(this.form.getRawValue());
    this.error.set('');
    this.notice.set('');
    this.editorOpen.set(true);
  }

  close(): void {
    if (!this.busy()) { this.editorOpen.set(false); }
  }

  save(): void {
    this.form.markAllAsTouched();
    if (this.form.invalid || this.busy() || !this.changes.changed(this.form.getRawValue())) { return; }
    if (!this.sourceAvailable(this.form.controls.source_node.value)) {
      this.error.set(this.translation.translate('RECONNECT_SOURCE_SAVE_ERROR', {
        source: this.form.controls.source_node.value || this.primary(),
        destination: this.form.controls.node.value
      }));
      return;
    }
    if (this.form.controls.node.value === (this.form.controls.source_node.value || this.primary())) {
      this.error.set(this.translation.translate('RECONNECT_SELF'));
      return;
    }
    this.persist(this.form.getRawValue(), this.editingID(), true);
  }

  toggle(entry: NodeReconnection): void {
    if (this.busy() || this.loading() || this.editorOpen()) { return; }
    this.persist({
      source_node: entry.source_node,
      node: entry.node, enabled: !entry.enabled, interval_seconds: entry.interval_seconds,
      action: entry.action, permanent: entry.permanent
    }, entry.id, false);
  }

  private persist(input: NodeReconnectionInput, id: number | null, close: boolean): void {
    this.busy.set(true);
    this.error.set('');
    this.notice.set('');
    this.service.guardar(input, id).pipe(takeUntilDestroyed(this.destroyRef)).subscribe({
      next: () => {
        this.busy.set(false);
        if (close) { this.editorOpen.set(false); }
        this.notice.set('RECONNECT_SAVED');
        this.load();
      },
      error: (error: HttpErrorResponse) => {
        this.busy.set(false);
        this.error.set(this.errorMessage(error));
      }
    });
  }

  delete(): void {
    const entry = this.pendingDelete();
    if (!entry || this.busy()) { return; }
    this.busy.set(true);
    this.error.set('');
    this.notice.set('');
    this.service.eliminar(entry.id).pipe(takeUntilDestroyed(this.destroyRef)).subscribe({
      next: () => {
        this.busy.set(false);
        this.pendingDelete.set(null);
        this.notice.set('RECONNECT_DELETED');
        this.load();
      },
      error: (error: HttpErrorResponse) => {
        this.busy.set(false);
        this.error.set(this.errorMessage(error));
      }
    });
  }

  actionLabel(action: ReconnectionAction): string {
    const labels: Record<ReconnectionAction, string> = {
      connect: 'RECONNECT_TRANSCEIVER', monitor: 'RECONNECT_MONITOR', localmonitor: 'RECONNECT_LOCAL_MONITOR'
    };
    return labels[action];
  }

  sourceAvailable(source: string): boolean {
    return this.sources().some(item => item.value === (source || this.primary()));
  }

  private errorMessage(error: HttpErrorResponse): string {
    return this.translation.translate(error.error?.message ?? 'RECONNECT_DB_ERROR', {
      source: error.error?.source_node ?? '',
      destination: error.error?.destination_node ?? ''
    });
  }
}
