<script setup lang="ts">
import AppSidebar from './AppSidebar.vue';
import ConfirmDialog from './ConfirmDialog.vue';
import AuthShell from './AuthShell.vue';
import PasswordInput from './PasswordInput.vue';
import { computed, onMounted, ref } from 'vue';
import { attempt, call, loadState, session, toast, type Admin } from './api';
import CatMood from './CatMood.vue';
import ThemeSwitch from './ThemeSwitch.vue';
import MembersView from './MembersView.vue';
import WorkspacesView from './WorkspacesView.vue';
import DevicesView from './DevicesView.vue';
import ActivityView from './ActivityView.vue';
import BackupView from './BackupView.vue';
import AccountView from './AccountView.vue';
import AdminsView from './AdminsView.vue';
import icon from './assets/icon.png';
import { passkeysSupported, signInWithPasskey } from './passkey';

/** The admin panel: first-run setup, sign in, accepting an admin invite, then members, workspaces, devices (tokens + invites), admins, activity. */
type Page = 'members' | 'workspaces' | 'devices' | 'admins' | 'activity' | 'backup' | 'account';
const page = ref<Page>((location.hash.slice(1) as Page) || 'members');
const go = (p: Page) => {
  page.value = p;
  location.hash = p;
};

const form = ref({ email: '', name: '', password: '', confirm: '' });
const error = ref('');
const busy = ref(false);
const loadError = ref('');
/** Two-step sign-in: the ticket from the password step ('' when Google / GitHub left it in a cookie), and the code. */
const codeStep = ref<{ ticket: string } | null>(null);
const code = ref('');
/** An admin invite link (/?admin-invite=<code>): who it is for, and the password they choose. */
const invite = ref<{ code: string; name: string; email: string; invitedBy: string } | null>(null);
const inviteError = ref('');
const inviteForm = ref({ name: '', password: '', confirm: '' });

// Google / GitHub sign-in comes back to /?signin=<reason>.
const SIGNIN: Record<string, string> = {
  nomatch: 'No admin of this server has that email. Sign in with your password, or ask the admin to use the same email.',
  denied: 'The sign-in was cancelled.',
  expired: 'The sign-in took too long or was started in another browser. Try again.',
  failed: 'The provider did not answer as expected. Try again, or sign in with your password.',
  off: 'That sign-in is not set up on this server.',
};

onMounted(async () => {
  window.addEventListener('hashchange', () => (page.value = (location.hash.slice(1) as Page) || 'members'));
  const inviteCode = new URLSearchParams(location.search).get('admin-invite');
  if (inviteCode) {
    history.replaceState(null, '', location.pathname);
    try {
      const i = await call<{ name: string; email: string; invitedBy: string }>('GET', `/admin-invite?code=${encodeURIComponent(inviteCode)}`);
      invite.value = { code: inviteCode, ...i };
      inviteForm.value.name = i.name;
    } catch (e) {
      inviteError.value = (e as Error).message;
    }
  }
  const reason = new URLSearchParams(location.search).get('signin');
  if (reason) {
    history.replaceState(null, '', location.pathname + location.hash);
    if (reason === 'code') codeStep.value = { ticket: '' };
    else error.value = SIGNIN[reason] ?? 'The sign-in did not work.';
  }
  try {
    await loadState();
  } catch (e) {
    loadError.value = (e as Error).message;
  }
});

async function acceptInvite() {
  error.value = '';
  if (inviteForm.value.password !== inviteForm.value.confirm) {
    error.value = 'The two passwords differ';
    return;
  }
  busy.value = true;
  try {
    session.admin = await call<Admin>('POST', '/admin-invite/accept', { code: invite.value!.code, name: inviteForm.value.name, password: inviteForm.value.password });
    session.needsSetup = false;
    invite.value = null;
    go('account');
  } catch (e) {
    error.value = (e as Error).message;
  } finally {
    busy.value = false;
  }
}

async function passkeySignIn() {
  error.value = '';
  busy.value = true;
  try {
    session.admin = await signInWithPasskey();
  } catch (e) {
    error.value = (e as Error).message;
  } finally {
    busy.value = false;
  }
}

async function submit() {
  error.value = '';
  if (session.needsSetup && form.value.password !== form.value.confirm) {
    error.value = 'The two passwords differ';
    return;
  }
  busy.value = true;
  try {
    const r = await call<Admin | { needsCode: true; ticket: string }>('POST', session.needsSetup ? '/setup' : '/login', { email: form.value.email, name: form.value.name, password: form.value.password });
    form.value = { email: '', name: '', password: '', confirm: '' };
    if ('needsCode' in r) {
      codeStep.value = { ticket: r.ticket };
      code.value = '';
      return;
    }
    session.admin = r;
    session.needsSetup = false;
  } catch (e) {
    error.value = (e as Error).message;
  } finally {
    busy.value = false;
  }
}
async function submitCode() {
  error.value = '';
  busy.value = true;
  try {
    session.admin = await call<Admin>('POST', '/login/code', { ticket: codeStep.value!.ticket, code: code.value });
    codeStep.value = null;
    code.value = '';
  } catch (e) {
    error.value = (e as Error).message;
    if ((e as Error).message.includes('expired')) codeStep.value = null;
  } finally {
    busy.value = false;
  }
}
function cancelCode() {
  codeStep.value = null;
  code.value = '';
  error.value = '';
}
const PROVIDERS = { google: 'Google', github: 'GitHub' } as const;
/** Sign out asks first (a slip of the mouse in the sidebar should not end the session). */
const signingOut = ref(false);
const host = computed(() => session.publicUrl.replace(/^https?:\/\//, ''));
async function logout() {
  await attempt(() => call('POST', '/logout'));
  session.admin = null;
}
const NAV: { id: Page; label: string }[] = [
  { id: 'members', label: 'Members' },
  { id: 'workspaces', label: 'Workspaces' },
  { id: 'devices', label: 'Devices & invites' },
  { id: 'admins', label: 'Admins' },
  { id: 'activity', label: 'Activity' },
  { id: 'backup', label: 'Backup' },
  { id: 'account', label: 'Your account' },
];
</script>

<template>
  <div v-if="loadError" class="grid min-h-screen place-items-center p-6">
    <div class="alert alert-error alert-soft max-w-md"><CatMood mood="error" class="size-7 shrink-0 -my-0.5" /><span>The server did not answer: {{ loadError }}</span></div>
  </div>

  <div v-else-if="!session.ready" class="grid min-h-screen place-items-center"><span class="loading loading-dots loading-lg" /></div>

  <!-- An admin invite: choose a password -->
  <AuthShell v-else-if="invite" title="Become an admin" :subtitle="`${invite.invitedBy} invited you to manage this server`">
    <form class="space-y-4" @submit.prevent="acceptInvite">
      <p class="text-sm opacity-70">You will manage its members, workspaces and devices. Your sign-in is <b>{{ invite.email }}</b>; choose your own password.</p>
      <label class="auth-label">Your name<input v-model="inviteForm.name" class="input auth-input w-full" required autocomplete="name" /></label>
      <input :value="invite.email" type="email" class="hidden" autocomplete="username" readonly />
      <label class="auth-label">Password<PasswordInput v-model="inviteForm.password" autocomplete="new-password" :minlength="10" placeholder="At least 10 characters" /></label>
      <label class="auth-label">Password again<PasswordInput v-model="inviteForm.confirm" autocomplete="new-password" /></label>
      <div v-if="error" class="alert alert-error alert-soft text-sm"><CatMood mood="error" class="size-7 shrink-0 -my-0.5" /><span>{{ error }}</span></div>
      <button class="btn btn-primary h-11 w-full" :disabled="busy"><span v-if="busy" class="loading loading-spinner loading-xs" />Become an admin</button>
      <p v-if="session.admin" class="text-xs opacity-60">You are signed in as {{ session.admin.email }}; accepting signs you in as {{ invite.email }} instead. <a class="link" @click="invite = null">Back to the panel</a></p>
    </form>
  </AuthShell>

  <!-- Two-step sign-in: the code from the authenticator app, or a recovery code -->
  <AuthShell v-else-if="!session.admin && codeStep" title="Two-step sign-in" subtitle="Enter the 6-digit code from your authenticator app">
    <form class="space-y-4" @submit.prevent="submitCode">
      <input v-model="code" class="input auth-input w-full text-center font-mono text-xl tracking-[0.4em]" required autofocus autocomplete="one-time-code" inputmode="text" maxlength="12" placeholder="123456" aria-label="Code" />
      <p class="text-xs opacity-60">Lost the phone? A recovery code (like <code>k7m2q-x9p4t</code>) works here too, once each.</p>
      <div v-if="error" class="alert alert-error alert-soft text-sm"><CatMood mood="error" class="size-7 shrink-0 -my-0.5" /><span>{{ error }}</span></div>
      <button class="btn btn-primary h-11 w-full" :disabled="busy || !code.trim()"><span v-if="busy" class="loading loading-spinner loading-xs" />Sign in</button>
      <button type="button" class="btn btn-ghost btn-sm w-full" @click="cancelCode">Back</button>
    </form>
  </AuthShell>

  <!-- First run / sign in -->
  <AuthShell v-else-if="!session.admin" :title="session.needsSetup ? 'Set up your Den' : 'Welcome back'" :subtitle="session.needsSetup ? 'First start: create the admin account' : `Sign in to manage ${host}`">
    <form class="space-y-4" @submit.prevent="submit">
      <p v-if="session.needsSetup" class="text-sm opacity-70">This account manages members, workspaces and device tokens. Team members never sign in here — you invite them from the panel.</p>
      <template v-if="!session.needsSetup && session.passkeys && passkeysSupported()">
        <button type="button" class="btn h-11 w-full gap-2" :disabled="busy" @click="passkeySignIn"><svg viewBox="0 0 24 24" class="size-4" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><circle cx="7.5" cy="15.5" r="4.5" /><path d="m10.7 12.3 9.3-9.3M16 7l3 3M19 4l2 2" /></svg>Sign in with a passkey</button>
        <div class="divider my-1 text-xs opacity-50">or with your password</div>
      </template>
      <label v-if="session.needsSetup" class="auth-label">Your name<input v-model="form.name" class="input auth-input w-full" required autocomplete="name" placeholder="Your full name" /></label>
      <label class="auth-label">Email<input v-model="form.email" type="email" class="input auth-input w-full" required autocomplete="username" placeholder="you@company.com" /></label>
      <label class="auth-label">Password<PasswordInput v-model="form.password" :autocomplete="session.needsSetup ? 'new-password' : 'current-password'" :minlength="session.needsSetup ? 10 : undefined" :placeholder="session.needsSetup ? 'At least 10 characters' : undefined" /></label>
      <label v-if="session.needsSetup" class="auth-label">Password again<PasswordInput v-model="form.confirm" autocomplete="new-password" /></label>
      <div v-if="inviteError" class="alert alert-warning alert-soft text-sm"><CatMood mood="warning" class="size-7 shrink-0 -my-0.5" /><span>{{ inviteError }}</span></div>
      <div v-if="error" class="alert alert-error alert-soft text-sm"><CatMood mood="error" class="size-7 shrink-0 -my-0.5" /><span>{{ error }}</span></div>
      <button class="btn btn-primary h-11 w-full" :disabled="busy"><span v-if="busy" class="loading loading-spinner loading-xs" />{{ session.needsSetup ? 'Create admin account' : 'Sign in' }}</button>
      <template v-if="!session.needsSetup && session.oauth.length">
        <div class="divider my-1 text-xs opacity-50">or continue with</div>
        <div class="grid gap-2" :class="session.oauth.length > 1 ? 'grid-cols-2' : ''">
          <a v-for="p in session.oauth" :key="p" class="btn h-11" :href="`/api/admin/oauth/${p}/start`">{{ PROVIDERS[p] }}</a>
        </div>
      </template>
    </form>
  </AuthShell>

  <!-- Panel -->
  <div v-else class="flex min-h-screen">
    <AppSidebar :page="page" :nav="NAV" @go="(id) => go(id as Page)" @sign-out="signingOut = true" />
    <main class="min-w-0 flex-1">
      <MembersView v-if="page === 'members'" />
      <WorkspacesView v-else-if="page === 'workspaces'" />
      <DevicesView v-else-if="page === 'devices'" />
      <AdminsView v-else-if="page === 'admins'" />
      <ActivityView v-else-if="page === 'activity'" />
      <BackupView v-else-if="page === 'backup'" />
      <AccountView v-else />
    </main>
  </div>

  <ConfirmDialog
    v-if="signingOut && session.admin"
    title="Sign out?"
    :text="`You will need your password or a passkey to sign in to ${host} again. Members and their devices are not affected.`"
    confirm="Sign out"
    tone="primary"
    @ok="((signingOut = false), logout())"
    @cancel="signingOut = false"
  />

  <div v-if="toast.text" class="toast toast-end toast-bottom z-50">
    <div class="alert" :class="`alert-${toast.kind}`"><CatMood :mood="toast.kind" class="size-8 shrink-0 -my-1" /><span class="max-w-md text-sm">{{ toast.text }}</span></div>
  </div>
</template>
