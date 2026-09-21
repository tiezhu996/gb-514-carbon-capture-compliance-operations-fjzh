
import { request } from './client';
import type { DomainRecord } from '../types/domain';

// 作废复核 flow endpoints. These mirror the dedicated backend routes and keep
// the void / rollback / finalize actions separate from generic transitions.
export async function voidEmissionSample(id: number, reason: string) {
  return request<DomainRecord>(`/samples/${id}/void`, { method: 'POST', body: JSON.stringify({ reason }) });
}

export async function getDecisionRollback(id: number) {
  return request<DomainRecord>(`/decisions/${id}/rollback`);
}

export async function finalizeDecisionRollback(id: number, substituteSampleId: number, reason: string) {
  return request<DomainRecord>(`/decisions/${id}/finalize`, {
    method: 'POST', body: JSON.stringify({ substituteSampleId, reason }),
  });
}
