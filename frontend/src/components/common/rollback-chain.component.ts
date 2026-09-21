import { CommonModule } from '@angular/common';
import { Component, Input } from '@angular/core';
import type { DomainRecord } from '../../types/domain';
import { formatDate } from '../../utils/format';

// RollbackChainComponent renders the append-only rollback link of a 合规决定:
// which 已接受/升级 conclusion was withdrawn, why the referenced sample was
// invalidated, and which replacement sample re-finalised the decision.
@Component({
  selector: 'app-rollback-chain',
  standalone: true,
  imports: [CommonModule],
  template: `<section class="rollback-panel" aria-label="回退链路">
    <header><strong>回退复核链路</strong><span>样本作废后决定自动回退，原结论与证据保留，决定在终审前不生效</span></header>
    <div *ngIf="rollbackRecords.length; else empty">
      <article *ngFor="let item of rollbackRecords" class="rollback-card">
        <div class="rollback-head">
          <strong>{{ item.code }} · {{ item.name }}</strong>
          <span *ngIf="openRollback(item)" class="status status--warning">已回退 · 待终审（{{ openRollback(item)!.chainOrder }}）</span>
          <span *ngIf="!openRollback(item)" class="status status--success">已恢复生效</span>
        </div>
        <ol class="rollback-chain">
          <li *ngFor="let rollback of item.rollbacks || []">
            <div class="chain-line">
              <span class="chain-index">第 {{ rollback.chainOrder }} 次回退</span>
              <span class="status status--danger">{{ rollback.fromState }} → review</span>
            </div>
            <p>作废样本 <strong>{{ rollback.invalidatedSampleCode }}</strong> · 作废时间 {{ formatDate(rollback.invalidatedAt) }}</p>
            <p class="chain-reason">作废原因：{{ rollback.invalidationReason }}</p>
            <p *ngIf="rollback.resolvedAt; else pendingResolve">
              替代样本 <strong>{{ rollback.replacementSampleCode || ('#' + rollback.replacementSampleId) }}</strong>
              已由 {{ rollback.resolvedBy || '复核人' }} 于 {{ formatDate(rollback.resolvedAt) }} 终审恢复
            </p>
            <ng-template #pendingResolve><p class="chain-pending">等待复核员选择同装置、已验证且采样晚于作废时间的替代样本终审</p></ng-template>
          </li>
        </ol>
      </article>
    </div>
    <ng-template #empty><div class="empty">暂无回退记录</div></ng-template>
  </section>`,
  styles: [`.rollback-panel { background: white; border: 1px solid #dbe4e8; margin-bottom: 14px; padding: 15px; }
    .rollback-panel > header { display: flex; justify-content: space-between; gap: 16px; margin-bottom: 12px; }
    .rollback-panel > header span { color: #6c7e8a; font-size: 12px; }
    .rollback-card { padding: 12px; background: #f6f9fa; border-left: 3px solid #c84855; margin-bottom: 10px; }
    .rollback-head { display: flex; justify-content: space-between; align-items: center; gap: 10px; margin-bottom: 8px; }
    .rollback-chain { margin: 0; padding-left: 18px; }
    .rollback-chain li { margin-bottom: 8px; }
    .chain-line { display: flex; justify-content: space-between; align-items: center; gap: 10px; }
    .chain-index { font-weight: 650; color: #506572; font-size: 13px; }
    .rollback-chain p { margin: 3px 0; color: #526473; font-size: 13px; }
    .chain-reason { color: #922b34 !important; }
    .chain-pending { color: #8a5a08 !important; }`],
})
export class RollbackChainComponent {
  @Input() records: DomainRecord[] = [];
  readonly formatDate = formatDate;

  get rollbackRecords(): DomainRecord[] {
    return this.records.filter((item) => item.rollbacks && item.rollbacks.length > 0);
  }
  openRollback(item: DomainRecord) {
    return (item.rollbacks || []).find((rollback) => !rollback.resolvedAt) || null;
  }
}
