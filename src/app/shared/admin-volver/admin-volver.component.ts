import { Component } from '@angular/core';
import { RouterLink } from '@angular/router';
import { TranslatePipe } from '../../core/pipes/translate.pipe';

@Component({
  selector: 'app-admin-volver',
  standalone: true,
  imports: [RouterLink, TranslatePipe],
  template: `
    <a class="admin-volver" routerLink="/allstarlink">
      <span aria-hidden="true">←</span> {{ 'VOLVER' | translate }}
    </a>
  `
})
export class AdminVolverComponent {}
