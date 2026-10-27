import { HttpErrorResponse } from '@angular/common/http';
import { Component, OnInit, inject, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { CopyValueDirective } from '../../../shared/copy-value/copy-value.directive';
import { AllmonConfigService, AllmonNodoEntrada } from '../../../core/services/allmon-config.service';
import { AdminVolverComponent } from '../../../shared/admin-volver/admin-volver.component';
import { PasswordFieldComponent } from '../../../shared/password-field/password-field.component';
import { TranslatePipe } from '../../../core/pipes/translate.pipe';

interface Fila extends AllmonNodoEntrada {
  passwdSet: boolean;
}

@Component({
  selector: 'app-allmon-config',
  standalone: true,
  imports: [FormsModule, AdminVolverComponent, PasswordFieldComponent, CopyValueDirective, TranslatePipe],
  templateUrl: './allmon-config.component.html',
  styleUrl: './allmon-config.component.css'
})
export class AllmonConfigComponent implements OnInit {
  private readonly service = inject(AllmonConfigService);

  readonly filas = signal<Fila[]>([]);
  readonly ruta = signal('');
  readonly escribible = signal(true);
  readonly cargando = signal(false);
  readonly guardando = signal(false);
  readonly error = signal('');
  readonly aviso = signal('');

  /** Carga la configuración de nodos de Allmon al iniciar la pantalla. */
  ngOnInit(): void {
    this.cargar();
  }

  /** Recupera nodos y metadatos del archivo para presentarlos en el editor. */
  cargar(): void {
    this.cargando.set(true);
    this.error.set('');
    this.service.obtener().subscribe({
      next: r => {
        this.cargando.set(false);
        this.ruta.set(r.path);
        this.escribible.set(r.writable);
        this.filas.set(r.nodes.map(n => ({
          node: n.node,
          host: n.host,
          port: n.port,
          user: n.user,
          passwd: n.passwd ?? '',
          passwdSet: n.passwd_set,
          tracking_enabled: n.tracking_enabled,
        })));
      },
      error: (e: HttpErrorResponse) => {
        this.cargando.set(false);
        this.error.set(e.error?.message ?? 'ALLMON_CONFIG_LOAD_ERROR');
      }
    });
  }

  /** Añade una fila con valores iniciales para configurar un nodo Allmon. */
  agregar(): void {
    this.filas.update(f => [...f, {
      node: '', host: '127.0.0.1', port: '5038', user: '', passwd: '',
      passwdSet: false, tracking_enabled: true,
    }]);
  }

  /** Quita de la edición la fila seleccionada. */
  quitar(i: number): void {
    this.filas.update(f => f.filter((_, idx) => idx !== i));
  }

  /** Persiste las filas sin el indicador interno de contraseña y recarga el estado. */
  guardar(): void {
    this.error.set('');
    this.aviso.set('');
    this.guardando.set(true);
    this.service.guardar(this.filas().map(({ passwdSet, ...n }) => n)).subscribe({
      next: r => {
        this.guardando.set(false);
        this.aviso.set('ALLMON_CONFIG_SAVED');
        this.cargar();
      },
      error: (e: HttpErrorResponse) => {
        this.guardando.set(false);
        this.error.set(e.error?.message ?? 'ALLMON_CONFIG_SAVE_ERROR');
      }
    });
  }
}
