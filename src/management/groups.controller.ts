import { Body, Controller, Delete, Get, HttpCode, HttpException, Param, Post, Put, UseGuards } from '@nestjs/common';
import { K8sService } from '../k8s/k8s.service.js';
import { AppConfig } from '../common/config.js';
import { RegistryService } from '../registry/registry.service.js';
import { ReconcilerService } from '../reconciler/reconciler.service.js';
import { AdminAuthGuard } from '../auth/admin-auth.guard.js';
import { k8sCall, assertName } from './k8s-errors.js';
import { GROUP_PLURAL, type Group, type GroupSpec } from '../common/types.js';

export interface GroupView {
  name: string;
  spec: GroupSpec;
  ready: boolean;
  message?: string;
  keys: number;
}

@Controller('api/v1/groups')
@UseGuards(AdminAuthGuard)
export class GroupsController {
  constructor(
    private readonly k8s: K8sService,
    private readonly config: AppConfig,
    private readonly registry: RegistryService,
    private readonly reconciler: ReconcilerService,
  ) {}

  @Get()
  list(): GroupView[] {
    return this.registry.getAllGroups().map((e) => this.view(e.group));
  }

  @Get(':name')
  get(@Param('name') name: string): GroupView {
    const e = this.registry.getGroup(name);
    if (!e) throw new HttpException({ message: `group "${name}" not found` }, 404);
    return this.view(e.group);
  }

  @Post()
  async create(@Body() body: { name?: string; spec: GroupSpec }): Promise<GroupView> {
    assertName(body?.name, 'tenant name');
    const name = body.name;
    const created = await k8sCall(() => this.k8s.createObject<Group>(GROUP_PLURAL, this.ns, {
      apiVersion: 'llmproxy.llmproxy.io/v1alpha1', kind: 'Group', metadata: { name, namespace: this.ns }, spec: body.spec,
    }));
    return this.view(created);
  }

  @Put(':name')
  async update(@Param('name') name: string, @Body() body: { spec: GroupSpec }): Promise<GroupView> {
    if (!body?.spec) throw new HttpException({ message: '"spec" is required' }, 400);
    return this.view(await k8sCall(() => this.k8s.replaceSpec<Group>(GROUP_PLURAL, this.ns, name, body.spec)));
  }

  @Delete(':name')
  @HttpCode(204)
  async delete(@Param('name') name: string): Promise<void> {
    await k8sCall(() => this.k8s.deleteObject(GROUP_PLURAL, this.ns, name));
  }

  private get ns(): string {
    return this.config.operatorNamespace;
  }

  private view(g: Group): GroupView {
    const name = g.metadata!.name!;
    return {
      name,
      spec: g.spec,
      ready: g.status?.ready ?? false,
      message: g.status?.message,
      keys: this.reconciler.getAll().filter((pk) => pk.metadata!.namespace === name).length,
    };
  }
}
