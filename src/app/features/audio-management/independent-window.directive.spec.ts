import { Component } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { vi } from 'vitest';
import { IndependentWindowDirective } from './independent-window.directive';

@Component({
  imports: [IndependentWindowDirective],
  template: '<button [independentWindow]="url" (windowBlocked)="blocked = true">Open</button>',
})
class WindowHost {
  url = 'admin/audio';
  blocked = false;
}

describe('Independent windows', () => {
  afterEach(() => vi.restoreAllMocks());

  it('opens the destination without changing the original page or retaining its opener', () => {
    const replace = vi.fn();
    const popup = { opener: window, location: { replace } };
    const open = vi.spyOn(window, 'open').mockReturnValue(popup as unknown as Window);
    const fixture = TestBed.createComponent(WindowHost);
    fixture.detectChanges();
    fixture.nativeElement.querySelector('button').click();
    expect(open).toHaveBeenCalledWith('about:blank', '_blank', expect.stringContaining('popup'));
    expect(popup.opener).toBeNull();
    expect(replace).toHaveBeenCalledWith(new URL('admin/audio', document.baseURI).href);
    fixture.destroy();
  });

  it('reports blocked popups and rejects destinations outside the application origin', () => {
    const open = vi.spyOn(window, 'open').mockReturnValue(null);
    const fixture = TestBed.createComponent(WindowHost);
    fixture.detectChanges();
    fixture.nativeElement.querySelector('button').click();
    expect(fixture.componentInstance.blocked).toBe(true);
    fixture.componentInstance.url = 'https://example.org/';
    fixture.componentInstance.blocked = false;
    fixture.detectChanges();
    fixture.nativeElement.querySelector('button').click();
    expect(fixture.componentInstance.blocked).toBe(true);
    expect(open).toHaveBeenCalledOnce();
    fixture.destroy();
  });
});
