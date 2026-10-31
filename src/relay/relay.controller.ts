import { Controller, Get, Post, Req, Res, UseGuards } from '@nestjs/common';
import type { Response } from 'express';
import { RelayService } from './relay.service.js';
import { RegistryService } from '../registry/registry.service.js';
import { servesModel } from './channel-picker.service.js';
import { TokenAuthGuard, type AuthenticatedRequest } from '../auth/token-auth.guard.js';

@Controller('v1')
@UseGuards(TokenAuthGuard)
export class RelayController {
  constructor(
    private readonly relay: RelayService,
    private readonly registry: RegistryService,
  ) {}

  /** Models the calling key can use: its group's channel models, narrowed by the key's allowlist. */
  @Get('models')
  getModels(@Req() req: AuthenticatedRequest): { object: string; data: Array<{ id: string; object: string; created: number; owned_by: string }> } {
    const seen = new Map<string, string>();
    for (const e of this.registry.getGroupChannels(req.route.groupName)) {
      for (const m of [...(e.channel.spec.models ?? []), ...Object.keys(e.channel.spec.modelMapping ?? {})]) {
        if (!seen.has(m) && servesModel(e, m)) seen.set(m, e.channel.spec.type);
      }
    }
    const allowed = req.route.models;
    const data = [...seen]
      .filter(([id]) => !allowed || allowed.includes(id))
      .map(([id, owner]) => ({ id, object: 'model', created: 0, owned_by: owner }));
    return { object: 'list', data };
  }

  @Post('chat/completions')
  chatCompletions(@Req() req: AuthenticatedRequest, @Res() res: Response): Promise<void> {
    return this.relay.handle(req, res, req.route, 'openai');
  }

  @Post('messages')
  messages(@Req() req: AuthenticatedRequest, @Res() res: Response): Promise<void> {
    return this.relay.handle(req, res, req.route, 'anthropic');
  }
}
