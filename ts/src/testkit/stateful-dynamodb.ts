import {
  BatchGetItemCommand,
  BatchWriteItemCommand,
  ConditionalCheckFailedException,
  CreateTableCommand,
  DeleteItemCommand,
  DeleteTableCommand,
  DescribeTableCommand,
  GetItemCommand,
  ListTablesCommand,
  PutItemCommand,
  QueryCommand,
  ResourceInUseException,
  ResourceNotFoundException,
  ScanCommand,
  TransactionCanceledException,
  TransactGetItemsCommand,
  TransactWriteItemsCommand,
  UpdateItemCommand,
  UpdateTimeToLiveCommand,
  type AttributeValue,
  type DynamoDBClient,
  type WriteRequest,
} from '@aws-sdk/client-dynamodb';

type Item = Record<string, AttributeValue>;
type Table = { items: Map<string, Item>; pk: string; sk?: string };

export interface StatefulDynamoDBClient {
  readonly client: DynamoDBClient;
  readonly fake: StatefulDynamoDBFake;
}

type SupportedCommand =
  | PutItemCommand
  | GetItemCommand
  | UpdateItemCommand
  | DeleteItemCommand
  | QueryCommand
  | ScanCommand
  | BatchGetItemCommand
  | BatchWriteItemCommand
  | CreateTableCommand
  | DescribeTableCommand
  | DeleteTableCommand
  | ListTablesCommand
  | UpdateTimeToLiveCommand
  | TransactGetItemsCommand
  | TransactWriteItemsCommand;

export function createStatefulDynamoDBClient(): StatefulDynamoDBClient {
  const fake = new StatefulDynamoDBFake();
  return { fake, client: fake.client };
}

export class StatefulDynamoDBFake {
  private readonly tables = new Map<string, Table>();

  readonly client = {
    send: async (command: SupportedCommand): Promise<unknown> =>
      this.send(command),
  } as unknown as DynamoDBClient;

  reset(): void {
    this.tables.clear();
  }

  seed(tableName: string, ...items: Item[]): void {
    const table = this.table(tableName);
    for (const item of items)
      table.items.set(itemKey(table, item), cloneItem(item));
  }

  items(tableName: string): Item[] {
    const table = this.tables.get(tableName);
    if (!table) return [];
    return [...table.items.values()]
      .map(cloneItem)
      .sort((a, b) => itemKey(table, a).localeCompare(itemKey(table, b)));
  }

  async send(command: SupportedCommand): Promise<unknown> {
    if (command instanceof PutItemCommand) return this.put(command);
    if (command instanceof GetItemCommand) return this.get(command);
    if (command instanceof UpdateItemCommand) return this.update(command);
    if (command instanceof DeleteItemCommand) return this.delete(command);
    if (command instanceof QueryCommand) return this.query(command);
    if (command instanceof ScanCommand) return this.scan(command);
    if (command instanceof BatchGetItemCommand) return this.batchGet(command);
    if (command instanceof BatchWriteItemCommand)
      return this.batchWrite(command);
    if (command instanceof CreateTableCommand) return this.createTable(command);
    if (command instanceof DescribeTableCommand)
      return this.describeTable(command);
    if (command instanceof DeleteTableCommand) return this.deleteTable(command);
    if (command instanceof ListTablesCommand) return this.listTables(command);
    if (command instanceof UpdateTimeToLiveCommand)
      return this.updateTimeToLive(command);
    if (command instanceof TransactGetItemsCommand)
      return this.transactGet(command);
    if (command instanceof TransactWriteItemsCommand)
      return this.transactWrite(command);
    throw new Error('Unsupported DynamoDB command');
  }

  private createTable(command: CreateTableCommand): object {
    const name = command.input.TableName ?? 'default';
    if (this.tables.has(name)) throw resourceInUse(name);
    const table: Table = {
      pk: keySchemaAttribute(command.input.KeySchema, 'HASH') ?? 'PK',
      items: new Map(),
    };
    const sk = keySchemaAttribute(command.input.KeySchema, 'RANGE');
    if (sk) table.sk = sk;
    this.tables.set(name, table);
    return { $metadata: {}, TableDescription: tableDescription(name, table) };
  }

  private describeTable(command: DescribeTableCommand): object {
    const name = command.input.TableName ?? 'default';
    const table = this.tables.get(name);
    if (!table) throw resourceNotFound(name);
    return { $metadata: {}, Table: tableDescription(name, table) };
  }

  private deleteTable(command: DeleteTableCommand): object {
    const name = command.input.TableName ?? 'default';
    const table = this.tables.get(name);
    if (!table) throw resourceNotFound(name);
    this.tables.delete(name);
    return { $metadata: {}, TableDescription: tableDescription(name, table) };
  }

  private listTables(command: ListTablesCommand): object {
    const limit = command.input.Limit ?? this.tables.size;
    return {
      $metadata: {},
      TableNames: [...this.tables.keys()].sort().slice(0, limit),
    };
  }

  private updateTimeToLive(command: UpdateTimeToLiveCommand): object {
    const name = command.input.TableName ?? 'default';
    if (!this.tables.has(name)) throw resourceNotFound(name);
    return {
      $metadata: {},
      TimeToLiveSpecification: command.input.TimeToLiveSpecification,
    };
  }

  private put(command: PutItemCommand): object {
    const input = command.input;
    const table = this.table(input.TableName);
    const key = itemKey(table, input.Item ?? {});
    const existing = table.items.get(key);
    if (
      !matchesExpression(
        input.ConditionExpression,
        existing,
        input.ExpressionAttributeNames,
        input.ExpressionAttributeValues,
      )
    ) {
      throw conditionalFailed();
    }
    table.items.set(key, cloneItem(input.Item ?? {}));
    return { $metadata: {} };
  }

  private get(command: GetItemCommand): object {
    const input = command.input;
    const table = this.table(input.TableName);
    const item = table.items.get(keyFromMap(table, input.Key ?? {}));
    return {
      $metadata: {},
      ...(item
        ? {
            Item: projectItem(
              item,
              input.ProjectionExpression,
              input.ExpressionAttributeNames,
            ),
          }
        : {}),
    };
  }

  private update(command: UpdateItemCommand): object {
    const input = command.input;
    const table = this.table(input.TableName);
    const key = keyFromMap(table, input.Key ?? {});
    const item = cloneItem(table.items.get(key) ?? input.Key ?? {});
    if (
      !matchesExpression(
        input.ConditionExpression,
        table.items.get(key),
        input.ExpressionAttributeNames,
        input.ExpressionAttributeValues,
      )
    ) {
      throw conditionalFailed();
    }
    applyUpdate(
      item,
      input.UpdateExpression,
      input.ExpressionAttributeNames,
      input.ExpressionAttributeValues,
    );
    table.items.set(key, item);
    return {
      $metadata: {},
      ...(input.ReturnValues === 'ALL_NEW' ||
      input.ReturnValues === 'UPDATED_NEW'
        ? { Attributes: cloneItem(item) }
        : {}),
    };
  }

  private delete(command: DeleteItemCommand): object {
    const input = command.input;
    const table = this.table(input.TableName);
    const key = keyFromMap(table, input.Key ?? {});
    const existing = table.items.get(key);
    if (
      !matchesExpression(
        input.ConditionExpression,
        existing,
        input.ExpressionAttributeNames,
        input.ExpressionAttributeValues,
      )
    ) {
      throw conditionalFailed();
    }
    table.items.delete(key);
    return { $metadata: {} };
  }

  private query(command: QueryCommand): object {
    const input = command.input;
    const table = this.table(input.TableName);
    return readResponse(table, [...table.items.values()], {
      keyExpression: input.KeyConditionExpression,
      filterExpression: input.FilterExpression,
      projectionExpression: input.ProjectionExpression,
      names: input.ExpressionAttributeNames,
      values: input.ExpressionAttributeValues,
      limit: input.Limit,
      exclusiveStartKey: input.ExclusiveStartKey,
      forward: input.ScanIndexForward ?? true,
      select: input.Select,
    });
  }

  private scan(command: ScanCommand): object {
    const input = command.input;
    const table = this.table(input.TableName);
    return readResponse(table, [...table.items.values()], {
      filterExpression: input.FilterExpression,
      projectionExpression: input.ProjectionExpression,
      names: input.ExpressionAttributeNames,
      values: input.ExpressionAttributeValues,
      limit: input.Limit,
      exclusiveStartKey: input.ExclusiveStartKey,
      select: input.Select,
    });
  }

  private batchGet(command: BatchGetItemCommand): object {
    const responses: Record<string, Item[]> = {};
    for (const [tableName, req] of Object.entries(
      command.input.RequestItems ?? {},
    )) {
      const table = this.table(tableName);
      responses[tableName] = [];
      for (const key of req.Keys ?? []) {
        const item = table.items.get(keyFromMap(table, key));
        if (item) {
          responses[tableName]!.push(
            projectItem(
              item,
              req.ProjectionExpression,
              req.ExpressionAttributeNames,
            ),
          );
        }
      }
    }
    return { $metadata: {}, Responses: responses };
  }

  private batchWrite(command: BatchWriteItemCommand): object {
    for (const [tableName, writes] of Object.entries(
      command.input.RequestItems ?? {},
    )) {
      const table = this.table(tableName);
      for (const write of writes ?? []) this.applyWriteRequest(table, write);
    }
    return { $metadata: {} };
  }

  private transactGet(command: TransactGetItemsCommand): object {
    const Responses = (command.input.TransactItems ?? []).map((op) => {
      const get = op.Get;
      if (!get) return {};
      const table = this.table(get.TableName);
      const item = table.items.get(keyFromMap(table, get.Key ?? {}));
      return {
        ...(item
          ? {
              Item: projectItem(
                item,
                get.ProjectionExpression,
                get.ExpressionAttributeNames,
              ),
            }
          : {}),
      };
    });
    return { $metadata: {}, Responses };
  }

  private transactWrite(command: TransactWriteItemsCommand): object {
    const snapshot = cloneTables(this.tables);
    try {
      for (const op of command.input.TransactItems ?? [])
        applyTransact(snapshot, op);
    } catch (err) {
      throw transactionCanceled(err);
    }
    this.tables.clear();
    for (const [name, table] of snapshot) this.tables.set(name, table);
    return { $metadata: {} };
  }

  private applyWriteRequest(table: Table, write: WriteRequest): void {
    if (write.PutRequest?.Item) {
      table.items.set(
        itemKey(table, write.PutRequest.Item),
        cloneItem(write.PutRequest.Item),
      );
    }
    if (write.DeleteRequest?.Key)
      table.items.delete(keyFromMap(table, write.DeleteRequest.Key));
  }

  private table(name?: string): Table {
    const tableName = name ?? 'default';
    let table = this.tables.get(tableName);
    if (!table) {
      table = { pk: 'PK', sk: 'SK', items: new Map() };
      this.tables.set(tableName, table);
    }
    return table;
  }
}

function readResponse(
  table: Table,
  source: Item[],
  opts: {
    keyExpression?: string | undefined;
    filterExpression?: string | undefined;
    projectionExpression?: string | undefined;
    names?: Record<string, string> | undefined;
    values?: Record<string, AttributeValue> | undefined;
    limit?: number | undefined;
    exclusiveStartKey?: Item | undefined;
    forward?: boolean | undefined;
    select?: string | undefined;
  },
): object {
  let items = source
    .filter((item) =>
      matchesExpression(opts.keyExpression, item, opts.names, opts.values),
    )
    .filter((item) =>
      matchesExpression(opts.filterExpression, item, opts.names, opts.values),
    )
    .map(cloneItem)
    .sort((a, b) => compareAV(a[table.sk ?? ''], b[table.sk ?? '']));
  if (opts.forward === false) items = items.reverse();
  if (opts.exclusiveStartKey) {
    const start = keyFromMap(table, opts.exclusiveStartKey);
    const index = items.findIndex((item) => itemKey(table, item) === start);
    if (index >= 0) items = items.slice(index + 1);
  }
  const scanned = items.length;
  let LastEvaluatedKey: Item | undefined;
  if (opts.limit && items.length > opts.limit) {
    LastEvaluatedKey = keyMap(table, items[opts.limit - 1]!);
    items = items.slice(0, opts.limit);
  }
  if (opts.select === 'COUNT') {
    return {
      $metadata: {},
      Count: scanned,
      ScannedCount: scanned,
      LastEvaluatedKey,
    };
  }
  return {
    $metadata: {},
    Items: items.map((item) =>
      projectItem(item, opts.projectionExpression, opts.names),
    ),
    Count: items.length,
    ScannedCount: scanned,
    LastEvaluatedKey,
  };
}

function applyTransact(
  tables: Map<string, Table>,
  op: NonNullable<TransactWriteItemsCommand['input']['TransactItems']>[number],
): void {
  if (op.Put) {
    const table = tableFrom(tables, op.Put.TableName);
    const key = itemKey(table, op.Put.Item ?? {});
    if (
      !matchesExpression(
        op.Put.ConditionExpression,
        table.items.get(key),
        op.Put.ExpressionAttributeNames,
        op.Put.ExpressionAttributeValues,
      )
    )
      throw conditionalFailed();
    table.items.set(key, cloneItem(op.Put.Item ?? {}));
  }
  if (op.Update) {
    const table = tableFrom(tables, op.Update.TableName);
    const key = keyFromMap(table, op.Update.Key ?? {});
    const item = cloneItem(table.items.get(key) ?? op.Update.Key ?? {});
    if (
      !matchesExpression(
        op.Update.ConditionExpression,
        table.items.get(key),
        op.Update.ExpressionAttributeNames,
        op.Update.ExpressionAttributeValues,
      )
    )
      throw conditionalFailed();
    applyUpdate(
      item,
      op.Update.UpdateExpression,
      op.Update.ExpressionAttributeNames,
      op.Update.ExpressionAttributeValues,
    );
    table.items.set(key, item);
  }
  if (op.Delete) {
    const table = tableFrom(tables, op.Delete.TableName);
    const key = keyFromMap(table, op.Delete.Key ?? {});
    if (
      !matchesExpression(
        op.Delete.ConditionExpression,
        table.items.get(key),
        op.Delete.ExpressionAttributeNames,
        op.Delete.ExpressionAttributeValues,
      )
    )
      throw conditionalFailed();
    table.items.delete(key);
  }
  if (op.ConditionCheck) {
    const table = tableFrom(tables, op.ConditionCheck.TableName);
    const key = keyFromMap(table, op.ConditionCheck.Key ?? {});
    if (
      !matchesExpression(
        op.ConditionCheck.ConditionExpression,
        table.items.get(key),
        op.ConditionCheck.ExpressionAttributeNames,
        op.ConditionCheck.ExpressionAttributeValues,
      )
    )
      throw conditionalFailed();
  }
}

function tableFrom(tables: Map<string, Table>, name?: string): Table {
  const tableName = name ?? 'default';
  let table = tables.get(tableName);
  if (!table) {
    table = { pk: 'PK', sk: 'SK', items: new Map() };
    tables.set(tableName, table);
  }
  return table;
}

function keySchemaAttribute(
  schema: NonNullable<CreateTableCommand['input']['KeySchema']> | undefined,
  keyType: 'HASH' | 'RANGE',
): string | undefined {
  return schema?.find((key) => key.KeyType === keyType)?.AttributeName;
}

function tableDescription(name: string, table: Table): object {
  return {
    TableName: name,
    TableStatus: 'ACTIVE',
    KeySchema: [
      { AttributeName: table.pk, KeyType: 'HASH' },
      ...(table.sk ? [{ AttributeName: table.sk, KeyType: 'RANGE' }] : []),
    ],
    ItemCount: table.items.size,
  };
}

function matchesExpression(
  expression?: string,
  item?: Item,
  names: Record<string, string> = {},
  values: Record<string, AttributeValue> = {},
): boolean {
  const expr = stripOuterParens(expression?.trim() ?? '');
  if (!expr) return true;
  const orParts = splitLogical(expr, 'OR');
  if (orParts.length > 1)
    return orParts.some((part) => matchesExpression(part, item, names, values));
  const andParts = splitLogical(expr, 'AND');
  if (andParts.length > 1)
    return andParts.every((part) =>
      matchesExpression(part, item, names, values),
    );
  if (expr.startsWith('attribute_not_exists('))
    return item?.[nameOf(expr.slice(21, -1), names)] === undefined;
  if (expr.startsWith('attribute_exists('))
    return item?.[nameOf(expr.slice(17, -1), names)] !== undefined;
  if (expr.startsWith('begins_with(')) {
    const [attr, value] = splitCsv(expr.slice(12, -1));
    return stringValue(item?.[nameOf(attr ?? '', names)]).startsWith(
      stringValue(values[(value ?? '').trim()]),
    );
  }
  for (const op of ['<>', '>=', '<=', '=', '>', '<']) {
    const needle = ` ${op} `;
    const idx = expr.indexOf(needle);
    if (idx < 0) continue;
    const left = expr.slice(0, idx);
    const right = expr.slice(idx + needle.length).trim();
    const cmp = compareAV(item?.[nameOf(left, names)], values[right]);
    if (op === '=') return cmp === 0;
    if (op === '<>') return cmp !== 0;
    if (op === '>') return cmp > 0;
    if (op === '>=') return cmp >= 0;
    if (op === '<') return cmp < 0;
    if (op === '<=') return cmp <= 0;
  }
  return false;
}

function applyUpdate(
  item: Item,
  expression?: string,
  names: Record<string, string> = {},
  values: Record<string, AttributeValue> = {},
): void {
  for (const [action, body] of updateSections(expression ?? '')) {
    if (action === 'SET') {
      for (const part of splitCsv(body)) {
        const [left, right] = part.split(/\s*=\s*/);
        if (left && right)
          item[nameOf(left, names)] = cloneAV(values[right.trim()]);
      }
    }
    if (action === 'ADD') {
      for (const part of splitCsv(body)) {
        const [left, right] = part.trim().split(/\s+/);
        if (left && right)
          item[nameOf(left, names)] = addAV(
            item[nameOf(left, names)],
            values[right],
          );
      }
    }
    if (action === 'REMOVE') {
      for (const part of splitCsv(body)) delete item[nameOf(part, names)];
    }
  }
}

function updateSections(expression: string): Array<[string, string]> {
  const re = /\b(SET|ADD|REMOVE|DELETE)\b/g;
  const matches = [...expression.matchAll(re)];
  return matches.map((match, index) => [
    match[1]!,
    expression
      .slice(match.index! + match[1]!.length, matches[index + 1]?.index)
      .trim(),
  ]);
}

function splitLogical(expr: string, op: 'AND' | 'OR'): string[] {
  const target = ` ${op} `;
  const parts: string[] = [];
  let depth = 0;
  let start = 0;
  for (let i = 0; i < expr.length; i += 1) {
    if (expr[i] === '(') depth += 1;
    if (expr[i] === ')') depth -= 1;
    if (depth === 0 && expr.slice(i, i + target.length) === target) {
      parts.push(expr.slice(start, i).trim());
      start = i + target.length;
      i += target.length - 1;
    }
  }
  if (parts.length === 0) return [expr];
  parts.push(expr.slice(start).trim());
  return parts;
}

function stripOuterParens(expr: string): string {
  while (expr.startsWith('(') && expr.endsWith(')'))
    expr = expr.slice(1, -1).trim();
  return expr;
}

function splitCsv(input: string): string[] {
  const parts: string[] = [];
  let depth = 0;
  let start = 0;
  for (let i = 0; i < input.length; i += 1) {
    if (input[i] === '(') depth += 1;
    if (input[i] === ')') depth -= 1;
    if (input[i] === ',' && depth === 0) {
      parts.push(input.slice(start, i).trim());
      start = i + 1;
    }
  }
  const last = input.slice(start).trim();
  if (last) parts.push(last);
  return parts;
}

function projectItem(
  item: Item,
  projection?: string,
  names: Record<string, string> = {},
): Item {
  if (!projection) return cloneItem(item);
  const out: Item = {};
  for (const token of splitCsv(projection)) {
    const attr = nameOf(token, names);
    if (item[attr] !== undefined) out[attr] = cloneAV(item[attr]);
  }
  return out;
}

function nameOf(token: string, names: Record<string, string>): string {
  const trimmed = token.trim();
  return names[trimmed] ?? trimmed;
}

function itemKey(table: Table, item: Item): string {
  const pk = item[table.pk];
  if (!pk) throw new Error(`missing partition key ${table.pk}`);
  return `${keyPart(pk)}|${table.sk ? keyPart(item[table.sk]) : ''}`;
}

function keyFromMap(table: Table, key: Item): string {
  return itemKey(table, key);
}

function keyMap(table: Table, item: Item): Item {
  const out: Item = { [table.pk]: cloneAV(item[table.pk]) };
  if (table.sk) out[table.sk] = cloneAV(item[table.sk]);
  return out;
}

function keyPart(av?: AttributeValue): string {
  if (!av) return '';
  if ('S' in av) return `S:${av.S ?? ''}`;
  if ('N' in av) return `N:${av.N ?? ''}`;
  if ('B' in av) return `B:${String(av.B ?? '')}`;
  return JSON.stringify(av);
}

function compareAV(left?: AttributeValue, right?: AttributeValue): number {
  if (!left && !right) return 0;
  if (!left) return -1;
  if (!right) return 1;
  if ('N' in left && 'N' in right) return Number(left.N) - Number(right.N);
  return keyPart(left).localeCompare(keyPart(right));
}

function stringValue(av?: AttributeValue): string {
  if (!av) return '';
  if ('S' in av) return av.S ?? '';
  if ('N' in av) return av.N ?? '';
  return keyPart(av);
}

function addAV(left?: AttributeValue, right?: AttributeValue): AttributeValue {
  if ('N' in (left ?? {}) && 'N' in (right ?? {})) {
    return { N: String(Number(left!.N) + Number(right!.N)) };
  }
  return cloneAV(right);
}

function cloneTables(tables: Map<string, Table>): Map<string, Table> {
  const out = new Map<string, Table>();
  for (const [name, table] of tables) {
    const cloned: Table = {
      pk: table.pk,
      items: new Map(
        [...table.items].map(([key, item]) => [key, cloneItem(item)]),
      ),
    };
    if (table.sk !== undefined) cloned.sk = table.sk;
    out.set(name, cloned);
  }
  return out;
}

function cloneItem(item?: Item): Item {
  const out: Item = {};
  for (const [key, value] of Object.entries(item ?? {}))
    out[key] = cloneAV(value);
  return out;
}

function cloneAV(av?: AttributeValue): AttributeValue {
  if (!av) return { NULL: true };
  if ('B' in av && av.B) return { B: new Uint8Array(av.B) };
  if ('L' in av) return { L: (av.L ?? []).map(cloneAV) };
  if ('M' in av) return { M: cloneItem(av.M) };
  if ('SS' in av) return { SS: [...(av.SS ?? [])] };
  if ('NS' in av) return { NS: [...(av.NS ?? [])] };
  if ('BS' in av) return { BS: (av.BS ?? []).map((b) => new Uint8Array(b)) };
  return { ...av };
}

function conditionalFailed(): ConditionalCheckFailedException {
  return new ConditionalCheckFailedException({
    $metadata: {},
    message: 'conditional request failed',
  });
}

function resourceNotFound(tableName: string): ResourceNotFoundException {
  return new ResourceNotFoundException({
    $metadata: {},
    message: `table not found: ${tableName}`,
  });
}

function resourceInUse(tableName: string): ResourceInUseException {
  return new ResourceInUseException({
    $metadata: {},
    message: `table already exists: ${tableName}`,
  });
}

function transactionCanceled(cause: unknown): TransactionCanceledException {
  return new TransactionCanceledException({
    $metadata: {},
    message: cause instanceof Error ? cause.message : 'transaction canceled',
  });
}
