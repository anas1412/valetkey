import { Module } from '@nestjs/common';
import { ChannelsController } from './channels.controller.js';
import { ProxyKeysController } from './proxykeys.controller.js';
import { GroupsController } from './groups.controller.js';
import { OverviewController } from './overview.controller.js';
import { ReconcilerModule } from '../reconciler/reconciler.module.js';

@Module({
  imports: [ReconcilerModule],
  controllers: [ChannelsController, ProxyKeysController, GroupsController, OverviewController],
})
export class ManagementModule {}
