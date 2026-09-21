import { CommonModule } from '@angular/common';
import { Component, Input } from '@angular/core';
import type { DomainRecord } from '../../types/domain';
import { formatDate } from '../../utils/format';

// AffectedDecisionsComponent is the 排放样本 page projection: every 合规决定
// referencing the sample and whether it was rolled back / re-finalised when
// the sample was invalidated.
@Component({
  selector: 'app-affected-decisions',
  standalone: true,
  imports: [CommonModule],
  template: `<section class="affected-panel" aria-label="受影响决定">
    <header><strong>受影响合规决定</strong><span>样本作废会自动回退引用它的已接受或升级决定</span></header>
    <div *ngIf="samples.length; else empty">
      <article *ngFor="let sample of samples" class="affected-group">
        <div class="sample-head">
          <strong>{{ sample.code }} · {{ sample.name }}</strong>
          <span class="status status--danger" *ngIf="sample.status === 'invalid'">已作废</span>
        </div>
        <p *ngIf="sample.status === 'invalid'" class="invalidation-reason">
          作废原因：{{ sample.invalidatedReason || '—' }} · {{ formatDate(sample.invalidatedAt) }}
        </p>
        <table *ngIf="sample.affectedDecisions && sample.affectedDecisions.length; else noDecision" class="affected-table">
          <thead><tr><th>决定</th><th>当前状态</th><th>回退时间</th><th>终审情况</th></tr></thead>
          <tbody>
            <tr *ngFor="let decision of sample.affectedDecisions">
              <td><strong>{{ decision.code }}</strong><small>{{ decision.name }}</small></td>
              <td><span class="status" [class]="'status status--' + tone(decision.status)">{{ decision.status }}</span></td>
              <td>{{ decision.rolledBackAt ? formatDate(decision.rolledBackAt) : '—' }}<small *ngIf="decision.rollbackReason">{{ decision.rollbackReason }}</small></td>
              <td>
                <span *ngIf="decision.resolvedAt" class="status status--success">已替代终审 · {{ formatDate(decision.resolvedAt) }}</span>
                <span *ngIf="!decision.resolvedAt && decision.rolledBackAt" class="status status--warning">等待替代样本终审</span>
                <span *ngIf="!decision.resolvedAt && !decision.rolledBackAt" class="muted">未回退</span>
              </td>
            </tr>
          </tbody>
        </table>
        <ng-template #noDecision><div class="empty">暂无引用该样本的合规决定</div></ng-template>
      </article>
    </div>
    <ng-template #empty><div class="empty">暂无样本数据</div></ng-template>
  </section>`,
  styles: [`.affected-panel { background: white; border: 1px solid #dbe4e8; margin-bottom: 14px; padding: 15px; }
    .affected-panel > header { display: flex; justify-content: space-between; gap: 16px; margin-bottom: 12px; }
    .affected-panel > header span { color: #6c7e8a; font-size: 12px; }
    .affected-group { padding: 12px; background: #f6f9fa; border-left: 3px solid #167c66; margin-bottom: 10px; }
    .sample-head { display: flex; justify-content: space-between; align-items: center; gap: 10px; margin-bottom: 6px; }
    .invalidation-reason { color: #922b34; font-size: 13px; margin: 4px 0 8px; }
    .affected-table { width: 100%; border-collapse: collapse; font-size: 13px; }
    .affected-table th, .affected-table td { text-align: left; padding: 7px 8px; border-bottom: 1px solid #e3eaee; vertical-align: top; }
    .affected-table th { color: #6c7e8a; font-weight: 600; }
    .affected-table small { display: block; color: #82919a; margin-top: 2px; }`],
})
export class AffectedDecisionsComponent {
  @Input() samples: DomainRecord[] = [];
  readonly formatDate = formatDate;

  tone(status: string): string {
    if (status === 'accepted') return 'success';
    if (status === 'escalated') return 'danger';
    if (status === 'review') return 'warning';
    return 'neutral';
  }
}
