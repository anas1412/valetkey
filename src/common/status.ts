import type { KubernetesCondition } from './types.js';

export interface ReadyStatus {
  ready: boolean;
  message: string;
  conditions: KubernetesCondition[];
}

/**
 * Builds a status with a standard `Ready` condition. lastTransitionTime is kept
 * from the previous condition when readiness did not change.
 */
export function readyStatus(
  prev: { conditions?: KubernetesCondition[] } | undefined,
  ready: boolean,
  reason: string,
  message: string,
  generation: number | undefined,
): ReadyStatus {
  const status = ready ? 'True' : 'False';
  const old = prev?.conditions?.find((c) => c.type === 'Ready');
  const condition: KubernetesCondition = {
    type: 'Ready',
    status,
    reason,
    message,
    lastTransitionTime: old?.status === status ? old.lastTransitionTime : new Date().toISOString(),
    observedGeneration: generation,
  };
  return { ready, message, conditions: [condition] };
}

/** True if `next` differs from `prev` in readiness, message, generation, or any of the extra fields. */
export function statusChanged(prev: Record<string, unknown> | undefined, next: ReadyStatus): boolean {
  if (!prev) return true;
  const prevReady = (prev.conditions as KubernetesCondition[] | undefined)?.find((c) => c.type === 'Ready');
  const nextReady = next.conditions[0];
  if (prevReady?.reason !== nextReady.reason || prevReady?.observedGeneration !== nextReady.observedGeneration) return true;
  return Object.entries(next).some(([k, v]) => k !== 'conditions' && prev[k] !== v);
}
