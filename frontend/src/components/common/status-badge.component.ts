
import { Component, Input } from '@angular/core'; import { statusTone } from '../../utils/format'; import { statusLabel } from '../../types/status';
@Component({ selector: 'app-status-badge', standalone: true, template: `<span [class]="'status status--' + tone">{{ label }}</span>` })
export class StatusBadgeComponent { @Input({ required: true }) status = ''; get tone() { return statusTone(this.status); } get label() { return statusLabel(this.status); } }
