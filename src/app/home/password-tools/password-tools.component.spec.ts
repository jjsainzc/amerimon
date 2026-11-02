import { TestBed } from '@angular/core/testing';
import { By } from '@angular/platform-browser';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { provideHttpClient } from '@angular/common/http';
import { PasswordToolsComponent } from './password-tools.component';
import { PasswordGeneratorComponent } from '../password-generator/password-generator.component';
import { PasswordMeterComponent } from '../password-meter/password-meter.component';

describe('PasswordToolsComponent', () => {
  beforeEach(async () => {
    await TestBed.configureTestingModule({
      imports: [PasswordToolsComponent],
      providers: [provideHttpClient(), provideHttpClientTesting()]
    }).compileComponents();
  });

  it('shows both tools and evaluates the generated password in the meter', async () => {
    const fixture = TestBed.createComponent(PasswordToolsComponent);
    fixture.detectChanges();
    const http = TestBed.inject(HttpTestingController);
    http.expectOne('./assets/i18n/es.json').flush({});
    await fixture.whenStable();

    const generator = fixture.debugElement.query(By.directive(PasswordGeneratorComponent))
      .componentInstance as PasswordGeneratorComponent;
    const meter = fixture.debugElement.query(By.directive(PasswordMeterComponent))
      .componentInstance as PasswordMeterComponent;

    expect(generator.generatedPassword).toHaveLength(12);
    expect(meter.passwordControl.value).toBe(generator.generatedPassword);
    expect(meter.metrics.details.length).toBe(12);
  });
});
