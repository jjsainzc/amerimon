import { Routes } from '@angular/router';
import { adminGuard } from './core/guards/admin.guard';

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
    loadComponent: () => import('./home/admin/cabecera-admin/cabecera-admin.component').then((m) => m.CabeceraAdminComponent),
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
