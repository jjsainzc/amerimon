export interface SystemInfo {
  os: SystemOS;
  cpu: SystemCPU;
  mem: SystemMemory;
  storage: SystemStorage[];
  network: SystemNetwork[];
  traffic: SystemTraffic;
  top_proc: SystemProcess[];
  temperature: SystemTemperature[];
  voltage: string;
  asterisk_version: string;
}

export interface SystemOS {
  hostname: string;
  platform: string;
  distribution: string;
  version: string;
  os_arch: string;
  kernel: string;
  uptime: string;
}

export interface SystemCPU {
  cores: number;
  logicals: number;
  model: string;
  speed: string;
  used: string;
  load1: number;
  load5: number;
  load15: number;
}

export interface SystemMemory {
  total: string;
  available: string;
  used: string;
  used_percent: string;
}

export interface SystemStorage {
  mountpoint: string;
  fs_type: string;
  total: string;
  free: string;
  used_percent: string;
  swap: string;
}

export interface SystemNetwork {
  device: string;
  mac_address: string;
  status: string;
  ip_address: string[];
}

export interface SystemTraffic {
  bytes_recv: number;
  bytes_sent: number;
}

export interface SystemProcess {
  PID: number;
  Nombre: string;
  CPUPercent: number;
}

export interface SystemTemperature {
  zona: string;
  tipo: string;
  celsius: number;
}
