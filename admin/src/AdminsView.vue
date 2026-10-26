<script setup lang="ts">
import { onMounted, ref } from 'vue';
import { ago, attempt, call, initials, notify, when, type Admin } from './api';
import CatMood from './CatMood.vue';
import ConfirmDialog from './ConfirmDialog.vue';
import ListPager from './ListPager.vue';
import ListTools from './ListTools.vue';
import { useList } from './list';

/**
 * Admins: the people who manage this server (members, workspaces, devices). Each has their own sign-in, so a
 * team never shares one password. Invite one with a one-time link; they choose their own password.
 */
interface AdminRow extends Admin {
  passkeys: number;
}
interface Invite {
  id: string;
  email: string;
  name: string;
  invitedBy: string;
  createdAt: number;
  expiresAt: number;
}
const admins = ref<AdminRow[]>([]);
const L = useList(admins, (a) => `${a.name} ${a.email}`, [
  { id: 'all', label: 'All', test: () => true },
  { id: 'twostep', label: 'Two-step on', test: (a) => a.twoFactor },
  { id: 'password', label: 'Password only', test: (a) => !a.twoFactor && !a.passkeys },
  { id: 'passkeys', label: 'With passkeys', test: (a) => a.passkeys > 0 },
]);
const { search, filter, page } = L;
const invites = ref<Invite[]>([]);
const me = ref('');
const inviting = ref<{ name: string; email: string } | null>(null);
const link = ref<{ url: string; name: string; expiresAt: number } | null>(null);
const error = ref('');
const removing = ref<AdminRow | null>(null);
const typed = ref('');

async function load() {
  const r = await call<{ admins: AdminRow[]; invites: Invite[]; me: string }>('GET', '/admins');
  admins.value = r.admins;
  invites.value = r.invites;
  me.value = r.me;
}
onMounted(load);

async function invite() {
  error.value = '';
  try {
    const r = await call<{ link: string; invite: Invite }>('POST', '/admins/invites', inviting.value);
    inviting.value = null;
    link.value = { url: r.link, name: r.invite.name, expiresAt: r.invite.expiresAt };
    await load();
  } catch (e) {
    error.value = (e as Error).message;
  }
}
async function copy(text: string, what: string) {
  await navigator.clipboard.writeText(text);
  notify('success', `${what} copied`);
}
async function cancel(i: Invite) {
  if (await attempt(() => call('DELETE', `/admins/invites/${i.id}`), 'Invite cancelled')) await load();
}
function askRemove(a: AdminRow) {
  typed.value = '';
  removing.value = a;
}
async function remove() {
  const a = removing.value!;
  if (typed.value.trim() !== a.name.trim()) return;
  removing.value = null;
  if (await attempt(() => call('DELETE', `/admins/${a.id}`), `${a.name} is no longer an admin`)) await load();
}
</script>

<template>
  <div class="mx-auto max-w-4xl space-y-5 p-8">
    <header class="flex items-end gap-4">
      <div class="flex-1">
        <h1 class="text-xl font-semibold tracking-tight">Admins</h1>
        <p class="text-sm opacity-60">Who manages this server. Each admin has their own sign-in — invite people instead of sharing a password.</p>
      </div>
      <button class="btn btn-primary btn-sm" @click="((inviting = { name: '', email: '' }), (error = ''))">Invite an admin</button>
    </header>
    <ListTools v-model:filter="filter" v-model:search="search" :filters="L.filters" :counts="L.counts.value" placeholder="Search name or email" />

    <div class="card"><div class="card-body card-table">
      <table class="table">
        <thead><tr><th>Admin</th><th>Sign-in</th><th>Since</th><th /></tr></thead>
        <tbody>
          <tr v-if="!L.shown.value.length"><td colspan="4" class="py-8 text-center opacity-50">No admin matches.</td></tr>
          <tr v-for="a in L.shown.value" :key="a.id">
            <td>
              <div class="flex items-center gap-3">
                <span class="avatar-circle">{{ initials(a.name) }}</span>
                <div class="min-w-0">
                  <p class="truncate font-medium">{{ a.name }} <span v-if="a.id === me" class="badge badge-ghost badge-sm">you</span></p>
                  <p class="truncate text-xs opacity-60">{{ a.email }}</p>
                </div>
              </div>
            </td>
            <td class="space-x-1 text-xs">
              <span class="badge badge-sm" :class="a.twoFactor ? 'badge-soft badge-success' : 'badge-ghost'">{{ a.twoFactor ? 'two-step on' : 'password only' }}</span>
              <span v-if="a.passkeys" class="badge badge-soft badge-success badge-sm">{{ a.passkeys }} passkey{{ a.passkeys === 1 ? '' : 's' }}</span>
            </td>
            <td class="text-xs opacity-60">{{ when(a.createdAt) }}</td>
            <td class="text-right">
              <button v-if="a.id !== me && admins.length > 1" class="btn btn-ghost btn-xs text-error" @click="askRemove(a)">Remove</button>
            </td>
          </tr>
        </tbody>
      </table>
    </div></div>
    <ListPager v-model="page" :pages="L.pages.value" :range="L.range.value" />

    <section v-if="invites.length" class="space-y-2">
      <h2 class="label-caps">Open invites</h2>
      <div class="card"><div class="card-body card-table">
        <table class="table">
          <tbody>
            <tr v-for="i in invites" :key="i.id">
              <td><p class="font-medium">{{ i.name }}</p><p class="text-xs opacity-60">{{ i.email }}</p></td>
              <td class="text-xs opacity-60">invited by {{ i.invitedBy }} · {{ ago(i.createdAt) }}</td>
              <td class="text-xs">works until {{ when(i.expiresAt) }}</td>
              <td class="text-right"><button class="btn btn-ghost btn-xs" @click="cancel(i)">Cancel</button></td>
            </tr>
          </tbody>
        </table>
      </div></div>
    </section>

    <!-- Invite -->
    <dialog v-if="inviting" open class="modal modal-open" @keydown.esc="inviting = null">
      <form class="modal-box w-[30rem] max-w-[95vw] space-y-3" @submit.prevent="invite">
        <h3 class="text-lg font-semibold tracking-tight">Invite an admin</h3>
        <p class="text-sm opacity-70">They get a one-time link (48 hours) and choose their own password. Admins can manage everything here, including other admins.</p>
        <div class="opt-row"><span class="label-caps">Name</span><input v-model="inviting.name" class="input input-sm flex-1" required placeholder="Budi Santoso" /></div>
        <div class="opt-row"><span class="label-caps">Email</span><input v-model="inviting.email" type="email" class="input input-sm flex-1" required placeholder="budi@company.com" /></div>
        <p class="text-xs opacity-50">The email is their sign-in name (and matches Google / GitHub sign-in when that is set up).</p>
        <p v-if="error" class="text-sm text-error">{{ error }}</p>
        <div class="modal-action"><button type="button" class="btn btn-ghost btn-sm" @click="inviting = null">Cancel</button><button class="btn btn-primary btn-sm">Create invite link</button></div>
      </form>
      <div class="modal-backdrop" @click="inviting = null" />
    </dialog>

    <!-- The link, once -->
    <dialog v-if="link" open class="modal modal-open" @keydown.esc="link = null">
      <div class="modal-box w-[34rem] max-w-[95vw] space-y-3">
        <h3 class="text-lg font-semibold tracking-tight">Invite link for {{ link.name }}</h3>
        <div class="alert alert-success alert-soft text-sm"><CatMood mood="success" class="size-7 shrink-0 -my-0.5" /><span>Send it to {{ link.name.split(' ')[0] }}. It works once, until {{ when(link.expiresAt) }}.</span></div>
        <div class="secret-box">{{ link.url }}</div>
        <div class="modal-action"><button class="btn btn-sm" @click="copy(link.url, 'Link')">Copy link</button><button class="btn btn-primary btn-sm" @click="link = null">Done</button></div>
      </div>
      <div class="modal-backdrop" @click="link = null" />
    </dialog>

    <ConfirmDialog
      v-if="removing"
      :title="`Remove ${removing.name} as an admin?`"
      text="They are signed out everywhere and can no longer sign in to this panel. Their passkeys and two-step codes are deleted. Members and devices are not touched."
      confirm="Remove admin"
      tone="error"
      :blocked="typed.trim() !== removing.name.trim()"
      @ok="remove"
      @cancel="removing = null"
    >
      <div class="space-y-1.5">
        <p class="flex flex-wrap items-center gap-1 text-sm">Type <b>{{ removing.name }}</b><button type="button" class="btn btn-ghost btn-xs btn-square" title="Copy the name" aria-label="Copy the name" @click="copy(removing.name, 'Name')"><svg viewBox="0 0 24 24" class="size-3.5" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M20 9h-9a2 2 0 0 0-2 2v9a2 2 0 0 0 2 2h9a2 2 0 0 0 2-2v-9a2 2 0 0 0-2-2ZM5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1" /></svg></button>to confirm:</p>
        <input v-model="typed" class="input input-sm w-full" :placeholder="removing.name" autocomplete="off" @keydown.enter="remove" />
      </div>
    </ConfirmDialog>
  </div>
</template>
