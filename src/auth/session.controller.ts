import { Body, Controller, Delete, Get, HttpCode, HttpException, Post, Req, Res, UseGuards } from '@nestjs/common';
import type { Request, Response } from 'express';
import { SessionService } from './session.service.js';
import { AdminAuthGuard } from './admin-auth.guard.js';
import { AppConfig } from '../common/config.js';

@Controller('api/v1/session')
export class SessionController {
  constructor(
    private readonly sessions: SessionService,
    private readonly config: AppConfig,
  ) {}

  @Post()
  @HttpCode(204)
  login(@Body() body: { key?: string }, @Req() req: Request, @Res({ passthrough: true }) res: Response): void {
    if (!this.config.adminKey) throw new HttpException({ message: 'admin API disabled: ADMIN_KEY is not set' }, 503);
    if (typeof body?.key !== 'string' || !this.sessions.login(body.key, req, res)) {
      throw new HttpException({ message: 'wrong admin key' }, 401);
    }
  }

  @Get()
  @UseGuards(AdminAuthGuard)
  me(): { ok: true } {
    return { ok: true };
  }

  @Delete()
  @HttpCode(204)
  logout(@Res({ passthrough: true }) res: Response): void {
    this.sessions.logout(res);
  }
}
