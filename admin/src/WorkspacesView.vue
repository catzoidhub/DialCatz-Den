<script setup lang="ts">
import { onMounted, ref } from 'vue';
import { attempt, call, when, type Workspace } from './api';
import ConfirmDialog from './ConfirmDialog.vue';
import ListPager from './ListPager.vue';
import ListTools from './ListTools.vue';
import { useList } from './list';

/** Workspaces: shared sets of collections and environments. Members get access per workspace (Members page). */
const list = ref<Workspace[]>([]);
/** Add workspace: the dialog's name (null: closed). */
const adding = ref<string | null>(null);
const L = useList(list, (w) => w.name, [
  { id: 'all', label: 'All', test: () => true },
  { id: 'nobody', label: 'No members yet', test: (w) => w.members === 0 },
  { id: 'empty', label: 'Empty', test: (w) => w.items === 0 },
]);
const { search, filter, page } = L;
const renaming = ref<{ id: string; name: string } | null>(null);
const removing = ref<Workspace | null>(null);

const load = async () => (list.value = await call<Workspace[]>('GET', '/workspaces'));
onMounted(load);

async function create() {
  const name = adding.value?.trim();
  if (!name) return;
  if (await attempt(() => call('POST', '/workspaces', { name }), 'Workspace created')) {
    adding.value = null;
    await load();
  }
}
async function rename() {
  const r = renaming.value!;
  renaming.value = null;
  if (await attempt(() => call('PATCH', `/workspaces/${r.id}`, { name: r.name }))) await load();
}
async function remove() {
  const w = removing.value!;
  removing.value = null;
  if (await attempt(() => call('DELETE', `/workspaces/${w.id}`), 'Workspace deleted')) await load();
}
</script>

<template>
  <div class="mx-auto max-w-4xl space-y-5 p-8">
    <header class="flex items-end gap-4">
      <div class="flex-1">
        <h1 class="text-xl font-semibold tracking-tight">Workspaces</h1>
        <p class="text-sm opacity-60">Shared collections and environments. Variable values never reach this server — only their names.</p>
      </div>
      <button class="btn btn-primary btn-sm" @click="adding = ''">Add workspace</button>
    </header>
    <ListTools v-model:filter="filter" v-model:search="search" :filters="L.filters" :counts="L.counts.value" placeholder="Search workspaces" />
    <div class="card"><div class="card-body card-table">
      <table class="table">
        <thead><tr><th>Name</th><th>Members</th><th>Items</th><th>Revision</th><th>Created</th><th /></tr></thead>
        <tbody>
          <tr v-if="!L.shown.value.length"><td colspan="6" class="py-8 text-center opacity-50">{{ list.length ? 'No workspace matches.' : 'No workspaces yet.' }}</td></tr>
          <tr v-for="w in L.shown.value" :key="w.id">
            <td class="font-medium">{{ w.name }}</td>
            <td>{{ w.members }}</td>
            <td>{{ w.items }}</td>
            <td class="font-mono text-xs">{{ w.revision }}</td>
            <td class="text-xs opacity-60">{{ when(w.createdAt) }}</td>
            <td class="text-right whitespace-nowrap">
              <button class="btn btn-ghost btn-xs" @click="renaming = { id: w.id, name: w.name }">Rename</button>
              <button class="btn btn-ghost btn-xs text-error" @click="removing = w">Delete</button>
            </td>
          </tr>
        </tbody>
      </table>
    </div></div>
    <ListPager v-model="page" :pages="L.pages.value" :range="L.range.value" />

    <dialog v-if="adding !== null" open class="modal modal-open" @keydown.esc="adding = null">
      <form class="modal-box w-[28rem] max-w-[95vw] space-y-3" @submit.prevent="create">
        <h3 class="text-lg font-semibold tracking-tight">Add workspace</h3>
        <p class="text-sm opacity-70">A shared set of collections and environments. Give members access to it on the Members page.</p>
        <input v-model="adding" class="input input-sm w-full" required autofocus placeholder="Workspace name, e.g. Shop API" />
        <div class="modal-action"><button type="button" class="btn btn-ghost btn-sm" @click="adding = null">Cancel</button><button class="btn btn-primary btn-sm" :disabled="!adding.trim()">Add workspace</button></div>
      </form>
      <div class="modal-backdrop" @click="adding = null" />
    </dialog>

    <dialog v-if="renaming" open class="modal modal-open" @keydown.esc="renaming = null">
      <form class="modal-box w-96 space-y-3" @submit.prevent="rename">
        <h3 class="text-lg font-semibold">Rename workspace</h3>
        <input v-model="renaming.name" class="input input-sm w-full" required />
        <div class="modal-action"><button type="button" class="btn btn-ghost btn-sm" @click="renaming = null">Cancel</button><button class="btn btn-primary btn-sm">Rename</button></div>
      </form>
      <div class="modal-backdrop" @click="renaming = null" />
    </dialog>
    <ConfirmDialog v-if="removing" :title="`Delete ${removing.name}?`" :text="`Its ${removing.items} collections, requests and environments are deleted from the server, for everyone. Copies on members' computers are removed at their next sync.`" confirm="Delete workspace" tone="error" @ok="remove" @cancel="removing = null" />
  </div>
</template>
