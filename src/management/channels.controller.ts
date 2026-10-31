import { Body, Controller, Delete, Get, HttpCode, HttpException, Param, Post, Put, UseGuards } from '@nestjs/common';
import { K8sService } from '../k8s/k8s.service.js';
import { AppConfig } from '../common/config.js';
import { RegistryService } from '../registry/registry.service.js';
import { AdminAuthGuard } from '../auth/admin-auth.guard.js';
import { k8sCall, assertName } from './k8s-errors.js';
import { testChannel, type ChannelTestResult } from './channel-test.js';
import { CHANNEL_PLURAL, type Channel, type ChannelSpec } from '../common/types.js';

const MANAGED_BY = { 'app.kubernetes.io/managed-by': 'llmproxy-operator' };

/** What the dashboard sends: the spec plus, optionally, the provider API key to store. */
interface ChannelInput {
  name?: string;
  spec: Omit<ChannelSpec, 'keySecretRef'> & { keySecretRef?: ChannelSpec['keySecretRef'] };
  apiKey?: string;
}

export interface ChannelView {
  name: string;
  spec: ChannelSpec;
  ready: boolean;
  message?: string;
}

/** Secret the dashboard creates for a channel's provider key. */
function keySecretName(channel: string): string {
  return `${channel}-api-key`;
}

@Controller('api/v1/channels')
@UseGuards(AdminAuthGuard)
export class ChannelsController {
  constructor(
    private readonly k8s: K8sService,
    private readonly config: AppConfig,
    private readonly registry: RegistryService,
  ) {}

  @Get()
  list(): ChannelView[] {
    return this.registry.getAllChannels().map((e) => view(e.channel, !!e.upstreamKey));
  }

  @Get(':name')
  get(@Param('name') name: string): ChannelView {
    const e = this.registry.getChannel(name);
    if (!e) throw new HttpException({ message: `channel "${name}" not found` }, 404);
    return view(e.channel, !!e.upstreamKey);
  }

  @Post()
  async create(@Body() body: ChannelInput): Promise<ChannelView> {
    assertName(body?.name, 'channel name');
    const name = body.name;
    const spec = await this.specWithKey(name, body);
    const created = await k8sCall(() => this.k8s.createObject<Channel>(CHANNEL_PLURAL, this.ns, {
      apiVersion: 'llmproxy.llmproxy.io/v1alpha1', kind: 'Channel', metadata: { name, namespace: this.ns }, spec,
    }));
    return view(created, false);
  }

  @Put(':name')
  async update(@Param('name') name: string, @Body() body: ChannelInput): Promise<ChannelView> {
    const existing = this.registry.getChannel(name)?.channel.spec.keySecretRef;
    const spec = await this.specWithKey(name, body, existing);
    const updated = await k8sCall(() => this.k8s.replaceSpec<Channel>(CHANNEL_PLURAL, this.ns, name, spec));
    return view(updated, !!this.registry.getChannel(name)?.upstreamKey);
  }

  @Delete(':name')
  @HttpCode(204)
  async delete(@Param('name') name: string): Promise<void> {
    const ref = this.registry.getChannel(name)?.channel.spec.keySecretRef;
    await k8sCall(() => this.k8s.deleteObject(CHANNEL_PLURAL, this.ns, name));
    // Only remove the key Secret if the dashboard created it.
    if (ref?.name === keySecretName(name)) {
      const secret = await this.k8s.readSecret(this.ns, ref.name);
      if (secret?.metadata?.labels?.['app.kubernetes.io/managed-by'] === MANAGED_BY['app.kubernetes.io/managed-by']) {
        await this.k8s.deleteSecret(this.ns, ref.name);
      }
    }
  }

  @Post(':name/test')
  async test(@Param('name') name: string): Promise<ChannelTestResult> {
    const entry = this.registry.getChannel(name);
    if (!entry) throw new HttpException({ message: `channel "${name}" not found` }, 404);
    return testChannel(entry);
  }

  private get ns(): string {
    return this.config.operatorNamespace;
  }

  /** Stores a pasted API key in a managed Secret and points the spec at it. */
  private async specWithKey(name: string, body: ChannelInput, existing?: ChannelSpec['keySecretRef']): Promise<ChannelSpec> {
    if (!body?.spec) throw new HttpException({ message: '"spec" is required' }, 400);
    let keySecretRef = body.spec.keySecretRef ?? existing;
    if (body.apiKey) {
      keySecretRef = { name: keySecretName(name), key: 'apiKey' };
      await k8sCall(() => this.k8s.createOrReplaceSecret(this.ns, keySecretRef!.name, {
        metadata: { name: keySecretRef!.name, namespace: this.ns, labels: MANAGED_BY },
        type: 'Opaque',
        stringData: { apiKey: body.apiKey! },
      }));
    }
    if (!keySecretRef) throw new HttpException({ message: 'an API key is required' }, 400);
    return { ...body.spec, keySecretRef };
  }
}

function view(ch: Channel, hasKey: boolean): ChannelView {
  return {
    name: ch.metadata!.name!,
    spec: ch.spec,
    ready: hasKey && (ch.spec.status ?? 'Enabled') === 'Enabled',
    message: ch.status?.message,
  };
}
