import { TestBed } from '@angular/core/testing';
import { PasswordMeterComponent } from './password-meter.component';

describe('PasswordMeterComponent', () => {
  beforeEach(async () => {
    await TestBed.configureTestingModule({
      imports: [PasswordMeterComponent]
    }).compileComponents();
  });

  it('creates and evaluates the entered password', () => {
    const fixture = TestBed.createComponent(PasswordMeterComponent);
    const component = fixture.componentInstance;
    fixture.detectChanges();

    component.passwordControl.setValue('Abc123!@x');

    expect(component.metrics.details.length).toBe(9);
    expect(component.metrics.score).toBeGreaterThan(0);
  });

  it('allows a password to be revealed and hidden', () => {
    const fixture = TestBed.createComponent(PasswordMeterComponent);
    const component = fixture.componentInstance;

    component.toggleMask();
    expect(component.showPassword).toBe(true);

    component.toggleMask();
    expect(component.showPassword).toBe(false);
  });

  it('recommends reducing repetition and increasing diversity', () => {
    const fixture = TestBed.createComponent(PasswordMeterComponent);
    const component = fixture.componentInstance;
    fixture.detectChanges();

    component.passwordControl.setValue('aaaaaaa');

    expect(component.recommendations).toContain('PASSWORD_METER_RECOMMEND_LENGTH');
    expect(component.recommendations).toContain('PASSWORD_METER_RECOMMEND_DIVERSITY');
    expect(component.recommendations).toContain('PASSWORD_METER_RECOMMEND_REPETITION');
    expect(component.recommendations).toContain('PASSWORD_METER_RECOMMEND_CHARACTER_TYPES');
  });

  it('recommends against sequential patterns', () => {
    const fixture = TestBed.createComponent(PasswordMeterComponent);
    const component = fixture.componentInstance;
    fixture.detectChanges();

    component.passwordControl.setValue('abc123XYZ!');

    expect(component.recommendations).toContain('PASSWORD_METER_RECOMMEND_SEQUENCES');
    expect(component.recommendations).toContain('PASSWORD_METER_RECOMMEND_CONSECUTIVE');
  });
});
