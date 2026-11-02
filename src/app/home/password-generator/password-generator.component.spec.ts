import { TestBed } from '@angular/core/testing';
import { PasswordGeneratorComponent } from './password-generator.component';

describe('PasswordGeneratorComponent', () => {
  beforeEach(async () => {
    await TestBed.configureTestingModule({
      imports: [PasswordGeneratorComponent]
    }).compileComponents();
  });

  it('creates a password using the default options', () => {
    const fixture = TestBed.createComponent(PasswordGeneratorComponent);
    fixture.detectChanges();

    expect(fixture.componentInstance.generatedPassword).toHaveLength(12);
    expect(fixture.componentInstance.errorMessage).toBe('');
  });

  it('rejects a configuration with fewer than two character types', () => {
    const fixture = TestBed.createComponent(PasswordGeneratorComponent);
    fixture.detectChanges();
    const component = fixture.componentInstance;
    component.genForm.patchValue({
      useLower: true,
      useUpper: false,
      useNumbers: false,
      useSymbols: false
    });

    component.generate();

    expect(component.genForm.invalid).toBe(true);
    expect(component.generatedPassword).toBe('');
    expect(component.errorMessage).toBe('PASSWORD_TOOLS_ERROR_CRITERIA');
  });

  it('uses the C# default character groups when no options are selected', () => {
    const fixture = TestBed.createComponent(PasswordGeneratorComponent);
    fixture.detectChanges();
    const component = fixture.componentInstance;
    component.genForm.patchValue({
      useLower: false,
      useUpper: false,
      useNumbers: false,
      useSymbols: false
    });

    component.generate();

    expect(component.generatedPassword).toHaveLength(12);
    expect(component.generatedPassword).toMatch(/[a-z]/);
    expect(component.generatedPassword).toMatch(/[A-Z]/);
    expect(component.generatedPassword).toMatch(/[0-9]/);
    expect(component.generatedPassword).toMatch(/[^a-zA-Z0-9]/);
  });

  it('rejects lengths below the C# minimum', () => {
    const fixture = TestBed.createComponent(PasswordGeneratorComponent);
    fixture.detectChanges();
    const component = fixture.componentInstance;
    component.genForm.get('length')?.setValue(7);

    component.generate();

    expect(component.genForm.invalid).toBe(true);
    expect(component.generatedPassword).toBe('');
  });
});
