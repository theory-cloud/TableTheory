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
    };

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

    if (shape.indexType === 'GSI' && shape.consistentRead) {
      hints.push('ERROR: Consistent reads are not supported on GSIs');
    }

    if (shape.kind === 'query') {
      if (!shape.hasPartitionKey) {
        hints.push('ERROR: partitionKey() is not set (query will fail)');
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
        ...(shape.indexName ? { indexName: shape.indexName } : {}),
        ...(projections.length ? { projections } : {}),
        optimizationHints: hints,
      };
    }

    // Scan
    hints.push(
      'WARNING: Scan reads the full table/index; prefer Query when possible',
    );
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
      this.enableParallel && !shape.parallelScanConfigured
        ? Math.max(1, Math.min(this.maxParallelism, 16))
        : undefined;
    if (suggestedSegments && suggestedSegments > 1) {
      hints.push(
        `TIP: Use scanAllSegments(${suggestedSegments}) for faster large-table scans`,
      );
    }

    return {
      id: planId({ ...shape, projections }),
      operation: 'Scan',
      ...(shape.indexName ? { indexName: shape.indexName } : {}),
      ...(projections.length ? { projections } : {}),
      ...(suggestedSegments ? { parallelSegments: suggestedSegments } : {}),
      optimizationHints: hints,
    };
  }
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
