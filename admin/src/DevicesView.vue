<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { ago, attempt, call, notify, scopeText, when, type Invite, type Member, type Token } from './api';
import ConfirmDialog from './ConfirmDialog.vue';
import ListPager from './ListPager.vue';
import ListTools from './ListTools.vue';
import { useList } from './list';
import CatMood from './CatMood.vue';
import ScopePicker from './ScopePicker.vue';

/**
 * Devices: every member token (one per device), with its last use; revoke a lost laptop alone. Open invite links
 * are listed too. A token can also be made by hand (shown once) for a device that cannot open the link.
 */
const tokens = ref<Token[]>([]);
const invites = ref<Invite[]>([]);
const members = ref<Member[]>([]);
const revoking = ref<Token | null>(null);
const manual = ref<{ memberId: string; device: string; expiryDays: number; workspaceIds: string[] | null } | null>(null);
const manualMember = computed(() => members.value.find((m) => m.id === manual.value?.memberId));
const created = ref<string | null>(null);

async function load() {
  ;[tokens.value, invites.value, members.value] = await Promise.all([call<Token[]>('GET', '/tokens'), call<Invite[]>('GET', '/invites'), call<Member[]>('GET', '/members')]);
}
onMounted(load);

const expired = (t: Token) => t.expiresAt !== null && t.expiresAt < Date.now();
const L = useList(tokens, (t) => `${t.memberName} ${t.device} ${t.prefix} ${scopeText(t.scope)}`, [
  { id: 'active', label: 'Active', test: (t) => !t.revokedAt && !expired(t) },
  { id: 'expired', label: 'Expired', test: (t) => !t.revokedAt && expired(t) },
  { id: 'revoked', label: 'Revoked', test: (t) => !!t.revokedAt },
  { id: 'all', label: 'All', test: () => true },
]);
const { search, filter, page } = L;

async function revoke() {
  const t = revoking.value!;
  revoking.value = null;
  if (await attempt(() => call('DELETE', `/tokens/${t.id}`), `${t.device} revoked`)) await load();
}
async function cancelInvite(i: Invite) {
  if (await attempt(() => call('DELETE', `/invites/${i.id}`), 'Invite cancelled')) await load();
}
async function createManual() {
  const m = manual.value!;
  const r = await attempt(() => call<{ token: string }>('POST', '/tokens', { ...m, workspaceIds: m.workspaceIds ?? [] }));
  if (r) {
    manual.value = null;
    created.value = r.token;
    await load();
  }
}
async function copy(t: string) {
  await navigator.clipboard.writeText(t);
  notify('success', 'Token copied');
}
</script>

<template>
  <div class="mx-auto max-w-5xl space-y-5 p-8">
    <header class="flex items-end gap-4">
      <div class="flex-1">
        <h1 class="text-xl font-semibold tracking-tight">Devices &amp; invites</h1>
        <p class="text-sm opacity-60">One token per device. Revoke one and only that device is locked out.</p>
      </div>
      <button class="btn btn-sm" :disabled="!members.length" @click="manual = { memberId: members[0]?.id ?? '', device: '', expiryDays: 0, workspaceIds: null }">Make a token by hand</button>
    </header>
    <ListTools v-model:filter="filter" v-model:search="search" :filters="L.filters" :counts="L.counts.value" placeholder="Search member, device or token" />

    <div class="card"><div class="card-body card-table">
      <table class="table">
        <thead><tr><th>Member</th><th>Device</th><th>Workspaces</th><th>Token</th><th>Last used</th><th>Expires</th><th /></tr></thead>
        <tbody>
          <tr v-if="!L.shown.value.length"><td colspan="7" class="py-8 text-center opacity-50">{{ tokens.length ? 'No device matches.' : 'No devices yet. Invite a member from the Members page.' }}</td></tr>
          <tr v-for="t in L.shown.value" :key="t.id" :class="{ 'opacity-50': t.revokedAt || expired(t) }">
            <td class="font-medium">{{ t.memberName }}</td>
            <td>{{ t.device }}</td>
            <td class="max-w-48 truncate text-xs" :class="{ 'opacity-60': t.scope === null }" :title="scopeText(t.scope)">{{ scopeText(t.scope) }}</td>
            <td class="font-mono text-xs">{{ t.prefix }}…</td>
            <td class="text-xs" :title="when(t.lastUsedAt)">{{ ago(t.lastUsedAt) }}</td>
            <td class="text-xs">{{ t.revokedAt ? 'revoked' : expired(t) ? 'expired' : t.expiresAt ? when(t.expiresAt) : 'never' }}</td>
            <td class="text-right"><button v-if="!t.revokedAt" class="btn btn-ghost btn-xs text-error" @click="revoking = t">Revoke</button></td>
          </tr>
        </tbody>
      </table>
    </div></div>
    <ListPager v-model="page" :pages="L.pages.value" :range="L.range.value" />

    <section v-if="invites.length" class="space-y-2">
      <h2 class="label-caps">Open invite links</h2>
      <div class="card"><div class="card-body card-table">
        <table class="table">
          <tbody>
            <tr v-for="i in invites" :key="i.id">
              <td class="font-medium">{{ i.memberName }}</td>
              <td class="max-w-48 truncate text-xs" :class="{ 'opacity-60': i.scope === null }" :title="scopeText(i.scope)">{{ scopeText(i.scope) }}</td>
              <td class="text-xs opacity-60">created {{ ago(i.createdAt) }}</td>
              <td class="text-xs">works until {{ when(i.expiresAt) }}</td>
              <td class="text-right"><button class="btn btn-ghost btn-xs" @click="cancelInvite(i)">Cancel</button></td>
            </tr>
          </tbody>
        </table>
      </div></div>
    </section>

    <dialog v-if="manual" open class="modal modal-open" @keydown.esc="manual = null">
      <form class="modal-box w-[30rem] max-w-[95vw] space-y-3" @submit.prevent="createManual">
        <h3 class="text-lg font-semibold">Make a token by hand</h3>
        <p class="text-sm opacity-70">Prefer invite links: a raw token has to travel to the device somehow. Use this for a device that cannot open a dialcatz:// link.</p>
        <div class="opt-row"><span class="label-caps">Member</span>
          <select v-model="manual.memberId" class="select select-sm flex-1" @change="manual.workspaceIds = null"><option v-for="m in members" :key="m.id" :value="m.id">{{ m.name }}</option></select>
        </div>
        <div class="opt-row"><span class="label-caps">Device</span><input v-model="manual.device" class="input input-sm flex-1" placeholder="Andi – office PC" required /></div>
        <div class="opt-row"><span class="label-caps">Expires</span>
          <div class="seg seg-choice"><button v-for="d in [0, 90, 365]" :key="d" type="button" class="seg-item normal-case" :class="{ 'is-active': manual.expiryDays === d }" @click="manual.expiryDays = d">{{ d === 0 ? 'Never' : `${d} days` }}</button></div>
        </div>
        <ScopePicker v-if="manualMember" v-model="manual.workspaceIds" :member="manualMember" />
        <div class="modal-action"><button type="button" class="btn btn-ghost btn-sm" @click="manual = null">Cancel</button><button class="btn btn-primary btn-sm" :disabled="manual.workspaceIds !== null && !manual.workspaceIds.length">Create token</button></div>
      </form>
      <div class="modal-backdrop" @click="manual = null" />
    </dialog>

    <dialog v-if="created" open class="modal modal-open">
      <div class="modal-box w-[32rem] max-w-[95vw] space-y-3">
        <h3 class="text-lg font-semibold">Token created</h3>
        <div class="alert alert-warning alert-soft text-sm"><CatMood mood="warning" class="size-7 shrink-0 -my-0.5" /><span>This is the only time it is shown. It works like a password for this member’s device.</span></div>
        <div class="secret-box">{{ created }}</div>
        <div class="modal-action"><button class="btn btn-sm" @click="copy(created)">Copy</button><button class="btn btn-primary btn-sm" @click="created = null">Done</button></div>
      </div>
    </dialog>

    <ConfirmDialog v-if="revoking" :title="`Revoke ${revoking.device}?`" :text="`${revoking.memberName}'s other devices keep working. This one is disconnected at its next request.`" confirm="Revoke" tone="error" @ok="revoke" @cancel="revoking = null" />
  </div>
</template>
