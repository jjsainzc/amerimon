export class FormChanges {
  private original: string | undefined;

  reset(value: unknown): void {
    this.original = JSON.stringify(value);
  }

  changed(value: unknown): boolean {
    return this.original !== undefined && JSON.stringify(value) !== this.original;
  }
}
