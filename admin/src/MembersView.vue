<script setup lang="ts">
import { onMounted, ref } from 'vue';
import { attempt, call, type Member, type Role, type Workspace } from './api';
import InviteDialog from './InviteDialog.vue';
import MemberAvatar from './MemberAvatar.vue';
import ConfirmDialog from './ConfirmDialog.vue';
import ListPager from './ListPager.vue';
import ListTools from './ListTools.vue';
import { useList } from './list';

/**
 * Members: the people of the team. The admin creates them (name and email are set here, so nobody can pretend
 * to be someone else), gives them workspaces with a role, and invites each of their devices.
 */
const members = ref<Member[]>([]);
const L = useList(members, (m) => `${m.name} ${m.email} ${m.workspaces.map((w) => w.name).join(' ')}`, [
  { id: 'all', label: 'All', test: () => true },
  { id: 'active', label: 'Active', test: (m) => !m.disabled },
  { id: 'disabled', label: 'Disabled', test: (m) => m.disabled },
  { id: 'nodevice', label: 'Without devices', test: (m) => m.devices === 0 },
]);
const { search, filter, page } = L;
const workspaces = ref<Workspace[]>([]);
const editing = ref<{ id: string | null; name: string; email: string; avatarUrl: string; disabled: boolean } | null>(null);
const inviting = ref<Member | null>(null);
const removing = ref<Member | null>(null);
const error = ref('');

async function load() {
  ;[members.value, workspaces.value] = await Promise.all([call<Member[]>('GET', '/members'), call<Workspace[]>('GET', '/workspaces')]);
}
onMounted(load);

function edit(m?: Member) {
  error.value = '';
  // A Gravatar URL is the fallback, not something the admin set: the field stays empty for it.
  editing.value = m ? { id: m.id, name: m.name, email: m.email, avatarUrl: m.gravatar ? '' : m.avatarUrl, disabled: m.disabled } : { id: null, name: '', email: '', avatarUrl: '', disabled: false };
}
async function save() {
  const e = editing.value!;
  error.value = '';
  try {
    const m = await call<Member>(e.id ? 'PATCH' : 'POST', e.id ? `/members/${e.id}` : '/members', e);
    editing.value = null;
    await load();
    if (!e.id) inviting.value = m;
  } catch (err) {
    error.value = (err as Error).message;
  }
}
async function setRole(m: Member, wsId: string, role: Role | '') {
  if (await attempt(() => call('PUT', `/members/${m.id}/workspaces/${wsId}`, { role }))) await load();
}
async function remove() {
  const m = removing.value!;
  removing.value = null;
  if (await attempt(() => call('DELETE', `/members/${m.id}`), `${m.name} removed`)) await load();
}
const roleOf = (m: Member, wsId: string) => m.workspaces.find((w) => w.id === wsId)?.role ?? '';
</script>

<template>
  <div class="mx-auto max-w-5xl space-y-5 p-8">
    <header class="flex items-end gap-4">
      <div class="flex-1">
        <h1 class="text-xl font-semibold tracking-tight">Members</h1>
        <p class="text-sm opacity-60">Everyone who uses this server. Members never sign in: you invite each of their devices.</p>
      </div>
      <button class="btn btn-primary btn-sm" @click="edit()">Add member</button>
    </header>

    <div v-if="!members.length" class="card"><div class="card-body items-center py-12 text-center">
      <p class="font-medium">No members yet</p>
      <p class="text-sm opacity-60">Add the people of your team, give them a workspace, and send each an invite link.</p>
      <button class="btn btn-primary btn-sm mt-2" @click="edit()">Add the first member</button>
    </div></div>

    <ListTools v-if="members.length" v-model:filter="filter" v-model:search="search" :filters="L.filters" :counts="L.counts.value" placeholder="Search name, email or workspace" />
    <p v-if="members.length && !L.shown.value.length" class="py-8 text-center text-sm opacity-50">No member matches.</p>
    <div v-for="m in L.shown.value" :key="m.id" class="card" :class="{ 'opacity-60': m.disabled }">
      <div class="card-body">
        <div class="flex flex-wrap items-center gap-3">
          <MemberAvatar :name="m.name" :url="m.avatarUrl" />
          <div class="min-w-0 flex-1 basis-40">
            <h2 class="truncate">{{ m.name }} <span v-if="m.disabled" class="badge badge-soft badge-warning badge-sm">disabled</span></h2>
            <p class="truncate text-xs opacity-60">{{ m.email }} · {{ m.devices }} active device{{ m.devices === 1 ? '' : 's' }}</p>
          </div>
          <button class="btn btn-sm" :disabled="m.disabled" @click="inviting = m">Invite a device</button>
          <button class="btn btn-ghost btn-sm" @click="edit(m)">Edit</button>
          <button class="btn btn-ghost btn-sm text-error" @click="removing = m">Remove</button>
        </div>
        <div v-if="workspaces.length" class="mt-1 grid gap-x-6 gap-y-1 sm:grid-cols-2">
          <div v-for="w in workspaces" :key="w.id" class="opt-row">
            <span class="truncate text-sm">{{ w.name }}</span>
            <select class="select select-xs w-32" :value="roleOf(m, w.id)" @change="setRole(m, w.id, ($event.target as HTMLSelectElement).value as Role | '')">
              <option value="">No access</option>
              <option value="viewer">Viewer</option>
              <option value="editor">Editor</option>
              <option value="owner">Owner</option>
            </select>
          </div>
        </div>
        <p v-else class="text-xs opacity-50">Create a workspace to give members access.</p>
      </div>
    </div>

    <ListPager v-model="page" :pages="L.pages.value" :range="L.range.value" />

    <dialog v-if="editing" open class="modal modal-open" @keydown.esc="editing = null">
      <form class="modal-box w-[30rem] max-w-[95vw] space-y-3" @submit.prevent="save">
        <h3 class="text-lg font-semibold tracking-tight">{{ editing.id ? 'Edit member' : 'Add member' }}</h3>
        <div class="opt-row"><span class="label-caps">Name</span><input v-model="editing.name" class="input input-sm flex-1" required placeholder="Andi Pratama" /></div>
        <div class="opt-row"><span class="label-caps">Email</span><input v-model="editing.email" type="email" class="input input-sm flex-1" required placeholder="andi@company.com" /></div>
        <div class="opt-row"><span class="label-caps">Avatar URL</span><input v-model="editing.avatarUrl" class="input input-sm flex-1" placeholder="optional, https://… (else Gravatar)" /></div>
        <div v-if="editing.id" class="opt-row">
          <span class="flex flex-col"><span class="label-caps">Disabled</span><span class="text-xs opacity-50">All of their devices stop working at once.</span></span>
          <input v-model="editing.disabled" type="checkbox" class="toggle toggle-sm toggle-warning" />
        </div>
        <p v-if="error" class="text-sm text-error">{{ error }}</p>
        <div class="modal-action">
          <button type="button" class="btn btn-ghost btn-sm" @click="editing = null">Cancel</button>
          <button class="btn btn-primary btn-sm">{{ editing.id ? 'Save' : 'Add and invite' }}</button>
        </div>
      </form>
      <div class="modal-backdrop" @click="editing = null" />
    </dialog>

    <InviteDialog v-if="inviting" :member="inviting" @close="(inviting = null), load()" />
    <ConfirmDialog v-if="removing" :title="`Remove ${removing.name}?`" text="Their devices are disconnected and they lose every workspace. Their past changes stay." confirm="Remove member" tone="error" @ok="remove" @cancel="removing = null" />
  </div>
</template>
