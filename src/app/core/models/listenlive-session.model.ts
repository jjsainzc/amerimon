export interface ListenLiveSession {
  listenliveid: number;
  uuid?: string | null;
  userid: number | null;
  nombre: string | null;
  callsign: string | null;
  ip_origen: string | null;
  fecha_inicio: string;
  fecha_fin: string | null;
}

export interface ListenLiveSessionsResponse {
  status: string;
  data: ListenLiveSession[];
}

export interface ListenLiveSessionResponse {
  status: string;
  data: ListenLiveSession;
}
