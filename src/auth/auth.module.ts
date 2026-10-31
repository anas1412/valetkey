import { Global, Module } from '@nestjs/common';
import { TokenAuthGuard } from './token-auth.guard.js';
import { AdminAuthGuard } from './admin-auth.guard.js';
import { SessionService } from './session.service.js';
import { SessionController } from './session.controller.js';

@Global()
@Module({
  controllers: [SessionController],
  providers: [TokenAuthGuard, AdminAuthGuard, SessionService],
  exports: [TokenAuthGuard, AdminAuthGuard, SessionService],
})
export class AuthModule {}
