<script setup lang="ts">
import { onMounted, ref } from 'vue';
import QRCode from 'qrcode';
import { ago, attempt, call, notify, session, when } from './api';
import { addPasskey, passkeysSupported } from './passkey';
import ConfirmDialog from './ConfirmDialog.vue';
import CatMood from './CatMood.vue';
import ThemeSwitch from './ThemeSwitch.vue';

/** The admin's own account: theme, password (signs out every other session), passkeys, two-step sign-in. */
const current = ref('');
const next = ref('');
const again = ref('');
const error = ref('');

async function save() {
  error.value = '';
  if (next.value !== again.value) {
    error.value = 'The two new passwords differ';
    return;
  }
  if (await attempt(() => call('POST', '/password', { current: current.value, new: next.value }), 'Password changed')) {
    current.value = next.value = again.value = '';
  }
}

// ------------------------------------------------------------------ passkeys

interface PasskeyRow {
  id: string;
  name: string;
  createdAt: number;
  lastUsedAt: number | null;
}
const passkeys = ref<{ enabled: boolean; rpId?: string; passkeys: PasskeyRow[] } | null>(null);
const naming = ref<string | null>(null);
const adding = ref(false);
const pkError = ref('');
const removing = ref<PasskeyRow | null>(null);
const loadPasskeys = async () => (passkeys.value = await call('GET', '/passkeys'));
onMounted(loadPasskeys);
/** A name to start from: what this device is. */
function guessName(): string {
  const ua = navigator.userAgent;
  if (/iPhone/.test(ua)) return 'iPhone';
  if (/iPad/.test(ua)) return 'iPad';
  if (/Android/.test(ua)) return 'Android phone';
  if (/Mac OS X/.test(ua)) return 'Mac';
  if (/Windows/.test(ua)) return 'Windows PC';
  return 'This device';
}
async function add() {
  pkError.value = '';
  adding.value = true;
  try {
    await addPasskey(naming.value?.trim() || guessName());
    naming.value = null;
    notify('success', 'Passkey added — you can sign in with it now');
    await loadPasskeys();
  } catch (e) {
    pkError.value = (e as Error).message;
  } finally {
    adding.value = false;
  }
}
async function remove() {
  const p = removing.value!;
  removing.value = null;
  if (await attempt(() => call('DELETE', `/passkeys/${p.id}`), `“${p.name}” removed`)) await loadPasskeys();
}

// ------------------------------------------------------------------ two-step sign-in (TOTP)

const twoFactor = ref<{ enabled: boolean; recoveryCodesLeft: number } | null>(null);
/** Setting up: the secret as a QR code and as text, waiting for the first code. */
const setup = ref<{ secret: string; uri: string; qr: string } | null>(null);
const setupCode = ref('');
const recovery = ref<string[] | null>(null);
const disabling = ref<{ password: string; code: string } | null>(null);
const tfError = ref('');

const loadTwoFactor = async () => (twoFactor.value = await call('GET', '/2fa'));
onMounted(loadTwoFactor);

async function start() {
  tfError.value = '';
  const r = await attempt(() => call<{ secret: string; uri: string }>('POST', '/2fa/start'));
  if (r) {
    setup.value = { ...r, qr: await QRCode.toDataURL(r.uri, { margin: 1, width: 200 }) };
    setupCode.value = '';
  }
}
async function confirm() {
  tfError.value = '';
  try {
    const r = await call<{ recoveryCodes: string[] }>('POST', '/2fa/confirm', { code: setupCode.value });
    setup.value = null;
    recovery.value = r.recoveryCodes;
    if (session.admin) session.admin.twoFactor = true;
    await loadTwoFactor();
  } catch (e) {
    tfError.value = (e as Error).message;
  }
}
async function disable() {
  tfError.value = '';
  try {
    await call('POST', '/2fa/disable', disabling.value);
    disabling.value = null;
    if (session.admin) session.admin.twoFactor = false;
    notify('success', 'Two-step sign-in is off');
    await loadTwoFactor();
  } catch (e) {
    tfError.value = (e as Error).message;
  }
}
async function copy(text: string, what: string) {
  await navigator.clipboard.writeText(text);
  notify('success', `${what} copied`);
}
</script>

<template>
  <div class="mx-auto max-w-2xl space-y-5 p-8">
    <header>
      <h1 class="text-xl font-semibold tracking-tight">Your account</h1>
      <p class="text-sm opacity-60">{{ session.admin?.name }} · {{ session.admin?.email }}</p>
    </header>

    <div class="card">
      <div class="card-body">
        <h2>Appearance</h2>
        <div class="opt-row">
          <span class="flex flex-col"><span class="label-caps">Theme</span><span class="text-xs opacity-50">For this browser. Auto follows your computer’s setting.</span></span>
          <ThemeSwitch />
        </div>
      </div>
    </div>

    <form class="card" @submit.prevent="save">
      <div class="card-body">
        <h2>Change password</h2>
        <div class="opt-row"><span class="label-caps">Current</span><input v-model="current" type="password" class="input input-sm flex-1" required autocomplete="current-password" /></div>
        <div class="opt-row"><span class="label-caps">New</span><input v-model="next" type="password" class="input input-sm flex-1" required minlength="10" autocomplete="new-password" /></div>
        <div class="opt-row"><span class="label-caps">New again</span><input v-model="again" type="password" class="input input-sm flex-1" required autocomplete="new-password" /></div>
        <p v-if="error" class="text-sm text-error">{{ error }}</p>
        <p class="text-xs opacity-50">Other signed-in browsers are signed out. Members’ devices are not affected.</p>
        <div class="flex justify-end"><button class="btn btn-primary btn-sm">Change password</button></div>
      </div>
    </form>

    <div class="card">
      <div class="card-body">
        <div class="flex items-center gap-3">
          <h2 class="flex-1">Passkeys</h2>
          <span v-if="passkeys?.passkeys.length" class="badge badge-soft badge-success badge-sm">{{ passkeys.passkeys.length }}</span>
        </div>
        <p class="text-sm opacity-70">Sign in with Touch ID, Face ID, Windows Hello, your phone or a security key instead of your password — no code needed afterwards. A passkey cannot be phished: it only works on this server’s address.</p>
        <div v-if="passkeys && !passkeys.enabled" class="alert alert-info alert-soft text-sm"><CatMood mood="info" class="size-7 shrink-0 -my-0.5" /><span>Passkeys need the server on an <b>https://</b> address with a domain name (set DIALCATZ_PUBLIC_URL to it).</span></div>
        <div v-else-if="!passkeysSupported()" class="alert alert-warning alert-soft text-sm"><CatMood mood="warning" class="size-7 shrink-0 -my-0.5" /><span>This browser cannot use passkeys.</span></div>
        <template v-else-if="passkeys">
          <div v-for="p in passkeys.passkeys" :key="p.id" class="opt-row">
            <span class="flex min-w-0 items-center gap-2">
              <svg viewBox="0 0 24 24" class="size-4 shrink-0 opacity-60" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><circle cx="7.5" cy="15.5" r="4.5" /><path d="m10.7 12.3 9.3-9.3M16 7l3 3M19 4l2 2" /></svg>
              <span class="truncate text-sm font-medium">{{ p.name }}</span>
              <span class="text-xs opacity-50">added {{ when(p.createdAt) }} · used {{ ago(p.lastUsedAt) }}</span>
            </span>
            <button class="btn btn-ghost btn-xs text-error" @click="removing = p">Remove</button>
          </div>
          <div v-if="naming !== null" class="flex items-center gap-2">
            <input v-model="naming" class="input input-sm flex-1" :placeholder="guessName()" maxlength="60" @keydown.enter="add" @keydown.esc="naming = null" />
            <button class="btn btn-primary btn-sm" :disabled="adding" @click="add"><span v-if="adding" class="loading loading-spinner loading-xs" />Continue</button>
            <button class="btn btn-ghost btn-sm" :disabled="adding" @click="naming = null">Cancel</button>
          </div>
          <p v-if="pkError" class="text-sm text-error">{{ pkError }}</p>
          <div v-if="naming === null" class="flex items-center justify-between gap-3">
            <span class="text-xs opacity-50">{{ passkeys.passkeys.length ? 'Add one per device you sign in from.' : 'No passkey yet.' }} Bound to {{ passkeys.rpId }}.</span>
            <button class="btn btn-primary btn-sm" @click="naming = ''">Add a passkey</button>
          </div>
        </template>
      </div>
    </div>

    <div class="card">
      <div class="card-body">
        <div class="flex items-center gap-3">
          <h2 class="flex-1">Two-step sign-in</h2>
          <span v-if="twoFactor?.enabled" class="badge badge-soft badge-success badge-sm">on</span>
          <span v-else-if="twoFactor" class="badge badge-ghost badge-sm">off</span>
        </div>

        <!-- Recovery codes, shown once right after turning it on -->
        <template v-if="recovery">
          <div class="alert alert-warning alert-soft text-sm"><CatMood mood="warning" class="size-7 shrink-0 -my-0.5" /><span>Save these recovery codes somewhere safe. Each works once if you lose your phone; this is the only time they are shown.</span></div>
          <div class="secret-box grid grid-cols-2 gap-x-6 font-mono">
            <span v-for="c in recovery" :key="c">{{ c }}</span>
          </div>
          <div class="flex justify-end gap-2">
            <button class="btn btn-sm" @click="copy(recovery.join('\n'), 'Recovery codes')">Copy</button>
            <button class="btn btn-primary btn-sm" @click="recovery = null">I saved them</button>
          </div>
        </template>

        <!-- Setting up: scan, then confirm with the first code -->
        <form v-else-if="setup" class="space-y-3" @submit.prevent="confirm">
          <p class="text-sm opacity-70">Scan this with an authenticator app (1Password, Google Authenticator, Authy, …), then enter the code it shows.</p>
          <div class="flex items-start gap-4">
            <img :src="setup.qr" alt="QR code for the authenticator app" class="size-40 shrink-0 rounded-lg bg-white p-1" />
            <div class="min-w-0 space-y-1">
              <span class="label-caps">Or type this key</span>
              <div class="secret-box font-mono break-all">{{ setup.secret }}</div>
              <button type="button" class="btn btn-ghost btn-xs" @click="copy(setup.secret, 'Key')">Copy key</button>
            </div>
          </div>
          <div class="opt-row"><span class="label-caps">Code</span><input v-model="setupCode" class="input input-sm flex-1 font-mono tracking-widest" required inputmode="numeric" maxlength="6" autocomplete="one-time-code" placeholder="123456" /></div>
          <p v-if="tfError" class="text-sm text-error">{{ tfError }}</p>
          <div class="flex justify-end gap-2">
            <button type="button" class="btn btn-ghost btn-sm" @click="setup = null">Cancel</button>
            <button class="btn btn-primary btn-sm" :disabled="setupCode.trim().length !== 6">Turn on</button>
          </div>
        </form>

        <template v-else-if="twoFactor?.enabled">
          <p class="text-sm opacity-70">Signing in asks for a code from your authenticator app after the password (and after Google / GitHub). {{ twoFactor.recoveryCodesLeft }} recovery code{{ twoFactor.recoveryCodesLeft === 1 ? '' : 's' }} left.</p>
          <div class="flex justify-end"><button class="btn btn-sm" @click="(disabling = { password: '', code: '' }), (tfError = '')">Turn off</button></div>
        </template>

        <template v-else-if="twoFactor">
          <p class="text-sm opacity-70">Ask for a code from an authenticator app on your phone each time you sign in, so a leaked password alone is not enough.</p>
          <div class="flex justify-end"><button class="btn btn-primary btn-sm" @click="start">Set up two-step sign-in</button></div>
        </template>
      </div>
    </div>

    <dialog v-if="disabling" open class="modal modal-open" @keydown.esc="disabling = null">
      <form class="modal-box w-[28rem] max-w-[95vw] space-y-3" @submit.prevent="disable">
        <h3 class="text-lg font-semibold tracking-tight">Turn off two-step sign-in?</h3>
        <p class="text-sm opacity-70">Your password alone will sign you in again. Confirm with both.</p>
        <div class="opt-row"><span class="label-caps">Password</span><input v-model="disabling.password" type="password" class="input input-sm flex-1" required autocomplete="current-password" /></div>
        <div class="opt-row"><span class="label-caps">Code</span><input v-model="disabling.code" class="input input-sm flex-1 font-mono" required autocomplete="one-time-code" placeholder="123456 or a recovery code" /></div>
        <p v-if="tfError" class="text-sm text-error">{{ tfError }}</p>
        <div class="modal-action"><button type="button" class="btn btn-ghost btn-sm" @click="disabling = null">Cancel</button><button class="btn btn-error btn-sm">Turn off</button></div>
      </form>
      <div class="modal-backdrop" @click="disabling = null" />
    </dialog>
    <ConfirmDialog v-if="removing" :title="`Remove the passkey “${removing.name}”?`" text="It can no longer sign in to this server. Your password (and two-step sign-in) still work." confirm="Remove passkey" tone="error" @ok="remove" @cancel="removing = null" />
  </div>
</template>
