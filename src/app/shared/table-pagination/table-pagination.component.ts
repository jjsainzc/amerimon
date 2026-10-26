import { Component, EventEmitter, Input, Output } from '@angular/core';
import { CommonModule } from '@angular/common';
import { TranslatePipe } from '../../core/pipes/translate.pipe';

@Component({
  selector: 'app-table-pagination',
  standalone: true,
  imports: [CommonModule, TranslatePipe],
  templateUrl: './table-pagination.component.html',
  styleUrls: ['./table-pagination.component.css']
})
export class TablePaginationComponent {
  @Input() currentPage = 1;
  @Input() totalPages = 1;
  @Input() placement: 'top' | 'bottom' = 'bottom';
  @Input() showPageSize = false;
  @Input() showSearch = false;
  @Input() pageSize = 25;
  @Input() searchText = '';
  @Input() searchPlaceholder = '';
  @Input() searchLabel = '';
  @Input() summaryText = '';
  readonly pageSizeOptions = [5, 10, 25, 50, 100] as const;
  @Input() hideNavigation = false;
  @Output() pageChange = new EventEmitter<number>();
  @Output() pageSizeChange = new EventEmitter<string>();
  @Output() searchChange = new EventEmitter<string>();
}
