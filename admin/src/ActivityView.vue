<script setup lang="ts">
import { ref, watch } from 'vue';
import { attempt, call, when, type AuditEntry } from './api';
import ActivityCharts from './ActivityCharts.vue';

/**
 * The audit log: who did what on this server (admin actions, devices connecting), newest first, a page at a time;
 * filtered by kind and searched on the server (who, what, detail). The Dashboard tab draws the same log as charts.
 */
const tab = ref<'charts' | 'list'>('charts');
const PAGE = 100;
const KINDS = [
  { id: '', label: 'All' },
  { id: 'members', label: 'Members' },
  { id: 'workspaces', label: 'Workspaces' },
  { id: 'devices', label: 'Devices & invites' },
  { id: 'admins', label: 'Admins' },
  { id: 'server', label: 'Server' },
] as const;
const kind = ref('');
const search = ref('');
const entries = ref<AuditEntry[]>([]);
const more = ref(false);
const total = ref(0);
const loading = ref(false);

async function load(older: boolean) {
  loading.value = true;
  const last = older ? entries.value[entries.value.length - 1] : undefined;
  const q = `?limit=${PAGE}&kind=${kind.value}&q=${encodeURIComponent(search.value.trim())}${last ? `&beforeAt=${last.at}&beforeId=${encodeURIComponent(last.id)}` : ''}`;
  const r = await attempt(() => call<{ entries: AuditEntry[]; more: boolean; total: number }>('GET', `/audit${q}`));
  loading.value = false;
  if (!r) return;
  entries.value = older ? [...entries.value, ...r.entries] : r.entries;
  more.value = r.more;
  total.value = r.total;
}
// the list loads when its tab is first opened
watch(tab, (t) => t === 'list' && !entries.value.length && load(false));
watch(kind, () => load(false));
let timer: ReturnType<typeof setTimeout> | null = null;
watch(search, () => {
  if (timer) clearTimeout(timer);
  timer = setTimeout(() => load(false), 250);
});
</script>

<template>
  <div class="mx-auto max-w-5xl space-y-5 p-8">
    <header>
      <h1 class="text-xl font-semibold tracking-tight">Activity</h1>
      <p class="text-sm opacity-60">Admin actions and device connections<template v-if="tab === 'list'">, newest first<template v-if="total"> · {{ entries.length }} of {{ total }} shown</template></template>.</p>
    </header>
    <div class="tabs-line" role="tablist" aria-label="View">
      <button role="tab" class="tabs-line-item" :class="{ 'is-active': tab === 'charts' }" :aria-selected="tab === 'charts'" @click="tab = 'charts'">
        <svg viewBox="0 0 24 24" class="size-4" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M3 3v18h18M7 16v-4M12 16V8M17 16v-7" /></svg>Dashboard
      </button>
      <button role="tab" class="tabs-line-item" :class="{ 'is-active': tab === 'list' }" :aria-selected="tab === 'list'" @click="tab = 'list'">
        <svg viewBox="0 0 24 24" class="size-4" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M8 6h13M8 12h13M8 18h13M3 6h.01M3 12h.01M3 18h.01" /></svg>List
      </button>
    </div>
    <ActivityCharts v-if="tab === 'charts'" />
    <template v-else>
    <div class="flex flex-wrap items-center gap-3">
      <div class="seg seg-choice max-w-full overflow-x-auto" role="tablist" aria-label="Kind">
        <button v-for="k in KINDS" :key="k.id" role="tab" class="seg-item shrink-0 normal-case whitespace-nowrap" :class="{ 'is-active': kind === k.id }" :aria-selected="kind === k.id" @click="kind = k.id">{{ k.label }}</button>
      </div>
      <input v-model="search" type="search" class="input input-sm min-w-48 flex-1" placeholder="Search who, what or detail" />
    </div>
    <div class="card"><div class="card-body card-table">
      <table class="table table-sm">
        <thead><tr><th>When</th><th>Who</th><th>What</th><th>Detail</th></tr></thead>
        <tbody>
          <tr v-if="!entries.length && !loading"><td colspan="4" class="py-8 text-center opacity-50">{{ kind || search.trim() ? 'Nothing matches.' : 'Nothing yet.' }}</td></tr>
          <tr v-for="e in entries" :key="e.id">
            <td class="text-xs whitespace-nowrap opacity-60">{{ when(e.at) }}</td>
            <td class="text-xs">{{ e.actor }}</td>
            <td><span class="badge badge-ghost badge-sm font-mono">{{ e.action }}</span></td>
            <td class="text-xs break-all">{{ e.detail }}</td>
          </tr>
        </tbody>
      </table>
    </div></div>
    <div v-if="more || loading" class="flex justify-center">
      <button class="btn btn-sm" :disabled="loading" @click="load(true)"><span v-if="loading" class="loading loading-spinner loading-xs" />Show older</button>
    </div>
    </template>
  </div>
</template>
