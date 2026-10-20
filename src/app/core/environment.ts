// Prefijo donde cuelga la app (p. ej. /amerimon detrás de un proxy), tomado de <base href>.
const base = new URL(document.baseURI).pathname.replace(/\/$/, '');

export const environment = {
  empresa: 'AMERIMON',
  apiUrl: `${base}/mon`,

  /** Construye la base WebSocket del mismo servidor y prefijo, usando WSS si la página usa HTTPS. */
  get wsUrl(): string {
    const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    return `${proto}//${window.location.host}${base}/mon`;
  },
};
