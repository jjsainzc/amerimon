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
    path: 'admin/allmon',
    canActivate: [adminGuard],
    loadComponent: () => import('./home/admin/allmon-config/allmon-config.component').then((m) => m.AllmonConfigComponent),
  },
  {
    path: 'admin/favoritos',
    canActivate: [adminGuard],
    loadComponent: () => import('./home/admin/favoritos-admin/favoritos-admin.component').then((m) => m.FavoritosAdminComponent),
  },
  { path: '**', redirectTo: 'allstarlink' },
];
