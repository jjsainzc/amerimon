import { vi } from 'vitest';
import { NoticePreferencesService } from './notice-preferences.service';

describe('NoticePreferencesService', () => {
  beforeEach(() => localStorage.clear());
  afterEach(() => {
    vi.restoreAllMocks();
    localStorage.clear();
  });

  it('persists each notice type independently across service recreation', () => {
    const preferences = new NoticePreferencesService();
    preferences.hide('confirm.asterisk.connect');
    const restored = new NoticePreferencesService();
    expect(restored.isHidden('confirm.asterisk.connect')).toBe(true);
    expect(restored.isHidden('confirm.asterisk.disconnect')).toBe(false);
    expect(restored.isHidden('notice.asterisk.connected')).toBe(false);
    expect(restored.isHidden('')).toBe(false);
  });

  it('restores every notice without deleting other browser data', () => {
    localStorage.setItem('amerimon.admin.user', 'admin');
    const preferences = new NoticePreferencesService();
    preferences.hide('confirm.asterisk.connect');
    preferences.hide('notice.asterisk.connected');
    expect(preferences.reset()).toBe(true);
    expect(preferences.isHidden('confirm.asterisk.connect')).toBe(false);
    expect(preferences.isHidden('notice.asterisk.connected')).toBe(false);
    expect(new NoticePreferencesService().isHidden('confirm.asterisk.connect')).toBe(false);
    expect(localStorage.getItem('amerimon.admin.user')).toBe('admin');
  });

  it('reports corrupt preferences and keeps confirmations enabled', () => {
    const log = vi.spyOn(console, 'error').mockImplementation(() => {});
    localStorage.setItem('amerimon.notice-preferences.v1', '{"invalid":true}');
    const preferences = new NoticePreferencesService();
    expect(preferences.error()).toBe('NOTICE_PREFERENCES_ERROR');
    expect(preferences.isHidden('confirm.asterisk.connect')).toBe(false);
    expect(log).toHaveBeenCalledOnce();
  });

  it('reports failed storage writes without hiding notices in memory', () => {
    const log = vi.spyOn(console, 'error').mockImplementation(() => {});
    const preferences = new NoticePreferencesService();
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => { throw new Error('Storage blocked'); });
    preferences.hide('confirm.asterisk.connect');
    expect(preferences.isHidden('confirm.asterisk.connect')).toBe(false);
    expect(preferences.error()).toBe('NOTICE_PREFERENCES_ERROR');
    expect(log).toHaveBeenCalledOnce();
  });

  it('does not report a successful reset when storage removal fails', () => {
    vi.spyOn(console, 'error').mockImplementation(() => {});
    const preferences = new NoticePreferencesService();
    preferences.hide('confirm.asterisk.connect');
    vi.spyOn(Storage.prototype, 'removeItem').mockImplementation(() => { throw new Error('Storage blocked'); });
    expect(preferences.reset()).toBe(false);
    expect(preferences.isHidden('confirm.asterisk.connect')).toBe(true);
    expect(preferences.error()).toBe('NOTICE_PREFERENCES_ERROR');
  });
});
