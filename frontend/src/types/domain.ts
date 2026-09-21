
export interface DomainRecord {
  id: number;
  code: string;
  name: string;
  status: string;
  version: number;
  description: string;
  facility: string;
  owner: string;
  category: string;
  riskLevel: 'low' | 'medium' | 'high' | 'critical';
  metricValue: number;
  metricUnit: string;
  effectiveAt: string;
  evidence: string;
  relatedCode: string;
  createdAt: string;
  updatedAt: string;
  revisions?: DecisionRevision[];
  // 合规决定 → 排放样本绑定及回退链路。
  sampleId?: number | null;
  sampleCode?: string;
  replacementSampleCode?: string;
  rollbacks?: DecisionRollback[];
  // 排放样本作废上下文。
  invalidatedReason?: string;
  invalidatedAt?: string | null;
  affectedDecisions?: AffectedDecision[];
}

export interface DecisionRevision {
  id: number;
  complianceDecisionId: number;
  version: number;
  state: string;
  evidence: string;
  reason: string;
  actor: string;
  requestId: string;
  createdAt: string;
}

export interface DecisionRollback {
  id: number;
  complianceDecisionId: number;
  chainOrder: number;
  fromState: string;
  invalidatedSampleId: number;
  invalidatedSampleCode: string;
  invalidationReason: string;
  invalidatedAt: string;
  rolledBackAt: string;
  replacementSampleId?: number | null;
  replacementSampleCode?: string;
  resolvedAt?: string | null;
  resolvedBy?: string;
}

export interface AffectedDecision {
  decisionId: number;
  code: string;
  name: string;
  status: string;
  version: number;
  rolledBackAt?: string | null;
  rollbackReason?: string;
  replacementSampleId?: number | null;
  resolvedAt?: string | null;
}

export interface PageMeta { page: number; pageSize: number; total: number }
export interface ApiEnvelope<T> { data: T; error?: string; message?: string; meta?: PageMeta }
export interface UserSession { token: string; username: string; displayName: string; role: string; expiresIn: number }
export interface AuditLog {
  id: number; requestId: string; actor: string; action: string; entityType: string;
  entityId: number; beforeState: string; afterState: string; detail: string; createdAt: string;
}
export interface EntityConfig { key: string; path: string; label: string; statuses: readonly string[] }
