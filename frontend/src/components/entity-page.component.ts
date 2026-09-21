import { AsyncPipe, CommonModule } from '@angular/common';
import { ChangeDetectorRef, Component, Input, OnInit } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { MatButtonModule } from '@angular/material/button';
import { MatInputModule } from '@angular/material/input';
import { MatSelectModule } from '@angular/material/select';
import { authState } from '../hooks/use-auth';
import type { EntityStore } from '../stores/factory';
import type { DomainRecord, EntityConfig } from '../types/domain';
import { formatDate, nextStatus } from '../utils/format';
import { AffectedDecisionsComponent } from './common/affected-decisions.component';
import { ConfirmDialogComponent } from './common/confirm-dialog.component';
import { ComplianceBadgeComponent } from './common/compliance-badge.component';
import { EvidenceListComponent } from './common/evidence-list.component';
import { MetricCardComponent } from './common/metric-card.component';
import { RollbackChainComponent } from './common/rollback-chain.component';
import { RuleDiffComponent } from './common/rule-diff.component';
import { StatusBadgeComponent } from './common/status-badge.component';

interface PendingTransition {
  item: DomainRecord;
  status: string;
  reason: string;
  replacementSampleId?: number;
}

@Component({
  selector: 'app-entity-page',
  standalone: true,
  imports: [CommonModule, AsyncPipe, FormsModule, MatButtonModule, MatInputModule, MatSelectModule,
    StatusBadgeComponent, MetricCardComponent, ConfirmDialogComponent, EvidenceListComponent,
    ComplianceBadgeComponent, RuleDiffComponent, RollbackChainComponent, AffectedDecisionsComponent],
  template: `<main class="workspace" *ngIf="store.state$ | async as state">
    <header class="page-header">
      <div><p class="eyebrow">业务工作台</p><h1>{{ config.label }}</h1><p>统一管理{{ config.label }}的状态、风险、证据与责任人。</p></div>
      <button *ngIf="canWrite()" mat-flat-button color="primary" (click)="openCreate()">新增{{ config.label }}</button>
    </header>
    <section class="metrics">
      <app-metric-card label="记录总数" [value]="state.meta.total" detail="当前筛选范围"/>
      <app-metric-card label="高风险" [value]="highRisk(state.items)" detail="需要优先复核"/>
      <app-metric-card label="状态种类" [value]="statusCount(state.items)" detail="状态机覆盖"/>
    </section>
    <app-compliance-badge *ngIf="config.path === 'units' || config.path === 'decisions'" [records]="state.items"/>
    <app-rule-diff *ngIf="config.path === 'rules' || config.path === 'samples'" [records]="state.items"/>
    <app-rollback-chain *ngIf="config.path === 'decisions'" [records]="state.items"/>
    <app-affected-decisions *ngIf="config.path === 'samples'" [samples]="state.items"/>
    <app-evidence-list [records]="state.items"/>
    <section class="toolbar">
      <input matInput aria-label="搜索" [(ngModel)]="search" [placeholder]="'搜索' + config.label + '编码或名称'"/>
      <button mat-flat-button color="primary" (click)="query()">查询</button>
      <button mat-button (click)="reset()">重置</button>
    </section>
    <div *ngIf="state.error" class="alert">{{ state.error }}</div>
    <section class="table-shell"><table><thead><tr><th>编码</th><th>名称</th><th>状态</th><th>风险</th><th>责任人</th><th>指标</th><th>更新时间</th><th>操作</th></tr></thead>
      <tbody><tr *ngFor="let item of state.items"><td><strong>{{ item.code }}</strong></td>
        <td>{{ item.name }}<small>{{ item.facility }}</small>
          <small *ngIf="config.path === 'decisions' && (item.sampleCode || item.replacementSampleCode)" class="ref-line">
            引用样本：{{ item.sampleCode || '—' }}<ng-container *ngIf="item.replacementSampleCode"> · 替代样本 {{ item.replacementSampleCode }}</ng-container>
          </small>
          <small *ngIf="config.path === 'samples' && item.status === 'invalid'" class="ref-line danger-line">已作废：{{ item.invalidatedReason }}</small>
        </td>
        <td><app-status-badge [status]="item.status"/></td><td>{{ item.riskLevel }}</td><td>{{ item.owner }}</td>
        <td>{{ item.metricValue }} {{ item.metricUnit }}</td><td>{{ formatDate(item.updatedAt) }}</td><td>
        <button *ngIf="canTransition(item)" class="table-action" (click)="openTransition(item, next(item)!)">{{ actionLabel(item, next(item)!) }}</button>
        <span *ngIf="!canTransition(item)" class="muted">{{ actionHint(item) }}</span>
      </td></tr><tr *ngIf="!state.items.length && !state.loading"><td colspan="8" class="empty">暂无记录</td></tr></tbody></table>
      <div *ngIf="state.loading" class="loading">正在同步业务数据…</div>
    </section>
    <app-confirm-dialog [open]="showCreate" [title]="'新增' + config.label" (cancel)="closeCreate()" (confirm)="createDemo()"><p>将创建一条包含完整责任人、风险和证据信息的演示记录。</p></app-confirm-dialog>
    <app-confirm-dialog [open]="!!pending" [title]="transitionTitle()" (cancel)="closeTransition()" (confirm)="confirmTransition()">
      <p>状态迁移会追加不可变版本并记录证据、操作者与请求 ID。</p>
      <strong>{{ pending?.item?.status }} → {{ pending?.status }}</strong>
      <label class="reason-field">操作/作废原因
        <textarea matInput rows="2" [(ngModel)]="pendingReason" placeholder="请填写本次状态迁移或样本作废原因"></textarea>
      </label>
      <div *ngIf="needsReplacement()" class="replacement-box">
        <p>该决定已因样本作废回退，终审必须选择替代样本（同装置、已验证、采样晚于作废时间）。</p>
        <label>替代样本
          <mat-select [(ngModel)]="pendingReplacementId" placeholder="选择已验证替代样本">
            <mat-option *ngFor="let candidate of replacementCandidates" [value]="candidate.id">
              {{ candidate.code }} · {{ candidate.name }}（采样 {{ formatDate(candidate.effectiveAt) }}）
            </mat-option>
          </mat-select>
        </label>
        <p *ngIf="!replacementCandidates.length" class="chain-pending">当前没有符合条件的替代样本，请先在同装置补采并验证样本。</p>
      </div>
    </app-confirm-dialog>
  </main>`,
  styles: [`.ref-line { display: block; color: #6c7e8a; font-size: 12px; margin-top: 2px; }
    .danger-line { color: #922b34; }
    .reason-field { display: block; margin-top: 12px; color: #506572; font-size: 13px; }
    .reason-field textarea { width: 100%; margin-top: 4px; padding: 8px; border: 1px solid #c4d0d7; box-sizing: border-box; font: inherit; }
    .replacement-box { margin-top: 12px; padding: 10px; background: #fff7e8; border-left: 3px solid #c98a1e; }
    .replacement-box p { margin: 0 0 8px; color: #8a5a08; font-size: 13px; }
    .replacement-box mat-select { width: 100%; margin-top: 4px; border-bottom: 1px solid #c4d0d7; }
    .chain-pending { color: #922b34 !important; }`],
})
export class EntityPageComponent implements OnInit {
  @Input({ required: true }) config!: EntityConfig;
  @Input({ required: true }) store!: EntityStore;
  search = '';
  showCreate = false;
  pending: PendingTransition | null = null;
  pendingReason = '';
  pendingReplacementId: number | null = null;
  replacementCandidates: DomainRecord[] = [];
  readonly formatDate = formatDate;

  constructor(private readonly changeDetector: ChangeDetectorRef) {}
  async ngOnInit() { await this.load(); }
  next(item: DomainRecord) { return nextStatus(item.status, this.config.statuses); }
  canWrite() { return authState.hasMinimumRole('operator'); }
  canReview() { return authState.hasMinimumRole('reviewer'); }

  openRollback(item: DomainRecord) {
    return (item.rollbacks || []).find((rollback) => !rollback.resolvedAt) || null;
  }

  canTransition(item: DomainRecord): boolean {
    const target = this.next(item);
    if (!target || !this.canWrite()) return false;
    // A rolled-back decision can only be finalised on a replacement sample by
    // a reviewer; the review → draft fallback keeps the existing flow.
    if (this.config.path === 'decisions') {
      if (this.openRollback(item) && (target === 'accepted' || target === 'escalated')) return this.canReview();
      if (['accepted', 'escalated'].includes(target)) return this.canReview();
    }
    return true;
  }

  actionLabel(item: DomainRecord, target: string): string {
    if (this.config.path === 'decisions' && this.openRollback(item) &&
      (target === 'accepted' || target === 'escalated')) return '替代样本终审';
    return `推进至 ${target}`;
  }

  actionHint(item: DomainRecord): string {
    if (!this.canWrite()) return '只读权限';
    const target = this.next(item);
    if (this.config.path === 'decisions' && target && ['accepted', 'escalated'].includes(target) && !this.canReview()) {
      return this.openRollback(item) ? '等待复核员替代样本终审' : '等待复核员决定';
    }
    return '流程结束';
  }

  highRisk(items: DomainRecord[]) { return items.filter((item) => ['high', 'critical'].includes(item.riskLevel)).length; }
  statusCount(items: DomainRecord[]) { return new Set(items.map((item) => item.status)).size; }
  async query() { await this.load(this.search); }
  async reset() { this.search = ''; await this.load(); }
  openCreate() { if (this.canWrite()) this.showCreate = true; this.changeDetector.detectChanges(); }
  closeCreate() { this.showCreate = false; this.changeDetector.detectChanges(); }

  async openTransition(item: DomainRecord, status: string) {
    if (!this.canTransition(item)) return;
    this.pending = { item, status, reason: '' };
    this.pendingReason = this.config.path === 'samples' && status === 'invalid'
      ? '样本检测/校准失效，申请作废并回退引用决定'
      : '前端工作台人工确认';
    this.pendingReplacementId = null;
    this.replacementCandidates = [];
    if (this.needsReplacement()) {
      try {
        const result = await this.store.replacementCandidates(item.id);
        this.replacementCandidates = result.data;
      } catch { /* error surfaces through the store state on submit */ }
    }
    this.changeDetector.detectChanges();
  }

  needsReplacement(): boolean {
    return !!this.pending && this.config.path === 'decisions' &&
      !!this.openRollback(this.pending.item) &&
      ['accepted', 'escalated'].includes(this.pending.status);
  }

  transitionTitle(): string {
    if (this.needsReplacement()) return '回退复核 · 替代样本终审';
    return '确认状态迁移';
  }

  closeTransition() {
    this.pending = null;
    this.pendingReason = '';
    this.pendingReplacementId = null;
    this.replacementCandidates = [];
    this.changeDetector.detectChanges();
  }

  async createDemo() {
    if (!this.canWrite()) return;
    const now = Date.now();
    try {
      await this.store.createRecord(this.config.path, {
        code: `${this.config.key.toUpperCase()}-${String(now).slice(-6)}`, name: `新增${this.config.label}`,
        description: '通过前端工作台创建的业务记录', facility: '默认作业区', owner: '现场操作员',
        category: '常规', riskLevel: 'medium', metricValue: 25, metricUnit: 'unit',
        effectiveAt: new Date().toISOString(), evidence: '已完成创建前检查', relatedCode: '',
      });
      this.search = '';
      this.showCreate = false;
    } catch { /* Store exposes the request error in its observable state. */ }
    finally { this.changeDetector.detectChanges(); }
  }

  async confirmTransition() {
    if (!this.pending || !this.canTransition(this.pending.item)) return;
    const reason = this.pendingReason.trim();
    if (reason.length < 3) return;
    if (this.needsReplacement() && !this.pendingReplacementId) return;
    try {
      await this.store.transition(this.config.path, this.pending.item, this.pending.status, {
        reason,
        replacementSampleId: this.needsReplacement() && this.pendingReplacementId
          ? this.pendingReplacementId : undefined,
      });
      this.search = '';
      this.pending = null;
      this.pendingReason = '';
      this.pendingReplacementId = null;
      this.replacementCandidates = [];
    } catch { /* Store exposes the request error in its observable state. */ }
    finally { this.changeDetector.detectChanges(); }
  }

  private async load(search = '') { await this.store.load(this.config.path, search); this.changeDetector.detectChanges(); }
}
