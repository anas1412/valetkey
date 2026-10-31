import { Global, Module } from '@nestjs/common';
import { StatsService } from './stats.service.js';

@Global()
@Module({
  providers: [StatsService],
  exports: [StatsService],
})
export class StatsModule {}
