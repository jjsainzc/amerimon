export type AudioPlaybackMode = 'broadcast_once' | 'broadcast_silence';

export interface AudioTrack {
  id: number;
  name: string;
  original_name: string;
  playback_mode: AudioPlaybackMode;
  enabled: boolean;
  file_size: number;
  duration_ms: number;
  sample_rate: number;
  bits_per_sample: number;
  interval_minutes?: number;
  last_played_at?: string | null;
  next_play_at?: string | null;
  created_at: string;
}

export interface AudioTrackUpload {
  name: string;
  playbackMode: AudioPlaybackMode;
  intervalMinutes: number;
  file: File;
}
