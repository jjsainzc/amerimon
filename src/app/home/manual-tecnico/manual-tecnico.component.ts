import { Component, inject } from '@angular/core';
import { Router } from '@angular/router';
import { TranslationService } from '../../core/services/translation.service';
import { TranslatePipe } from '../../core/pipes/translate.pipe';
import { MarkdownViewerComponent } from '../../shared/markdown-viewer/markdown-viewer.component';

@Component({
  selector: 'app-manual-tecnico',
  standalone: true,
  imports: [MarkdownViewerComponent, TranslatePipe],
  template: `
    <app-markdown-viewer
      [open]="true"
      presentation="page"
      [markdownUrl]="translationService.currentLang() === 'en' ? 'assets/docs/technical-operations-manual.en.md' : 'assets/docs/technical-operations-manual.es.md'"
      [title]="'MANUAL_TECNICO' | translate"
      [eyebrow]="'DOCUMENTACION' | translate"
      (closed)="volver()" />
  `,
})
export class ManualTecnicoComponent {
  protected readonly translationService = inject(TranslationService);
  private readonly router = inject(Router);

  protected volver(): void {
    void this.router.navigateByUrl('/allstarlink');
  }
}
