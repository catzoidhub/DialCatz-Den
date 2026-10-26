import { call, type Admin } from './api';

/**
 * Passkeys in the browser. The server sends WebAuthn options as JSON (binary fields base64url-encoded);
 * navigator.credentials wants ArrayBuffers, and the answer goes back as JSON the same way.
 */

export const passkeysSupported = () => typeof window !== 'undefined' && !!window.PublicKeyCredential && !!navigator.credentials;

const fromB64url = (s: string): ArrayBuffer => {
  const b = atob(s.replace(/-/g, '+').replace(/_/g, '/').padEnd(Math.ceil(s.length / 4) * 4, '='));
  const out = new Uint8Array(b.length);
  for (let i = 0; i < b.length; i++) out[i] = b.charCodeAt(i);
  return out.buffer;
};
const toB64url = (buf: ArrayBuffer | null | undefined): string => {
  if (!buf) return '';
  let s = '';
  for (const b of new Uint8Array(buf)) s += String.fromCharCode(b);
  return btoa(s).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '');
};

type Descriptor = { id: string; type: string; transports?: string[] };
const descriptors = (list?: Descriptor[]) => (list ?? []).map((d) => ({ ...d, id: fromB64url(d.id) })) as PublicKeyCredentialDescriptor[];

/** The browser's / OS's own words for a cancelled or refused prompt, in plain English. */
function friendly(e: unknown): Error {
  const name = (e as DOMException)?.name;
  if (name === 'NotAllowedError') return new Error('The passkey prompt was closed or timed out');
  if (name === 'InvalidStateError') return new Error('This device already has a passkey for this account');
  if (name === 'SecurityError') return new Error('Passkeys only work on the server’s own https:// address');
  return e instanceof Error ? e : new Error(String(e));
}

/** Registers a new passkey for the signed-in admin. */
export async function addPasskey(name: string): Promise<void> {
  const { token, options } = await call<{ token: string; options: { publicKey: Record<string, unknown> & { challenge: string; user: { id: string }; excludeCredentials?: Descriptor[] } } }>('POST', '/passkeys/register/begin');
  const pk = options.publicKey;
  let cred: PublicKeyCredential | null;
  try {
    cred = (await navigator.credentials.create({
      publicKey: { ...pk, challenge: fromB64url(pk.challenge), user: { ...pk.user, id: fromB64url(pk.user.id) }, excludeCredentials: descriptors(pk.excludeCredentials) } as unknown as PublicKeyCredentialCreationOptions,
    })) as PublicKeyCredential | null;
  } catch (e) {
    throw friendly(e);
  }
  if (!cred) throw new Error('No passkey was made');
  const r = cred.response as AuthenticatorAttestationResponse;
  await call('POST', '/passkeys/register/finish', {
    token,
    name,
    credential: {
      id: cred.id,
      rawId: toB64url(cred.rawId),
      type: cred.type,
      authenticatorAttachment: cred.authenticatorAttachment ?? undefined,
      clientExtensionResults: cred.getClientExtensionResults(),
      response: { clientDataJSON: toB64url(r.clientDataJSON), attestationObject: toB64url(r.attestationObject), transports: typeof r.getTransports === 'function' ? r.getTransports() : [] },
    },
  });
}

/** Signs in with any passkey this device has for the server. */
export async function signInWithPasskey(): Promise<Admin> {
  const { token, options } = await call<{ token: string; options: { publicKey: Record<string, unknown> & { challenge: string; allowCredentials?: Descriptor[] } } }>('POST', '/login/passkey/begin');
  const pk = options.publicKey;
  let cred: PublicKeyCredential | null;
  try {
    cred = (await navigator.credentials.get({ publicKey: { ...pk, challenge: fromB64url(pk.challenge), allowCredentials: descriptors(pk.allowCredentials) } as unknown as PublicKeyCredentialRequestOptions })) as PublicKeyCredential | null;
  } catch (e) {
    throw friendly(e);
  }
  if (!cred) throw new Error('No passkey was chosen');
  const r = cred.response as AuthenticatorAssertionResponse;
  return call<Admin>('POST', '/login/passkey/finish', {
    token,
    credential: {
      id: cred.id,
      rawId: toB64url(cred.rawId),
      type: cred.type,
      authenticatorAttachment: cred.authenticatorAttachment ?? undefined,
      clientExtensionResults: cred.getClientExtensionResults(),
      response: { clientDataJSON: toB64url(r.clientDataJSON), authenticatorData: toB64url(r.authenticatorData), signature: toB64url(r.signature), userHandle: toB64url(r.userHandle) },
    },
  });
}
