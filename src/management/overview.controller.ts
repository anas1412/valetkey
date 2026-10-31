import { Controller, Get, UseGuards } from '@nestjs/common';
import { AppConfig } from '../common/config.js';
import { K8sService } from '../k8s/k8s.service.js';
import { RegistryService } from '../registry/registry.service.js';
import { ReconcilerService } from '../reconciler/reconciler.service.js';
import { StatsService, type MinuteBucket, type OperatorEvent, type RequestEntry } from '../stats/stats.service.js';
import { AdminAuthGuard } from '../auth/admin-auth.guard.js';
import { k8sCall } from './k8s-errors.js';

const SERIES_STEP_MINUTES = 5;

@Controller('api/v1')
@UseGuards(AdminAuthGuard)
export class OverviewController {
  constructor(
    private readonly config: AppConfig,
    private readonly k8s: K8sService,
    private readonly registry: RegistryService,
    private readonly reconciler: ReconcilerService,
    private readonly stats: StatsService,
  ) {}

  /** Everything the dashboard's Overview page needs in one call. */
  @Get('overview')
  overview(): Record<string, unknown> {
    const series = downsample(this.stats.series(), SERIES_STEP_MINUTES);
    const totals = series.reduce((t, b) => ({
      requests: t.requests + b.requests,
      errors: t.errors + b.errors,
      inputTokens: t.inputTokens + b.inputTokens,
      outputTokens: t.outputTokens + b.outputTokens,
    }), { requests: 0, errors: 0, inputTokens: 0, outputTokens: 0 });
    const keys = this.reconciler.getAll();

    return {
      proxyURL: this.config.proxyURL,
      startedAt: this.stats.startedAt,
      stepMinutes: SERIES_STEP_MINUTES,
      totals,
      series,
      channelShare: this.stats.channelShare(),
      counts: {
        channels: this.registry.getAllChannels().length,
        groups: this.registry.getAllGroups().length,
        keys: keys.length,
        activeKeys: this.registry.getAllRoutes().length,
      },
      channels: this.registry.getAllChannels().map((e) => ({
        name: e.channel.metadata!.name,
        type: e.channel.spec.type,
        enabled: (e.channel.spec.status ?? 'Enabled') === 'Enabled',
        ready: !!e.upstreamKey,
        message: e.channel.status?.message,
      })),
    };
  }

  @Get('activity')
  activity(): { requests: RequestEntry[]; events: OperatorEvent[] } {
    return { requests: this.stats.recentRequests(), events: this.stats.recentEvents() };
  }

  /** Compact summary used by llmproxyctl. */
  @Get('status')
  status(): Record<string, unknown> {
    return {
      channels: this.registry.getAllChannels().map((e) => ({
        name: e.channel.metadata!.name, type: e.channel.spec.type, models: e.channel.spec.models ?? [], ready: !!e.upstreamKey,
      })),
      groups: this.registry.getAllGroups().map((e) => ({
        name: e.group.metadata!.name, status: e.group.spec.status ?? 'Enabled', channels: e.group.spec.channelRefs,
      })),
      keys: this.reconciler.getAll().map((pk) => ({
        namespace: pk.metadata!.namespace, name: pk.metadata!.name, group: pk.metadata!.namespace, ready: pk.status?.ready ?? false,
      })),
    };
  }

  /** Namespaces for the tenant picker, flagging which already have a Group. */
  @Get('namespaces')
  async namespaces(): Promise<Array<{ name: string; hasGroup: boolean }>> {
    const names = await k8sCall(() => this.k8s.listNamespaces());
    return names
      .filter((n) => !n.startsWith('kube-') && n !== this.config.operatorNamespace)
      .map((name) => ({ name, hasGroup: !!this.registry.getGroup(name) }));
  }
}

function downsample(buckets: MinuteBucket[], step: number): MinuteBucket[] {
  const out: MinuteBucket[] = [];
  for (let i = 0; i < buckets.length; i += step) {
    const slice = buckets.slice(i, i + step);
    out.push(slice.reduce((acc, b) => ({
      minute: acc.minute,
      requests: acc.requests + b.requests,
      errors: acc.errors + b.errors,
      inputTokens: acc.inputTokens + b.inputTokens,
      outputTokens: acc.outputTokens + b.outputTokens,
    }), { minute: slice[0].minute, requests: 0, errors: 0, inputTokens: 0, outputTokens: 0 }));
  }
  return out;
}
