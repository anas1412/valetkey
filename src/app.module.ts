import { Module, Global } from '@nestjs/common';

import { AppConfig } from './common/config.js';
import { K8sModule } from './k8s/k8s.module.js';
import { RegistryModule } from './registry/registry.module.js';
import { InformersModule } from './informers/informers.module.js';
import { ReconcilerModule } from './reconciler/reconciler.module.js';
import { RelayModule } from './relay/relay.module.js';
import { AuthModule } from './auth/auth.module.js';
import { ManagementModule } from './management/management.module.js';
import { MetricsModule } from './metrics/metrics.module.js';
import { HealthModule } from './health/health.module.js';
import { StatsModule } from './stats/stats.module.js';

@Global()
@Module({
  imports: [
    K8sModule,
    RegistryModule,
    InformersModule,
    ReconcilerModule,
    RelayModule,
    AuthModule,
    ManagementModule,
    MetricsModule,
    StatsModule,
    HealthModule,
  ],
  providers: [AppConfig],
  exports: [AppConfig],
})
export class AppModule {}
