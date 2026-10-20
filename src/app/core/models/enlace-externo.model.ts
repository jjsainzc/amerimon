export interface EnlaceExterno {
  dashid: number;
  descripcion: string | null;
  url: string | null;
  target: string;
  is_active: number;
}

export type EnlaceExternoEntrada = Pick<EnlaceExterno, 'descripcion' | 'url' | 'target' | 'is_active'>;
