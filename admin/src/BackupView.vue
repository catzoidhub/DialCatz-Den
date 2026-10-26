<script setup lang="ts">
import { computed, ref } from 'vue';
import { ApiError, notify, session, when } from './api';
import CatMood from './CatMood.vue';

/**
 * Backup: move this server's members, workspaces (with their collections and environments), devices and activity to
 * another DialCatz Den — export an encrypted file here, restore it there. Admins stay with each server.
 */
interface Summary {
  from: string;
  server: string;
  createdAt: number;
  counts: Record<string, number>;
}

const host = computed(() => {
  try {
    return new URL(session.publicUrl).hostname;
  } catch {
    return 'server';
  }
});

// ---- export
const pass = ref('');
const pass2 = ref('');
const exporting = ref(false);
const exportError = ref('');
async function exportBackup() {
  exportError.value = '';
  if (pass.value.length < 8) return void (exportError.value = 'Choose a passphrase of at least 8 characters');
  if (pass.value !== pass2.value) return void (exportError.value = 'The two passphrases are not the same');
  exporting.value = true;
  try {
    const res = await fetch('/api/admin/backup', { method: 'POST', headers: { 'X-DialCatz': '1', 'Content-Type': 'application/json' }, body: JSON.stringify({ passphrase: pass.value }), credentials: 'same-origin' });
    if (!res.ok) throw new ApiError(((await res.json().catch(() => null)) as { error?: string } | null)?.error ?? `HTTP ${res.status}`, res.status);
    const name = /filename="([^"]+)"/.exec(res.headers.get('Content-Disposition') ?? '')?.[1] ?? 'dialcatz-den.dcden';
    const url = URL.createObjectURL(await res.blob());
    const a = Object.assign(document.createElement('a'), { href: url, download: name });
    a.click();
    setTimeout(() => URL.revokeObjectURL(url), 10_000);
    notify('success', 'Backup downloaded — keep the passphrase with it');
    pass.value = pass2.value = '';
  } catch (e) {
    exportError.value = (e as Error).message;
  } finally {
    exporting.value = false;
  }
}

// ---- restore
const file = ref<File | null>(null);
const rpass = ref('');
const summary = ref<Summary | null>(null);
const typed = ref('');
const checking = ref(false);
const restoring = ref(false);
const restoreError = ref('');
const done = ref<Record<string, number> | null>(null);

function pick(e: Event) {
  file.value = (e.target as HTMLInputElement).files?.[0] ?? null;
  summary.value = null;
  done.value = null;
  restoreError.value = '';
}
async function send(dryRun: boolean): Promise<{ summary: Summary; restored?: Record<string, number> }> {
  const fd = new FormData();
  fd.append('file', file.value!);
  fd.append('passphrase', rpass.value);
  if (dryRun) fd.append('dryRun', '1');
  else fd.append('confirm', typed.value.trim());
  const res = await fetch('/api/admin/restore', { method: 'POST', headers: { 'X-DialCatz': '1' }, body: fd, credentials: 'same-origin' });
  const json = await res.json().catch(() => null);
  if (!res.ok) throw new ApiError((json as { error?: string } | null)?.error ?? `HTTP ${res.status}`, res.status);
  return json;
}
async function check() {
  if (!file.value) return;
  checking.value = true;
  restoreError.value = '';
  try {
    summary.value = (await send(true)).summary;
    typed.value = '';
  } catch (e) {
    restoreError.value = (e as Error).message;
  } finally {
    checking.value = false;
  }
}
async function restore() {
  if (typed.value.trim() !== host.value) return;
  restoring.value = true;
  restoreError.value = '';
  try {
    done.value = (await send(false)).restored ?? {};
    summary.value = null;
    file.value = null;
    rpass.value = '';
    notify('success', 'Restored');
  } catch (e) {
    restoreError.value = (e as Error).message;
  } finally {
    restoring.value = false;
  }
}
async function copyHost() {
  await navigator.clipboard.writeText(host.value);
  notify('success', 'Name copied');
}
const n = (c: Record<string, number> | undefined, k: string) => c?.[k] ?? 0;
</script>

<template>
  <div class="mx-auto max-w-4xl space-y-5 p-8">
    <header>
      <h1 class="text-xl font-semibold tracking-tight">Backup</h1>
      <p class="text-sm opacity-60">Move this team to another DialCatz Den: export a backup here, restore it on the new server.</p>
    </header>

    <div class="card">
      <div class="card-body space-y-1">
        <h2 class="label-caps">Moving to another server</h2>
        <ol class="list-decimal space-y-1 pl-5 text-sm opacity-80">
          <li>Here: <b>Export a backup</b> with a passphrase.</li>
          <li>On the new server: deploy DialCatz Den, create its admin at the first visit, then <b>Backup → Restore</b> this file.</li>
          <li>Point the server's address at the new server. With the same address, every member's devices keep working; with a new address, members change the server address in DialCatz → Team.</li>
        </ol>
      </div>
    </div>

    <div class="card">
      <div class="card-body space-y-3">
        <h2 class="text-base font-semibold">Export a backup</h2>
        <p class="text-sm opacity-70">One file with the members, workspaces with their collections and environments, devices and activity. Admins are not in it: each server keeps its own. The file is encrypted with the passphrase — without it the backup cannot be opened, so keep the two together and safe.</p>
        <div class="opt-row"><span class="label-caps">Passphrase</span><input v-model="pass" type="password" class="input input-sm w-72" autocomplete="new-password" minlength="8" /></div>
        <div class="opt-row"><span class="label-caps">Passphrase again</span><input v-model="pass2" type="password" class="input input-sm w-72" autocomplete="new-password" @keydown.enter="exportBackup" /></div>
        <p v-if="exportError" class="text-sm text-error">{{ exportError }}</p>
        <div class="flex justify-end"><button class="btn btn-primary btn-sm" :disabled="exporting || !pass" @click="exportBackup"><span v-if="exporting" class="loading loading-spinner loading-xs" />Export a backup</button></div>
      </div>
    </div>

    <div class="card border-error/25">
      <div class="card-body space-y-3">
        <h2 class="text-base font-semibold">Restore a backup</h2>
        <p class="text-sm opacity-70">Replaces this server's members, workspaces, devices and activity with the backup's. Admins and their sign-ins stay. Open invite links on this server are removed.</p>
        <div class="opt-row"><span class="label-caps">Backup file</span><input type="file" accept=".dcden" class="file-input file-input-sm w-72" @change="pick" /></div>
        <div class="opt-row"><span class="label-caps">Passphrase</span><input v-model="rpass" type="password" class="input input-sm w-72" autocomplete="off" @keydown.enter="check" /></div>
        <div v-if="!summary" class="flex justify-end"><button class="btn btn-sm" :disabled="checking || !file || !rpass" @click="check"><span v-if="checking" class="loading loading-spinner loading-xs" />Check the backup</button></div>

        <template v-if="summary">
          <div class="rounded-xl border border-base-300/70 bg-base-200/40 p-3 text-sm">
            <p>From <b>{{ summary.from }}</b> · {{ when(summary.createdAt) }} · DialCatz Den {{ summary.server }}</p>
            <p class="opacity-70">{{ n(summary.counts, 'members') }} members · {{ n(summary.counts, 'workspaces') }} workspaces · {{ n(summary.counts, 'items') }} collections, requests and environments · {{ n(summary.counts, 'tokens') }} devices · {{ n(summary.counts, 'audit_log') }} activity entries</p>
          </div>
          <div class="space-y-1.5">
            <p class="flex flex-wrap items-center gap-1 text-sm">Type <b class="font-mono">{{ host }}</b><button type="button" class="btn btn-ghost btn-xs btn-square" title="Copy the name" aria-label="Copy the name" @click="copyHost"><svg viewBox="0 0 24 24" class="size-3.5" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M20 9h-9a2 2 0 0 0-2 2v9a2 2 0 0 0 2 2h9a2 2 0 0 0 2-2v-9a2 2 0 0 0-2-2ZM5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1" /></svg></button>to replace this server's data:</p>
            <input v-model="typed" class="input input-sm w-full font-mono" :placeholder="host" autocomplete="off" spellcheck="false" @keydown.enter="restore" />
          </div>
          <div class="flex justify-end gap-2">
            <button class="btn btn-ghost btn-sm" :disabled="restoring" @click="summary = null">Cancel</button>
            <button class="btn btn-error btn-sm" :disabled="restoring || typed.trim() !== host" @click="restore"><span v-if="restoring" class="loading loading-spinner loading-xs" />Replace with this backup</button>
          </div>
        </template>

        <div v-if="done" class="alert alert-success alert-soft text-sm"><CatMood mood="success" class="size-7 shrink-0 -my-0.5" /><span>Restored {{ n(done, 'members') }} members, {{ n(done, 'workspaces') }} workspaces, {{ n(done, 'items') }} items and {{ n(done, 'tokens') }} devices.</span></div>
        <div v-if="restoreError" class="alert alert-error alert-soft text-sm"><CatMood mood="error" class="size-7 shrink-0 -my-0.5" /><span>{{ restoreError }}</span></div>
      </div>
    </div>
  </div>
</template>
