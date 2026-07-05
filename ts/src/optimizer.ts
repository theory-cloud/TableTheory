export type QueryOperation = 'Query' | 'Scan';

export interface QueryPlan {
  id: string;
  operation: QueryOperation;
  indexName?: string;
  projections?: string[];
  parallelSegments?: number;
  optimizationHints: string[];
}

export interface OptimizationOptions {
  enableParallel?: boolean;
  maxParallelism?: number;
}

export interface OptimizerCondition {
  field: string;
  operator: string;
}

export interface OptimizerIndexShape {
  name?: string;
  type: 'PRIMARY' | 'GSI' | 'LSI';
  partition: string;
  sort?: string;
  projectionType?: 'ALL' | 'KEYS_ONLY' | 'INCLUDE';
}

export type BuilderShape =
  | {
      kind: 'query';
      modelName: string;
      tableName: string;
      indexName?: string;
      indexType?: 'GSI' | 'LSI';
      hasPartitionKey: boolean;
      hasSortKey: boolean;
      hasSortKeyCondition: boolean;
      hasFilters: boolean;
      projections?: string[];
      consistentRead: boolean;
      sort: 'ASC' | 'DESC';
      indexes?: OptimizerIndexShape[];
      conditions?: OptimizerCondition[];
    }
  | {
      kind: 'scan';
      modelName: string;
      tableName: string;
      indexName?: string;
      indexType?: 'GSI' | 'LSI';
      hasFilters: boolean;
      projections?: string[];
      consistentRead: boolean;
      parallelScanConfigured: boolean;
      totalSegments?: number;
      indexes?: OptimizerIndexShape[];
      conditions?: OptimizerCondition[];
    };

export interface RequiredKeys {
  partitionKey: string;
  sortKey: string;
  sortKeyOp: string;
}

export class QueryOptimizer {
  private readonly enableParallel: boolean;
  private readonly maxParallelism: number;

  constructor(opts: OptimizationOptions = {}) {
    this.enableParallel = opts.enableParallel ?? true;
    this.maxParallelism = Math.max(1, Math.floor(opts.maxParallelism ?? 4));
  }

  explain(shape: BuilderShape): QueryPlan {
    const projections = normalizeProjections(shape.projections);
    const hints: string[] = [];
    const selectedIndex = selectOptimalIndex(
      analyzeConditions(shape.conditions ?? []),
      shape.indexes ?? [],
    );
    const effectiveIndexName = shape.indexName ?? selectedIndex?.name;
    const effectiveIndexType = shape.indexType ?? selectedIndex?.type;

    if (effectiveIndexType === 'GSI' && shape.consistentRead) {
      hints.push('ERROR: Consistent reads are not supported on GSIs');
    }

    if (shape.kind === 'query') {
      if (!shape.hasPartitionKey && !selectedIndex) {
        hints.push('ERROR: partitionKey() is not set (query will fail)');
      }
      if (!shape.indexName && selectedIndex) {
        hints.push(indexSelectionHint(selectedIndex));
      }
      if (shape.hasSortKey && !shape.hasSortKeyCondition) {
        hints.push(
          'TIP: Add sortKey() condition for more efficient queries when possible',
        );
      }
      if (shape.hasFilters) {
        hints.push(
          'INFO: Filters are applied after retrieval; prefer key conditions when possible',
        );
      }
      if (projections.length === 0) {
        hints.push(
          'TIP: Use projection() to select only needed attributes and reduce transfer',
        );
      }

      return {
        id: planId({ ...shape, projections }),
        operation: 'Query',
        ...(effectiveIndexName ? { indexName: effectiveIndexName } : {}),
        ...(projections.length ? { projections } : {}),
        optimizationHints: hints,
      };
    }

    // Scan
    const operation: QueryOperation = selectedIndex ? 'Query' : 'Scan';
    if (selectedIndex) {
      hints.push(indexSelectionHint(selectedIndex));
      hints.push(
        'TIP: This scan shape has indexable conditions; prefer Query with key conditions when possible',
      );
    } else {
      hints.push(
        'WARNING: Scan reads the full table/index; prefer Query when possible',
      );
    }
    if (shape.hasFilters) {
      hints.push(
        'INFO: Filters are applied after retrieval; consider narrowing with keys or indexes',
      );
    }
    if (projections.length === 0) {
      hints.push(
        'TIP: Use projection() to select only needed attributes and reduce transfer',
      );
    }

    const suggestedSegments =
      operation === 'Scan' &&
      this.enableParallel &&
      !shape.parallelScanConfigured
        ? Math.max(1, Math.min(this.maxParallelism, 16))
        : undefined;
    if (suggestedSegments && suggestedSegments > 1) {
      hints.push(
        `TIP: Use scanAllSegments(${suggestedSegments}) for faster large-table scans`,
      );
    }

    return {
      id: planId({ ...shape, projections }),
      operation,
      ...(effectiveIndexName ? { indexName: effectiveIndexName } : {}),
      ...(projections.length ? { projections } : {}),
      ...(suggestedSegments ? { parallelSegments: suggestedSegments } : {}),
      optimizationHints: hints,
    };
  }
}

export function analyzeConditions(
  conditions: readonly OptimizerCondition[],
): RequiredKeys {
  const required: RequiredKeys = {
    partitionKey: '',
    sortKey: '',
    sortKeyOp: '',
  };
  const pendingSort: OptimizerCondition[] = [];

  for (const cond of conditions) {
    if (isEqualityOperator(cond.operator) && required.partitionKey === '') {
      required.partitionKey = cond.field;
    }

    if (required.partitionKey === '') {
      pendingSort.push(cond);
      continue;
    }

    if (required.sortKey === '' && cond.field !== required.partitionKey) {
      required.sortKey = cond.field;
      required.sortKeyOp = normalizeOperator(cond.operator);
    }
  }

  if (required.sortKey === '' && required.partitionKey !== '') {
    for (const cond of pendingSort) {
      if (cond.field === required.partitionKey) continue;
      required.sortKey = cond.field;
      required.sortKeyOp = normalizeOperator(cond.operator);
      break;
    }
  }

  return required;
}

export function selectOptimalIndex(
  required: RequiredKeys,
  indexes: readonly OptimizerIndexShape[],
): OptimizerIndexShape | undefined {
  if (!required.partitionKey) return undefined;

  let bestIndex: OptimizerIndexShape | undefined;
  let bestScore = 0;

  for (const idx of indexes) {
    const score = scoreIndex(idx, required);
    if (score > bestScore) {
      bestScore = score;
      bestIndex = idx;
    }
  }

  return bestScore > 0 ? bestIndex : undefined;
}

function normalizeProjections(projections?: string[]): string[] {
  if (!projections || projections.length === 0) return [];
  return projections.slice().sort();
}

function planId(shape: BuilderShape & { projections: string[] }): string {
  const parts = [
    shape.kind,
    shape.modelName,
    shape.tableName,
    `idx=${shape.indexName ?? ''}`,
    `it=${shape.indexType ?? ''}`,
    `proj=${shape.projections.join(',')}`,
    `cond=${conditionSignature(shape.conditions)}`,
    `filter=${shape.hasFilters ? '1' : '0'}`,
    `cr=${shape.consistentRead ? '1' : '0'}`,
  ];

  if (shape.kind === 'query') {
    parts.push(
      `pk=${shape.hasPartitionKey ? '1' : '0'}`,
      `sk=${shape.hasSortKey ? '1' : '0'}`,
      `skc=${shape.hasSortKeyCondition ? '1' : '0'}`,
      `sort=${shape.sort}`,
    );
  } else {
    parts.push(
      `psc=${shape.parallelScanConfigured ? '1' : '0'}`,
      `ts=${shape.totalSegments ?? ''}`,
    );
  }

  return parts.join('|');
}

function conditionSignature(
  conditions?: readonly OptimizerCondition[],
): string {
  return (conditions ?? [])
    .map(
      (condition) =>
        `${condition.field}:${normalizeOperator(condition.operator)}`,
    )
    .sort()
    .join(',');
}

function scoreIndex(idx: OptimizerIndexShape, required: RequiredKeys): number {
  if (idx.partition !== required.partitionKey) return 0;

  let score = 100;
  if (required.sortKey && idx.sort === required.sortKey) {
    if (required.sortKeyOp === '=') {
      score += 50;
    } else if (required.sortKeyOp === 'begins_with') {
      score += 40;
    } else if (
      required.sortKeyOp === 'between' ||
      required.sortKeyOp === '<' ||
      required.sortKeyOp === '<=' ||
      required.sortKeyOp === '>' ||
      required.sortKeyOp === '>='
    ) {
      score += 30;
    }
  }

  if (idx.type === 'GSI') score += 10;
  if (idx.projectionType === 'ALL') score += 5;

  return score;
}

function isEqualityOperator(op: string): boolean {
  const normalized = op.toUpperCase();
  return normalized === '=' || normalized === 'EQ';
}

function normalizeOperator(op: string): string {
  switch (op.toUpperCase()) {
    case 'EQ':
    case '=':
      return '=';
    case 'LT':
    case '<':
      return '<';
    case 'LE':
    case '<=':
      return '<=';
    case 'GT':
    case '>':
      return '>';
    case 'GE':
    case '>=':
      return '>=';
    case 'BEGINS_WITH':
      return 'begins_with';
    case 'BETWEEN':
      return 'between';
    default:
      return op.toLowerCase();
  }
}

function indexSelectionHint(index: OptimizerIndexShape): string {
  if (index.name) {
    return `INFO: Optimizer selected ${index.type} ${index.name} from key-compatible conditions`;
  }
  return 'INFO: Optimizer selected the table primary index from key-compatible conditions';
}
