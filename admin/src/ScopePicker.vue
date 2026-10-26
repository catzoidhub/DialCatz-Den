<script setup lang="ts">
import type { Member } from './api';

/**
 * Which of the member's workspaces a token (or an invite's token) may use: all of them (null — also the ones they
 * get later), or only the picked ones.
 */
const props = defineProps<{ member: Member }>();
const ids = defineModel<string[] | null>({ required: true });
const first = () => props.member.name.split(' ')[0];

function toggle(id: string, on: boolean) {
  const cur = ids.value ?? [];
  ids.value = on ? [...cur, id] : cur.filter((x) => x !== id);
}
</script>

<template>
  <div class="space-y-2">
    <div class="opt-row">
      <span class="label-caps">Workspaces</span>
      <div class="seg seg-choice">
        <button type="button" class="seg-item normal-case" :class="{ 'is-active': ids === null }" @click="ids = null">All of {{ first() }}’s workspaces</button>
        <button type="button" class="seg-item normal-case" :class="{ 'is-active': ids !== null }" :disabled="!member.workspaces.length" @click="ids = ids ?? []">Only some</button>
      </div>
    </div>
    <template v-if="ids !== null">
      <label v-for="w in member.workspaces" :key="w.id" class="opt-row cursor-pointer pl-1">
        <span class="truncate text-sm">{{ w.name }} <span class="text-xs opacity-50">{{ w.role }}</span></span>
        <input type="checkbox" class="toggle toggle-sm toggle-primary" :checked="ids.includes(w.id)" @change="toggle(w.id, ($event.target as HTMLInputElement).checked)" />
      </label>
      <p v-if="!ids.length" class="text-xs text-warning">Pick at least one workspace.</p>
    </template>
    <p v-else-if="!member.workspaces.length" class="text-xs opacity-50">{{ first() }} has no workspace yet: give them one on the Members page.</p>
  </div>
</template>
