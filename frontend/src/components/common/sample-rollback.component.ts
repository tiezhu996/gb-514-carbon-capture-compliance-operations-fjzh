import { CommonModule } from '@angular/common';
import { ChangeDetectorRef, Component, Input, OnInit } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { MatButtonModule } from '@angular/material/button';
import { voidEmissionSample } from '../../api/review';
import { authState } from '../../hooks/use-auth';
import type { AffectedDecision, DomainRecord } from '../../types/domain';
import { statusLabel } from '../../types/status';
import { formatDate } from '../../utils/format';
import { StatusBadgeComponent } from './status-badge.component';

// SampleRollbackComponent renders the 作废 action for a verified/in-progress
// sample and, once voided, the list of affected accepted/escalated decisions.
// After voiding it emits rollback-changed so the table re-reads the chain.
@Component({
  selector: 'app-sample-rollback',
  standalone: true,
  imports: [CommonModule, FormsModule, MatButtonModule, StatusBadgeComponent],
  template: `<div class="sample-rollback">
    <ng-container *ngIf="record.status === 'invalid'; else voidForm">
      <div class="rollback-head"><app-status-badge [status]="record.status"/><span class="muted">{{ record.voidedBy }} · {{ formatDate(record.voidedAt || record.updatedAt) }}</span></div>
      <p class="rollback-line">作废原因：{{ record.voidedReason }}</p>
      <div class="affected" *ngIf="record.affectedDecisions?.length; else noAffected">
        <p class="affected-title">受影响的合规决定（{{ record.affectedDecisions?.length }}）</p>
        <ul>
          <li *ngFor="let decision of record.affectedDecisions">
            <strong>{{ decision.decisionCode }}</strong>
            <app-status-badge [status]="decision.state"/>
            <span class="muted">原结论 {{ statusLabel(decision.previousState) }}</span>
            <span class="muted" *ngIf="decision.finalizedAt">→ 替代样本 {{ decision.substituteCode }} · {{ formatDate(decision.finalizedAt) }}</span>
            <span class="muted" *ngIf="!decision.finalizedAt">回退于 {{ formatDate(decision.rolledBackAt) }}，待终审</span>
          </li>
        </ul>
      </div>
      <ng-template #noAffected><p class="muted">没有已接受或已升级的决定引用此样本。</p></ng-template>
    </ng-container>

    <ng-template #voidForm>
      <ng-container *ngIf="canWrite()">
        <input matInput [(ngModel)]="reason" placeholder="作废原因（必填）" class="void-input"/>
        <button mat-stroked-button color="warn" [disabled]="busy || !reason.trim()" (click)="void()">
          {{ busy ? '正在作废…' : '作废样本' }}
        </button>
        <small class="rollback-warn">作废将回退引用它的已接受/升级决定。</small>
      </ng-container>
      <span class="muted" *ngIf="!canWrite()">只读权限</span>
    </ng-template>
    <div *ngIf="error" class="alert alert--inline">{{ error }}</div>
  </div>`,
})
export class SampleRollbackComponent implements OnInit {
  @Input({ required: true }) record!: DomainRecord;
  readonly formatDate = formatDate;
  readonly statusLabel = statusLabel;
  reason = '';
  busy = false;
  error = '';

  constructor(private readonly changeDetector: ChangeDetectorRef) {}

  canWrite() { return authState.hasMinimumRole('operator'); }

  async ngOnInit() { this.changeDetector.detectChanges(); }

  async void() {
    if (!this.canWrite() || !this.reason.trim()) return;
    this.busy = true;
    this.error = '';
    try {
      await voidEmissionSample(this.record.id, this.reason.trim());
      this.busy = false;
      this.changeDetector.detectChanges();
      window.dispatchEvent(new CustomEvent('rollback-changed'));
    } catch (err) {
      this.busy = false;
      this.error = err instanceof Error ? err.message : String(err);
      this.changeDetector.detectChanges();
    }
  }

  static decisions(record: DomainRecord): AffectedDecision[] {
    return record.affectedDecisions || [];
  }
}
