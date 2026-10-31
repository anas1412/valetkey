import { HttpException, Injectable, type CanActivate, type ExecutionContext } from '@nestjs/common';
import type { Request } from 'express';
import { AppConfig } from '../common/config.js';
import { SessionService } from './session.service.js';

/** Admin API guard: accepts a dashboard session cookie or `Authorization: Bearer <ADMIN_KEY>`. */
@Injectable()
export class AdminAuthGuard implements CanActivate {
  constructor(
    private readonly config: AppConfig,
    private readonly sessions: SessionService,
  ) {}

  canActivate(context: ExecutionContext): boolean {
    if (!this.config.adminKey) {
      throw new HttpException({ message: 'admin API disabled: ADMIN_KEY is not set' }, 503);
    }
    const req = context.switchToHttp().getRequest<Request>();
    if (this.sessions.isValid(req) || this.sessions.isAdminBearer(req)) return true;
    throw new HttpException({ message: 'admin authentication required' }, 401);
  }
}
