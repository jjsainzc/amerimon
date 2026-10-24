import { Component, effect, inject, input, output, signal } from '@angular/core';
import { CopyValueDirective } from '../copy-value/copy-value.directive';
import { Favorito } from '../../core/models/favorito.model';
import { FavoritosService } from '../../core/services/favoritos.service';

/** Lista los favoritos y devuelve el número del nodo elegido. */
@Component({
  selector: 'app-favoritos-popup',
  standalone: true,
  imports: [CopyValueDirective],
  templateUrl: './favoritos-popup.component.html',
  styleUrl: './favoritos-popup.component.css'
})
export class FavoritosPopupComponent {
  private readonly service = inject(FavoritosService);

  readonly abierto = input(false);
  readonly seleccionado = output<string>();
  readonly cerrado = output<void>();

  readonly favoritos = signal<Favorito[]>([]);
  readonly cargando = signal(false);
  readonly error = signal('');

  /** Recarga los favoritos cada vez que se abre el selector y muestra los errores de consulta. */
  constructor() {
    effect(() => {
      if (!this.abierto()) { return; }
      this.cargando.set(true);
      this.error.set('');
      this.service.listar().subscribe({
        next: items => {
          this.favoritos.set(Array.isArray(items) ? items : []);
          this.cargando.set(false);
        },
        error: err => {
          this.cargando.set(false);
          this.error.set(err?.error?.message || 'No se pudieron cargar los favoritos.');
        }
      });
    });
  }
}
