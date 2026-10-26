<script setup lang="ts">
/** The bar above a list: a filter per kind (with counts) and a search box (list.ts). */
defineProps<{ filters: { id: string; label: string }[]; counts: Record<string, number>; placeholder: string }>();
const filter = defineModel<string>('filter', { required: true });
const search = defineModel<string>('search', { required: true });
</script>

<template>
  <div class="flex flex-wrap items-center gap-3">
    <div class="seg seg-choice max-w-full overflow-x-auto" role="tablist" aria-label="Filter">
      <button v-for="f in filters" :key="f.id" type="button" role="tab" class="seg-item shrink-0 normal-case whitespace-nowrap" :class="{ 'is-active': filter === f.id }" :aria-selected="filter === f.id" @click="filter = f.id">
        {{ f.label }}<span class="text-xs opacity-50">{{ counts[f.id] ?? 0 }}</span>
      </button>
    </div>
    <input v-model="search" type="search" class="input input-sm min-w-48 flex-1" :placeholder="placeholder" />
  </div>
</template>
