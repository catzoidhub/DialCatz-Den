import { computed, ref, watch, type Ref } from 'vue';

/**
 * Search, filter and pages for the panel's lists (Members, Workspaces, Devices, Admins). The lists come whole from the
 * server, so this runs in the browser: words match anywhere in `text(item)`, a filter is one predicate, 20 a page.
 */
export interface ListFilter<T> {
  id: string;
  label: string;
  test: (item: T) => boolean;
}

export function useList<T>(source: Ref<T[]>, text: (item: T) => string, filters: ListFilter<T>[], pageSize = 20) {
  const search = ref('');
  const filter = ref(filters[0]?.id ?? '');
  const page = ref(0);
  const matched = computed(() => {
    const words = search.value.toLowerCase().split(/\s+/).filter(Boolean);
    const f = filters.find((x) => x.id === filter.value);
    return source.value.filter((item) => (!f || f.test(item)) && (!words.length || words.every((w) => text(item).toLowerCase().includes(w))));
  });
  const pages = computed(() => Math.max(1, Math.ceil(matched.value.length / pageSize)));
  const shown = computed(() => matched.value.slice(page.value * pageSize, (page.value + 1) * pageSize));
  const range = computed(() => (matched.value.length ? `${page.value * pageSize + 1}–${Math.min(matched.value.length, (page.value + 1) * pageSize)} of ${matched.value.length}` : '0'));
  watch([search, filter], () => (page.value = 0));
  // A list that shrank (a delete on the last page) stays on a page that exists.
  watch(pages, (n) => (page.value = Math.min(page.value, n - 1)));
  const counts = computed(() => Object.fromEntries(filters.map((f) => [f.id, source.value.filter(f.test).length])));
  return { search, filter, page, pages, shown, matched, range, counts, filters };
}
