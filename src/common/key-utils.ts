import { createHash, createHmac, randomBytes, timingSafeEqual } from 'node:crypto';
import { BlockList, isIP } from 'node:net';

const KEY_PREFIX = 'sk-proxy-';

/** Generate a new virtual API key: sk-proxy- + 32 chars of base64url entropy */
export function generateVirtualKey(): string {
  return KEY_PREFIX + randomBytes(24).toString('base64url');
}

/** SHA-256 hex digest of a key — stored in ProxyKey status, never the raw key */
export function hashKey(key: string): string {
  return createHash('sha256').update(key).digest('hex');
}

/** Constant-time comparison of two hex-encoded SHA-256 hashes */
export function hashesEqual(a: string, b: string): boolean {
  const ba = Buffer.from(a, 'hex');
  const bb = Buffer.from(b, 'hex');
  return ba.length === bb.length && timingSafeEqual(ba, bb);
}

/** Constant-time comparison of two secrets of any length */
export function secretsEqual(a: string, b: string): boolean {
  return hashesEqual(hashKey(a), hashKey(b));
}

/** HMAC-SHA256 hex signature, used for dashboard session cookies */
export function sign(secret: string, value: string): string {
  return createHmac('sha256', secret).update(value).digest('hex');
}

/**
 * Builds an IP allow list from CIDRs (IPv4 and IPv6; a bare address means a single host).
 * Throws on an invalid entry so the ProxyKey can report it.
 */
export function buildAllowList(subnets: string[]): BlockList {
  const list = new BlockList();
  for (const cidr of subnets) {
    const [addr, prefixStr] = cidr.trim().split('/');
    const family = isIP(addr);
    if (!family) throw new Error(`invalid subnet "${cidr}"`);
    const max = family === 6 ? 128 : 32;
    const prefix = prefixStr === undefined ? max : Number(prefixStr);
    if (!Number.isInteger(prefix) || prefix < 0 || prefix > max) throw new Error(`invalid subnet "${cidr}"`);
    list.addSubnet(addr, prefix, family === 6 ? 'ipv6' : 'ipv4');
  }
  return list;
}

/** True if the client IP is in the allow list. IPv6-mapped IPv4 (::ffff:10.0.0.1) is matched as IPv4. */
export function ipAllowed(ip: string, list: BlockList): boolean {
  const normalized = ip.replace(/^::ffff:(?=\d+\.\d+\.\d+\.\d+$)/, '');
  const family = isIP(normalized);
  if (!family) return false;
  return list.check(normalized, family === 6 ? 'ipv6' : 'ipv4');
}
