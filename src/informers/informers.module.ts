import { Module } from '@nestjs/common';
import { ChannelInformerService } from './channel-informer.service.js';
import { SecretInformerService } from './secret-informer.service.js';
import { ProxyKeyInformerService } from './proxykey-informer.service.js';
import { GroupInformerService } from './group-informer.service.js';
import { ReconcilerModule } from '../reconciler/reconciler.module.js';

@Module({
  imports: [ReconcilerModule],
  providers: [
    ChannelInformerService,
    SecretInformerService,
    ProxyKeyInformerService,
    GroupInformerService,
  ],
})
export class InformersModule {}
