export interface Favorito {
  id: number;
  numero: string;
  descripcion: string;
  /** Texto del nodo en astdb.txt, si existe. */
  descripcion_astdb?: string;
}

export type FavoritoEntrada = Pick<Favorito, 'numero' | 'descripcion'>;

export interface AstdbCoincidencia {
  nodo: string;
  callsign: string;
  descripcion: string;
  ubicacion: string;
}
