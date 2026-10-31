import { HttpException, Injectable, type CanActivate, type ExecutionContext } from '@nestjs/common';
import type { Request } from 'express';
import { RegistryService } from '../registry/registry.service.js';
import { hashKey, ipAllowed } from '../common/key-utils.js';
import type { Route } from '../common/types.js';

/** Attached to the request by TokenAuthGuard on successful auth */
export interface AuthenticatedRequest extends Request {
  route: Route;
}

@Injectable()
export class TokenAuthGuard implements CanActivate {
  constructor(private readonly registry: RegistryService) {}

  canActivate(context: ExecutionContext): boolean {
    const req = context.switchToHttp().getRequest<Request>();
    const key = this.extractKey(req);
    if (!key) throw unauthorized(req, 'missing API key');

    const route = this.registry.lookup(hashKey(key));
    if (!route) throw unauthorized(req, 'invalid API key');

    if (route.allowList && !ipAllowed(req.ip ?? req.socket.remoteAddress ?? '', route.allowList)) {
      throw unauthorized(req, 'client IP not allowed for this key');
    }

    (req as AuthenticatedRequest).route = route;
    return true;
  }

  private extractKey(req: Request): string | undefined {
    const auth = req.headers.authorization;
    if (auth?.startsWith('Bearer ')) return auth.slice(7);
    const xApiKey = req.headers['x-api-key'];
    return typeof xApiKey === 'string' ? xApiKey : undefined;
  }
}

/** 401 in the error shape of the API the client is calling. */
function unauthorized(req: Request, message: string): HttpException {
  const body = req.path.endsWith('/messages')
    ? { type: 'error', error: { type: 'authentication_error', message } }
    : { error: { message, type: 'invalid_api_key', code: 'invalid_api_key' } };
  return new HttpException(body, 401);
}
