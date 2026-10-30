import { FormChanges } from './form-changes';

describe('FormChanges', () => {
  it('disables saving until initial data has been loaded', () => {
    expect(new FormChanges().changed({ name: 'edited' })).toBe(false);
  });

  it('detects changes and restores the unchanged state when values are reverted', () => {
    const changes = new FormChanges();
    const value = { name: 'original', active: true, rows: ['first'], secret: 'secret' };
    changes.reset(value);
    expect(changes.changed(value)).toBe(false);
    value.name = 'edited';
    expect(changes.changed(value)).toBe(true);
    value.name = 'original';
    expect(changes.changed(value)).toBe(false);
    value.rows.push('second');
    expect(changes.changed(value)).toBe(true);
    value.rows.pop();
    expect(changes.changed(value)).toBe(false);
    value.active = false;
    expect(changes.changed(value)).toBe(true);
    value.active = true;
    value.secret = 'new-secret';
    expect(changes.changed(value)).toBe(true);
  });

  it('takes a new baseline after a save or reload', () => {
    const changes = new FormChanges();
    changes.reset(['old']);
    expect(changes.changed(['new'])).toBe(true);
    changes.reset(['new']);
    expect(changes.changed(['new'])).toBe(false);
  });
});
