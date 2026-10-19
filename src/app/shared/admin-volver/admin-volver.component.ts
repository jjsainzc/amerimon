import { Component } from '@angular/core';
import { RouterLink } from '@angular/router';

@Component({
  selector: 'app-admin-volver',
  standalone: true,
  imports: [RouterLink],
  template: `
    <a class="admin-volver" routerLink="/allstarlink">
      <span aria-hidden="true">←</span> Volver
    </a>
  `
})
export class AdminVolverComponent {}
