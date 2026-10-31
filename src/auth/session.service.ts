import { Injectable } from '@nestjs/common';
import type { Request, Response } from 'express';
import { AppConfig } from '../common/config.js';
import { secretsEqual, sign } from '../common/key-utils.js';

const COOKIE = 'llmproxy_session';
const TTL_SECONDS = 8 * 60 * 60;

/**
 * Stateless dashboard sessions: the cookie is `<expiry>.<hmac(adminKey, expiry)>`.
 * Changing ADMIN_KEY invalidates every session.
 */
@Injectable()
export class SessionService {
  constructor(private readonly config: AppConfig) {}

  /** Checks the admin key and, if correct, sets the session cookie. */
  login(key: string, req: Request, res: Response): boolean {
    if (!this.config.adminKey || !secretsEqual(key, this.config.adminKey)) return false;
    const expiry = String(Math.floor(Date.now() / 1000) + TTL_SECONDS);
    res.cookie(COOKIE, `${expiry}.${sign(this.config.adminKey, expiry)}`, {
      httpOnly: true,
      sameSite: 'strict',
      secure: req.secure,
      path: '/',
      maxAge: TTL_SECONDS * 1000,
    });
    return true;
  }

  logout(res: Response): void {
    res.clearCookie(COOKIE, { path: '/' });
  }

  isValid(req: Request): boolean {
    const value = readCookie(req, COOKIE);
    if (!value || !this.config.adminKey) return false;
    const [expiry, sig] = value.split('.');
    if (!expiry || !sig || Number(expiry) * 1000 < Date.now()) return false;
    return secretsEqual(sig, sign(this.config.adminKey, expiry));
  }

  isAdminBearer(req: Request): boolean {
    const auth = req.headers.authorization;
    return !!this.config.adminKey && !!auth?.startsWith('Bearer ') && secretsEqual(auth.slice(7), this.config.adminKey);
  }
}

function readCookie(req: Request, name: string): string | undefined {
  for (const part of (req.headers.cookie ?? '').split(';')) {
    const [k, ...v] = part.trim().split('=');
    if (k === name) return decodeURIComponent(v.join('='));
  }
  return undefined;
}
