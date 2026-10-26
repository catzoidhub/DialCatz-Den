import { reactive } from 'vue';

/** The admin API (same origin). Every call carries X-DialCatz, which the server requires against CSRF. */

export interface Admin { id: string; email: string; name: string; createdAt: number; twoFactor: boolean }
export interface Membership { id: string; name: string; role: Role }
export type Role = 'owner' | 'editor' | 'viewer';
/** avatarUrl is the admin's picture, or (gravatar: true) Gravatar's for the email, which answers 404 when there is none. */
export interface Member { id: string; name: string; email: string; avatarUrl: string; gravatar: boolean; disabled: boolean; createdAt: number; workspaces: Membership[]; devices: number }
export interface Workspace { id: string; name: string; revision: number; createdAt: number; members: number; items: number }
/** A token's / invite's workspaces; null = every workspace of the member. */
export type Scope = { id: string; name: string }[] | null;
export interface Token { id: string; memberId: string; memberName: string; device: string; prefix: string; createdAt: number; expiresAt: number | null; lastUsedAt: number | null; revokedAt: number | null; scope: Scope }
export interface Invite { id: string; memberId: string; memberName: string; createdAt: number; expiresAt: number; usedAt: number | null; scope: Scope }
export type OAuthProvider = 'google' | 'github';
export interface AuditEntry { id: string; at: number; actor: string; action: string; detail: string }

export class ApiError extends Error {
  constructor(message: string, public status: number) {
    super(message);
  }
}

export async function call<T>(method: string, path: string, body?: unknown): Promise<T> {
  const res = await fetch(`/api/admin${path}`, {
    method,
    headers: { 'X-DialCatz': '1', ...(body !== undefined ? { 'Content-Type': 'application/json' } : {}) },
    body: body !== undefined ? JSON.stringify(body) : undefined,
    credentials: 'same-origin',
  });
  const json = await res.json().catch(() => null);
  if (!res.ok) {
    if (res.status === 401 && !path.startsWith('/login') && !path.startsWith('/2fa')) session.admin = null;
    throw new ApiError((json as { error?: string } | null)?.error ?? `HTTP ${res.status}`, res.status);
  }
  return json as T;
}

export const session = reactive<{ ready: boolean; needsSetup: boolean; admin: Admin | null; publicUrl: string; oauth: OAuthProvider[]; passkeys: boolean }>({ ready: false, needsSetup: false, admin: null, publicUrl: '', oauth: [], passkeys: false });

export async function loadState(): Promise<void> {
  const s = await call<{ needsSetup: boolean; admin: Admin | null; publicUrl: string; oauth: OAuthProvider[]; passkeys: boolean }>('GET', '/state');
  Object.assign(session, s, { ready: true });
}

/** How a token's / invite's scope reads in a table. */
export const scopeText = (s: Scope) => (s === null ? 'All workspaces' : s.length ? s.map((w) => w.name).join(', ') : 'No workspace');

export const toast = reactive<{ kind: 'success' | 'error'; text: string }>({ kind: 'success', text: '' });
let timer: ReturnType<typeof setTimeout> | null = null;
export function notify(kind: 'success' | 'error', text: string) {
  Object.assign(toast, { kind, text });
  if (timer) clearTimeout(timer);
  timer = setTimeout(() => (toast.text = ''), kind === 'error' ? 6000 : 3000);
}
export async function attempt<T>(fn: () => Promise<T>, ok?: string): Promise<T | undefined> {
  try {
    const r = await fn();
    if (ok) notify('success', ok);
    return r;
  } catch (e) {
    notify('error', (e as Error).message);
    return undefined;
  }
}

export const when = (ms: number | null) => (ms ? new Date(ms).toLocaleString() : '—');
export function ago(ms: number | null): string {
  if (!ms) return 'never';
  const s = Math.round((Date.now() - ms) / 1000);
  if (s < 60) return 'just now';
  if (s < 3600) return `${Math.round(s / 60)} min ago`;
  if (s < 86400) return `${Math.round(s / 3600)} h ago`;
  return `${Math.round(s / 86400)} d ago`;
}
export const initials = (name: string) => name.split(/\s+/).map((p) => p[0] ?? '').join('').slice(0, 2).toUpperCase();
