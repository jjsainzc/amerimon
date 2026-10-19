import { Component, EventEmitter, HostListener, Input, OnChanges, Output, SimpleChanges, inject, signal } from '@angular/core';
import { DOCUMENT } from '@angular/common';
import { HttpClient } from '@angular/common/http';
import { DomSanitizer, SafeHtml } from '@angular/platform-browser';
import { firstValueFrom } from 'rxjs';
import DOMPurify from 'dompurify';
import { marked } from 'marked';
import mermaid from 'mermaid';

@Component({
  selector: 'app-markdown-viewer',
  standalone: true,
  templateUrl: './markdown-viewer.component.html',
  styleUrl: './markdown-viewer.component.css'
})
export class MarkdownViewerComponent implements OnChanges {
  private static nextViewerId = 0;
  private readonly viewerId = ++MarkdownViewerComponent.nextViewerId;
  private readonly http = inject(HttpClient);
  private readonly document = inject(DOCUMENT);
  private readonly sanitizer = inject(DomSanitizer);
  private loadGeneration = 0;

  @Input() open = false;
  @Input({ required: true }) markdownUrl!: string;
  @Input() title = 'Documento';
  @Input() eyebrow = 'GUÍA';
  @Output() closed = new EventEmitter<void>();

  protected readonly loading = signal(false);
  protected readonly error = signal('');
  protected readonly html = signal<string | SafeHtml>('');

  ngOnChanges(changes: SimpleChanges): void {
    if ((changes['open']?.currentValue || (changes['markdownUrl'] && this.open))) {
      void this.loadMarkdown();
    }
  }

  protected close(): void {
    this.closed.emit();
  }

  protected async loadMarkdown(): Promise<void> {
    const generation = ++this.loadGeneration;
    this.loading.set(true);
    this.error.set('');
    this.html.set('');
    try {
      const url = new URL(this.markdownUrl, this.document.baseURI).toString();
      const markdown = await firstValueFrom(this.http.get(url, { responseType: 'text' }));
      const content = this.document.createElement('div');
      content.innerHTML = DOMPurify.sanitize(await marked.parse(markdown), {
        USE_PROFILES: { html: true }
      });
      const darkTheme = this.document.documentElement.getAttribute('data-tema') === 'oscuro';
      mermaid.initialize({
        startOnLoad: false,
        securityLevel: 'strict',
        theme: 'base',
        themeVariables: darkTheme ? {
          background: '#17242d',
          primaryColor: '#2b4a55',
          primaryTextColor: '#ffffff',
          primaryBorderColor: '#b5e5eb',
          nodeBkg: '#2b4a55',
          nodeBorder: '#b5e5eb',
          nodeTextColor: '#f8fcff',
          edgeLabelBackground: '#263641',
          secondaryColor: '#394c30',
          secondaryTextColor: '#f5ffe9',
          secondaryBorderColor: '#c5e2a6',
          tertiaryColor: '#514625',
          tertiaryTextColor: '#fff9e8',
          tertiaryBorderColor: '#ecd27a',
          lineColor: '#d8eaf0',
          textColor: '#f6fbfc',
          mainBkg: '#1d2a34',
          actorBkg: '#2b4a55',
          actorBorder: '#b5e5eb',
          actorTextColor: '#ffffff',
          actorLineColor: '#d8eaf0',
          signalColor: '#d8eaf0',
          signalTextColor: '#f6fbfc',
          labelBoxBkgColor: '#243943',
          labelBoxBorderColor: '#9ccbd4',
          labelTextColor: '#f6fbfc',
          loopTextColor: '#f6fbfc',
          noteBkgColor: '#4a4329',
          noteBorderColor: '#e4cf79',
          noteTextColor: '#fff9e8',
          activationBkgColor: '#315564',
          activationBorderColor: '#b5e5eb',
          sequenceNumberColor: '#f6fbfc'
        } : {
          background: '#ffffff',
          primaryColor: '#dceff1',
          primaryTextColor: '#102b33',
          primaryBorderColor: '#1c6673',
          nodeBkg: '#e4f2f3',
          nodeBorder: '#075b69',
          nodeTextColor: '#071a20',
          edgeLabelBackground: '#ffffff',
          secondaryColor: '#e7f0dc',
          secondaryTextColor: '#203018',
          secondaryBorderColor: '#4a682f',
          tertiaryColor: '#fbe8bc',
          tertiaryTextColor: '#30250e',
          tertiaryBorderColor: '#8b6418',
          lineColor: '#314b54',
          textColor: '#10242b',
          mainBkg: '#ffffff',
          actorBkg: '#dceff1',
          actorBorder: '#1c6673',
          actorTextColor: '#102b33',
          actorLineColor: '#314b54',
          signalColor: '#314b54',
          signalTextColor: '#10242b',
          labelBoxBkgColor: '#f4f8f8',
          labelBoxBorderColor: '#52717a',
          labelTextColor: '#10242b',
          loopTextColor: '#10242b',
          noteBkgColor: '#fff1cc',
          noteBorderColor: '#8b6418',
          noteTextColor: '#30250e',
          activationBkgColor: '#c9e1e5',
          activationBorderColor: '#1c6673',
          sequenceNumberColor: '#10242b'
        }
      });
      const diagrams = Array.from(content.querySelectorAll('pre > code.language-mermaid'));
      for (const [index, code] of diagrams.entries()) {
        try {
          const { svg } = await mermaid.render(`markdown-${this.viewerId}-diagram-${index}`, code.textContent ?? '');
          const diagram = this.document.createElement('div');
          diagram.className = 'markdown-viewer-diagram';
          if (index > 0) diagram.classList.add('markdown-viewer-sequence');
          diagram.innerHTML = svg;
          const svgElement = diagram.querySelector('svg');
          if (index === 0 && svgElement) {
            const width = svgElement.viewBox.baseVal.width;
            if (width > 0) {
              svgElement.style.width = `${Math.ceil(width)}px`;
              svgElement.style.maxWidth = 'none';
              svgElement.style.height = 'auto';
            }
            for (const label of svgElement.querySelectorAll<HTMLElement>('.nodeLabel p, .edgeLabel p')) {
              label.style.color = darkTheme ? '#f8fcff' : '#071a20';
              label.style.fontSize = '18px';
              label.style.fontWeight = '700';
            }
          }
          code.parentElement?.replaceWith(diagram);
        } catch {
          continue;
        }
      }
      if (generation === this.loadGeneration) {
        this.html.set(this.sanitizer.bypassSecurityTrustHtml(content.innerHTML));
      }
    } catch {
      if (generation === this.loadGeneration) this.error.set(`No se pudo cargar ${this.title}.`);
    } finally {
      if (generation === this.loadGeneration) this.loading.set(false);
    }
  }

  @HostListener('document:keydown.escape')
  closeOnEscape(): void {
    if (this.open) this.close();
  }
}
