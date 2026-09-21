import type { EntityConfig } from './domain';

export type UnitState = 'standby' | 'running' | 'limited' | 'stopped';
export const ALL_UNIT_STATE: readonly UnitState[] = ['standby', 'running', 'limited', 'stopped'];
export type DecisionState = 'draft' | 'review' | 'accepted' | 'escalated' | 'review_required';
export const ALL_DECISION_STATE: readonly DecisionState[] = ['draft', 'review', 'accepted', 'escalated', 'review_required'];

export const ENTITY_CONFIGS: readonly EntityConfig[] = [
  { key: 'captureUnit', path: 'units', label: '捕集装置', statuses: ['standby', 'running', 'limited', 'stopped'] as const },
  { key: 'permitRule', path: 'rules', label: '许可规则', statuses: ['draft', 'active', 'superseded', 'retired'] as const },
  { key: 'emissionSample', path: 'samples', label: '排放样本', statuses: ['collected', 'testing', 'verified', 'invalid'] as const },
  { key: 'complianceDecision', path: 'decisions', label: '合规决定', statuses: ['draft', 'review', 'accepted', 'escalated', 'review_required'] as const }
];

// Human-facing Chinese labels for machine statuses. Falls back to the raw
// status so unknown values stay visible instead of rendering blank.
const STATUS_LABELS: Record<string, string> = {
  standby: '待机', running: '运行中', limited: '限产', stopped: '停机',
  draft: '草稿', review: '复核中', accepted: '已接受', escalated: '已升级',
  review_required: '回退复核', collected: '已采样', testing: '检测中',
  verified: '已验证', invalid: '已作废', active: '生效中', superseded: '已替代', retired: '已停用',
};

export function statusLabel(status: string): string {
  return STATUS_LABELS[status] || status.replaceAll('_', ' ');
}
