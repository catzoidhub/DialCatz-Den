<script setup lang="ts">
import { ref } from 'vue';
import { call, notify, scopeText, when, type Invite, type Member } from './api';
import CatMood from './CatMood.vue';
import ScopePicker from './ScopePicker.vue';

/**
 * A one-time invite link for a member's new device: dialcatz://connect?server=…&code=…. Opening it in DialCatz
 * exchanges the code for a token of that device; the token itself is never shown or sent by chat.
 */
const props = defineProps<{ member: Member }>();
const emit = defineEmits<{ close: [] }>();
const valid = ref(24);
const expiry = ref(0);
const scope = ref<string[] | null>(null);
const result = ref<{ link: string; code: string; server: string; invite: Invite } | null>(null);
const busy = ref(false);
const error = ref('');

async function create() {
  busy.value = true;
  error.value = '';
  try {
    result.value = await call('POST', '/invites', { memberId: props.member.id, validHours: valid.value, tokenExpiryDays: expiry.value, workspaceIds: scope.value ?? [] });
  } catch (e) {
    error.value = (e as Error).message;
  } finally {
    busy.value = false;
  }
}
async function copy(text: string, what: string) {
  await navigator.clipboard.writeText(text);
  notify('success', `${what} copied`);
}
</script>

<template>
  <dialog open class="modal modal-open" @keydown.esc="emit('close')">
    <div class="modal-box w-[34rem] max-w-[95vw] space-y-4">
      <h3 class="text-lg font-semibold tracking-tight">Invite {{ member.name }}</h3>
      <template v-if="!result">
        <p class="text-sm opacity-70">One link per device. {{ member.name.split(' ')[0] }} opens it on the computer that should connect; DialCatz shows this server’s address, asks, and stores its own token there.</p>
        <div class="opt-row">
          <span class="label-caps">Link works for</span>
          <div class="seg seg-choice">
            <button v-for="h in [1, 24, 168]" :key="h" type="button" class="seg-item normal-case" :class="{ 'is-active': valid === h }" @click="valid = h">{{ h === 1 ? '1 hour' : h === 24 ? '24 hours' : '7 days' }}</button>
          </div>
        </div>
        <div class="opt-row">
          <span class="label-caps">Device token expires</span>
          <div class="seg seg-choice">
            <button v-for="d in [0, 90, 365]" :key="d" type="button" class="seg-item normal-case" :class="{ 'is-active': expiry === d }" @click="expiry = d">{{ d === 0 ? 'Never' : `${d} days` }}</button>
          </div>
        </div>
        <ScopePicker v-model="scope" :member="member" />
        <div v-if="member.disabled" class="alert alert-warning alert-soft text-sm"><CatMood mood="warning" class="size-7 shrink-0 -my-0.5" /><span>This member is disabled: the link will not work until you enable them.</span></div>
        <div v-if="error" class="alert alert-error alert-soft text-sm"><CatMood mood="error" class="size-7 shrink-0 -my-0.5" /><span>{{ error }}</span></div>
        <div class="modal-action">
          <button class="btn btn-ghost btn-sm" @click="emit('close')">Cancel</button>
          <button class="btn btn-primary btn-sm" :disabled="busy || (scope !== null && !scope.length)" @click="create">Create invite link</button>
        </div>
      </template>
      <template v-else>
        <div class="alert alert-success alert-soft text-sm"><CatMood mood="success" class="size-7 shrink-0 -my-0.5" /><span>Send this link to {{ member.name.split(' ')[0] }}. It works once, until {{ when(result.invite.expiresAt) }}.</span></div>
        <p class="text-sm opacity-70">The device may use: {{ scopeText(result.invite.scope) }}</p>
        <div class="space-y-1">
          <span class="label-caps">Invite link</span>
          <div class="secret-box">{{ result.link }}</div>
          <button class="btn btn-primary btn-sm" @click="copy(result.link, 'Link')">Copy link</button>
        </div>
        <details class="text-sm">
          <summary class="cursor-pointer opacity-70">If the link does not open DialCatz</summary>
          <p class="mt-2 opacity-70">In DialCatz → Team, the invite can also be pasted as a link. Server: <code>{{ result.server }}</code></p>
        </details>
        <div class="modal-action"><button class="btn btn-sm" @click="emit('close')">Done</button></div>
      </template>
    </div>
    <div class="modal-backdrop" @click="emit('close')" />
  </dialog>
</template>
