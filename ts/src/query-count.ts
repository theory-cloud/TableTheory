export interface CountPage {
  count: number;
  cursor?: string;
}

export async function countAllPages(
  initialCursor: string | undefined,
  setCursor: (cursor: string | undefined) => void,
  readPage: () => Promise<CountPage>,
): Promise<number> {
  let total = 0;
  let cursor = initialCursor;
  try {
    do {
      setCursor(cursor);
      const page = await readPage();
      total += page.count;
      cursor = page.cursor;
    } while (cursor);
    return total;
  } finally {
    setCursor(initialCursor);
  }
}
