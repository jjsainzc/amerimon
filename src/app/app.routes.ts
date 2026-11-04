import { inject } from '@angular/core';
import { Routes } from '@angular/router';
import { adminGuard } from './core/guards/admin.guard';
import { environment } from './core/environment';
import { TranslationService } from './core/services/translation.service';
import { AUDIO_MANAGEMENT_TRANSLATE } from './features/audio-management/audio-management.i18n';
import { provideAudioManagement } from './features/audio-management/audio-management.providers';

export const routes: Routes = [
  { path: '', pathMatch: 'full', redirectTo: 'allstarlink' },
  {
    path: 'allstarlink',
    loadComponent: () => import('./home/allstarlink/allstarlink.component').then((m) => m.AllstarlinkComponent),
  },
  {
    path: 'listen-live',
    loadComponent: () => import('./home/monitorcall/monitorcall.component').then((m) => m.MonitorCallComponent),
  },
  {
    path: 'password-tools',
    loadComponent: () => import('./home/password-tools/password-tools.component').then((m) => m.PasswordToolsComponent),
  },
  { path: 'password-meter', pathMatch: 'full', redirectTo: 'password-tools' },
  { path: 'password-generator', pathMatch: 'full', redirectTo: 'password-tools' },
  {
    path: 'admin/allstarlink-historial',
    canActivate: [adminGuard],
    loadComponent: () => import('./home/admin/allstarlink-historial/allstarlink-historial.component').then((m) => m.AllstarlinkHistorialComponent),
  },
  {
    path: 'admin/asterisk-logs',
    canActivate: [adminGuard],
    loadComponent: () => import('./home/admin/asterisk-logs/asterisk-logs.component').then((m) => m.AsteriskLogsComponent),
  },
  {
    path: 'admin/audio',
    canActivate: [adminGuard],
    providers: [
      provideAudioManagement(`${environment.apiUrl}/audio`),
      {
        provide: AUDIO_MANAGEMENT_TRANSLATE,
        useFactory: () => {
          const translations = inject(TranslationService);
          return (key: string) => {
            translations.revision();
            return translations.translate(key);
          };
        },
      },
    ],
    loadComponent: () => import('./features/audio-management/audio-management.component').then((m) => m.AudioManagementComponent),
  },
  {
    path: 'admin/allmon',
    canActivate: [adminGuard],
    loadComponent: () => import('./home/admin/allmon-config/allmon-config.component').then((m) => m.AllmonConfigComponent),
  },
  {
    path: 'admin/primary-node',
    canActivate: [adminGuard],
    loadComponent: () => import('./home/admin/primary-node/primary-node.component').then((m) => m.PrimaryNodeComponent),
  },
  {
    path: 'admin/terminal',
    canActivate: [adminGuard],
    loadComponent: () => import('./shared/terminal/terminal.component').then((m) => m.TerminalComponent),
  },
  {
    path: 'admin/private-nodes',
    canActivate: [adminGuard],
    loadComponent: () => import('./home/admin/private-nodes/private-nodes.component').then((m) => m.PrivateNodesComponent),
  },
  {
    path: 'admin/enlaces-externos',
    canActivate: [adminGuard],
    loadComponent: () => import('./home/admin/enlaces-externos/enlaces-externos-admin.component').then(m => m.EnlacesExternosAdminComponent),
  },
  {
    path: 'admin/favoritos',
    canActivate: [adminGuard],
    loadComponent: () => import('./home/admin/favoritos-admin/favoritos-admin.component').then((m) => m.FavoritosAdminComponent),
  },
  {
    path: 'admin/perfil',
    canActivate: [adminGuard],
    loadComponent: () => import('./home/admin/perfil-admin/perfil-admin.component').then((m) => m.PerfilAdminComponent),
  },
  { path: 'admin/cabecera', pathMatch: 'full', redirectTo: 'admin/perfil' },
  {
    path: 'admin/cambiar-contrasena',
    canActivate: [adminGuard],
    loadComponent: () => import('./home/admin/cambiar-contrasena/cambiar-contrasena.component').then((m) => m.CambiarContrasenaComponent),
  },
  {
    path: 'admin/pjsip-users',
    canActivate: [adminGuard],
    loadComponent: () => import('./home/admin/pjsip-users/pjsip-users.component').then((m) => m.PjsipUsersComponent),
  },
  {
    path: 'admin/iax-users',
    canActivate: [adminGuard],
    loadComponent: () => import('./home/admin/iax-users/iax-users.component').then((m) => m.IaxUsersComponent),
  },

  {
    path: 'manual-tecnico',
    loadComponent: () => import('./home/manual-tecnico/manual-tecnico.component').then((m) => m.ManualTecnicoComponent),
  },
  { path: '**', redirectTo: 'allstarlink' },
];
