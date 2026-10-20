import { inject } from '@angular/core';
import { CanActivateFn, Router } from '@angular/router';
import { AuthService } from '../services/auth.service';

/** Permite rutas administrativas solo con rol de administrador; redirige el resto a AllStarLink. */
export const adminGuard: CanActivateFn = () => {
  const auth = inject(AuthService);
  return auth.esAdmin() ? true : inject(Router).createUrlTree(['/allstarlink']);
};
