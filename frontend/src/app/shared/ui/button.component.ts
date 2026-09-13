import { Component, Input } from '@angular/core';

@Component({
  selector: 'ui-button',
  standalone: true,
  template: `<button [class]="'btn ' + variant"><ng-content /></button>`,
  styles: [`.btn{padding:8px 14px;border-radius:8px;border:0;cursor:pointer;font-size:14px} .primary{background:#111827;color:#fff} .secondary{background:#f3f4f6;color:#111827}`],
})
export class UIButtonComponent {
  @Input() variant: 'primary' | 'secondary' = 'primary';
}
