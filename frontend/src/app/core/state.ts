import { signal, computed } from '@angular/core';

// Example of Signals-based state management pattern.
// RxJS is used only for streaming (WebSocket, HTTP).

export function createListState<T>() {
  const items = signal<T[]>([]);
  const loading = signal(false);
  const error = signal<string | null>(null);

  return {
    items: items.asReadonly(),
    loading: loading.asReadonly(),
    error: error.asReadonly(),
    count: computed(() => items().length),
    setItems: (v: T[]) => items.set(v),
    setLoading: (v: boolean) => loading.set(v),
    setError: (v: string | null) => error.set(v),
    addItem: (item: T) => items.update(arr => [...arr, item]),
  };
}
