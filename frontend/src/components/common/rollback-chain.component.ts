import { CommonModule } from '@angular/common';
import { ChangeDetectorRef, Component, Input, OnInit } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { MatButtonModule } from '@angular/material/button';
import { finalizeDecisionRollback } from '../../api/review';
import { request } from '../../api/client';
import { authState } from '../../hooks/use-auth';
import type { DomainRecord } from '../../types/domain';
import { statusLabel } from '../../types/status';
import { formatDate } from '../../utils/format';
import { StatusBadgeComponent } from './status-badge.component';

interface SubstituteOption { id: number; code: string; effectiveAt: string; unitCode?: string }

// RollbackChainComponent renders the 回退链路 on a compliance decision row:
// which sample was voided and why, the preserved prior conclusion, and (for a
// reviewer) the substitute-sample final review form. Operators only see the
// waiting state.
@Component({
  selector: 'app-rollback-chain',
  standalone: true,
  imports: [CommonModule, FormsModule, MatButtonModule, StatusBadgeComponent],
  template: `<div class="rollback-chain" *ngIf="record.rollback as rb">
    <div class="rollback-head">
      <app-status-badge [status]="record.status"/>
      <span class="muted">原结论 {{ statusLabel(rb.previousState) }} · v{{ rb.previousVersion }}</span>
    </div>
    <p class="rollback-line">引用样本 <strong>{{ rb.voidedSampleCode }}</strong> 已作废：{{ rb.voidReason }}</p>
    <p class="muted">作废时间 {{ formatDate(rb.voidedAt) }} · 回退人 {{ rb.rolledBackBy }}</p>

    <ng-container *ngIf="rb.finalizedAt; else openChain">
      <p class="rollback-done">已使用替代样本 <strong>{{ rb.substituteCode }}</strong> 终审为
        <strong>{{ statusLabel(rb.finalState || record.status) }}</strong> · {{ rb.finalizedBy }} · {{ formatDate(rb.finalizedAt) }}</p>
    </ng-container>

    <ng-template #openChain>
      <p class="rollback-wait" *ngIf="!canReview()">等待复核员使用替代样本终审，操作员不能终审。</p>
      <div class="rollback-form" *ngIf="canReview()">
        <label>替代样本（同装置、已验证、采样晚于作废时间）
          <select [(ngModel)]="substituteId">
            <option [ngValue]="null" disabled>请选择替代样本</option>
            <option *ngFor="let option of options" [ngValue]="option.id">
              {{ option.code }} · {{ option.unitCode }} · 采样 {{ formatDate(option.effectiveAt) }}
            </option>
          </select>
        </label>
        <label>终审理由
          <input matInput [(ngModel)]="reason" placeholder="说明替代样本如何支持结论"/>
        </label>
        <button mat-flat-button color="primary" [disabled]="busy || !substituteId || !reason.trim()" (click)="finalize()">
          {{ busy ? '正在终审…' : '使用替代样本终审' }}
        </button>
        <small *ngIf="!options.length" class="rollback-warn">没有符合条件的替代样本（须同装置、已验证且采样晚于作废时间）。</small>
      </div>
    </ng-template>
    <div *ngIf="error" class="alert alert--inline">{{ error }}</div>
  </div>`,
})
export class RollbackChainComponent implements OnInit {
  @Input({ required: true }) record!: DomainRecord;
  readonly formatDate = formatDate;
  readonly statusLabel = statusLabel;
  options: SubstituteOption[] = [];
  substituteId: number | null = null;
  reason = '';
  busy = false;
  error = '';

  constructor(private readonly changeDetector: ChangeDetectorRef) {}

  canReview() { return authState.hasMinimumRole('reviewer'); }

  async ngOnInit() {
    const rb = this.record.rollback;
    if (!rb || rb.finalizedAt) return;
    await this.loadOptions(rb.voidedAt);
    this.changeDetector.detectChanges();
  }

  private async loadOptions(voidedAt: string) {
    try {
      const result = await request<DomainRecord[]>('/samples?page=1&pageSize=100&status=verified');
      this.options = result.data
        .filter((sample) => sample.unitCode === this.record.unitCode)
        .filter((sample) => new Date(sample.effectiveAt).getTime() > new Date(voidedAt).getTime())
        .map((sample) => ({ id: sample.id, code: sample.code, effectiveAt: sample.effectiveAt, unitCode: sample.unitCode }));
    } catch (err) {
      this.error = err instanceof Error ? err.message : String(err);
    }
  }

  async finalize() {
    if (!this.canReview() || !this.substituteId || !this.reason.trim()) return;
    this.busy = true;
    this.error = '';
    try {
      await finalizeDecisionRollback(this.record.id, this.substituteId, this.reason.trim());
      this.busy = false;
      this.changeDetector.detectChanges();
      window.dispatchEvent(new CustomEvent('rollback-changed'));
    } catch (err) {
      this.busy = false;
      this.error = err instanceof Error ? err.message : String(err);
      this.changeDetector.detectChanges();
    }
  }
}
