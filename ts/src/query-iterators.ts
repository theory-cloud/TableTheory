import type { Page } from './query.js';

export async function* pageIterator<T>(
  getCursor: () => string | undefined,
  setCursor: (cursor: string | undefined) => void,
  loadPage: () => Promise<Page<T>>,
): AsyncGenerator<Page<T>> {
  const original = getCursor();
  try {
    let cursor = original;
    for (;;) {
      setCursor(cursor);
      const page = await loadPage();
      yield page;
      if (!page.cursor) break;
      cursor = page.cursor;
    }
  } finally {
    setCursor(original);
  }
}

export async function* itemIterator<T>(
  pages: AsyncIterable<Page<T>>,
): AsyncGenerator<T> {
  for await (const page of pages) {
    for (const item of page.items) {
      yield item;
    }
  }
}

export async function collectAllItems<T>(
  getCursor: () => string | undefined,
  setCursor: (cursor: string | undefined) => void,
  loadPage: () => Promise<Page<T>>,
): Promise<T[]> {
  const out: T[] = [];
  for await (const page of pageIterator(getCursor, setCursor, loadPage)) {
    out.push(...page.items);
  }
  return out;
}
