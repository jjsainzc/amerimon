import { Component, OnInit, inject, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { Router, RouterLink } from '@angular/router';
import { TranslatePipe } from '../../../core/pipes/translate.pipe';
import { SiteSettingsService } from '../../../core/services/site-settings.service';
import { NoticePreferencesService } from '../../../core/services/notice-preferences.service';

type ErrorHttp = { error?: { message?: string } };

@Component({
  selector: 'app-cabecera-admin',
  standalone: true,
  imports: [FormsModule, RouterLink, TranslatePipe],
  templateUrl: './cabecera-admin.component.html',
  styleUrl: './cabecera-admin.component.css',
})
export class CabeceraAdminComponent implements OnInit {
  private readonly settingsService = inject(SiteSettingsService);
  private readonly router = inject(Router);
  protected readonly noticePreferences = inject(NoticePreferencesService);
  protected readonly avisosRestablecidos = signal(false);

  protected titulo = '';
  protected subtitulo = '';
  protected ubicacionClima = '';
  protected readonly cargando = signal(true);
  protected readonly guardando = signal(false);
  protected readonly error = signal('');

  ngOnInit(): void {
    this.settingsService.obtener().subscribe({
      next: settings => {
        this.titulo = settings.title;
        this.subtitulo = settings.subtitle;
        this.ubicacionClima = settings.weather_location ?? '';
        this.cargando.set(false);
      },
      error: (error: ErrorHttp) => {
        this.error.set(error.error?.message ?? 'ERROR_COMPLETAR_OPERACION');
        this.cargando.set(false);
      },
    });
  }

  protected guardar(): void {
    this.guardando.set(true);
    this.error.set('');
    this.settingsService.guardar(this.titulo, this.subtitulo, this.ubicacionClima).subscribe({
      next: () => {
        this.guardando.set(false);
        void this.router.navigateByUrl('/allstarlink');
      },
      error: (error: ErrorHttp) => {
        this.error.set(error.error?.message ?? 'HEADER_SAVE_ERROR');
        this.guardando.set(false);
      },
    });
  }

  protected restablecerAvisos(): void {
    this.avisosRestablecidos.set(this.noticePreferences.reset());
  }
}
