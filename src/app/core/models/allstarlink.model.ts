export interface AllstarlinkNode {
  protocolo: string;
  categoria?: string;
  nodo_local: string;
  nodo: string;
  informacion?: string;
  contacto?: string;
  callsign?: string;
  descripcion?: string;
  ubicacion?: string;
  recibido?: string;
  enlace?: string;
  direccion?: string;
  conectado?: string;
  modo?: string;
  modo_codigo?: string;
  latencia?: string;
  pendiente?: boolean;
  permanente?: boolean;
}

export interface AllstarlinkData {
  count: number;
  resultados: AllstarlinkNode[];
  tablas?: AllstarlinkTabla[];
  nodo_principal?: string;
}

export interface AllstarlinkTabla {
  nodo_local: string;
  descripcion?: string;
  ip?: string;
  hostname?: string;
  servidor_conectado: boolean;
  nodos_disponibles: { value: string; label: string }[];
}

export interface AllstarlinkResponse {
  status: string;
  message?: string;
  data?: AllstarlinkData;
}

export interface AllstarlinkActivity {
  local_node: string;
  remote_node: string;
  ultima_actividad: string;
}

export interface AllstarlinkActivityResponse {
  status: string;
  message?: string;
  data?: AllstarlinkActivity[];
}

export interface AllstarlinkConnectionHistory {
  id: number;
  instance_id: string;
  local_node: string;
  remote_node: string;
  connection_type: string;
  remote_ip?: string;
  remote_callsign?: string;
  remote_description?: string;
  direction?: string;
  mode?: string;
  link_state?: string;
  first_seen_at: string;
  last_seen_at: string;
  disconnected_at?: string;
  disconnect_reason?: string;
}

export interface AllstarlinkConnectionHistoryFilters {
  local_node?: string;
  remote_node?: string;
  connection_type?: 'iax' | 'echolink';
  status?: 'active' | 'disconnected' | 'all';
  from?: string;
  to?: string;
  limit?: number;
}

export interface AllstarlinkConnectionHistoryResponse {
  status: string;
  message?: string;
  data?: AllstarlinkConnectionHistory[];
}

export interface AllstarlinkGrupo {
  nodoLocal: string;
  detalle: string;
  nodos: AllstarlinkNode[];
  conectados: number;
  servidorConectado: boolean;
}

export interface SupermonCommand {
  id: string;
  label: string;
  command_template: string;
  permanent_command_template?: string;
  requires_remote_node: boolean;
  supports_permanent: boolean;
}

export interface SupermonCommandsResponse {
  status: string;
  message?: string;
  data?: SupermonCommand[];
}

export interface SupermonCommandRequest {
  local_node: string;
  remote_node?: string;
  action: string;
  permanent: boolean;
  timeout_seconds?: number;
}

export interface SupermonCommandResult {
  action: string;
  command: string;
  lines: string[];
}

export interface SupermonCommandResponse {
  status: string;
  message?: string;
  data?: SupermonCommandResult;
}
