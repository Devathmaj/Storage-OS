// API client for search functionality

export interface FileMetadata {
  id: string;
  name: string;
  path: string;
  size: number;
  type: string;
  created: string;
  modified: string;
  os_id: string;
}

export interface SearchQuery {
  name?: string;
  path?: string;
  type?: string;
  min_size?: number;
  max_size?: number;
  advanced?: string;
}

export interface SearchResult {
  files: FileMetadata[];
  total_count: number;
  query_time_ms: number;
  memory_used: number;
  cache_hit: boolean;
}

export interface CacheStats {
  memory_used: number;
  memory_limit: number;
  memory_pct: number;
  file_count: number;
  max_files: number;
  initialized: boolean;
}

class SearchAPI {
  private baseURL: string;

  constructor(baseURL: string = 'http://localhost:3000') {
    this.baseURL = baseURL;
  }

  /**
   * General search with multiple criteria
   */
  async search(query: SearchQuery): Promise<SearchResult> {
    const response = await fetch(`${this.baseURL}/api/search`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
      },
      body: JSON.stringify(query),
    });

    if (!response.ok) {
      throw new Error(`Search failed: ${response.statusText}`);
    }

    return response.json();
  }

  /**
   * Search by exact file name
   */
  async searchByName(name: string): Promise<SearchResult> {
    const response = await fetch(
      `${this.baseURL}/api/search/name?q=${encodeURIComponent(name)}`
    );

    if (!response.ok) {
      throw new Error(`Search by name failed: ${response.statusText}`);
    }

    return response.json();
  }

  /**
   * Search by path prefix
   */
  async searchByPath(path: string): Promise<SearchResult> {
    const response = await fetch(
      `${this.baseURL}/api/search/path?q=${encodeURIComponent(path)}`
    );

    if (!response.ok) {
      throw new Error(`Search by path failed: ${response.statusText}`);
    }

    return response.json();
  }

  /**
   * Search by file type
   */
  async searchByType(type: string): Promise<SearchResult> {
    const response = await fetch(
      `${this.baseURL}/api/search/type?q=${encodeURIComponent(type)}`
    );

    if (!response.ok) {
      throw new Error(`Search by type failed: ${response.statusText}`);
    }

    return response.json();
  }

  /**
   * Search by size range
   */
  async searchBySize(minSize?: number, maxSize?: number): Promise<SearchResult> {
    const params = new URLSearchParams();
    if (minSize !== undefined) params.append('min', minSize.toString());
    if (maxSize !== undefined) params.append('max', maxSize.toString());

    const response = await fetch(
      `${this.baseURL}/api/search/size?${params.toString()}`
    );

    if (!response.ok) {
      throw new Error(`Search by size failed: ${response.statusText}`);
    }

    return response.json();
  }

  /**
   * Advanced search with query string
   * Examples:
   *   "name~*.pdf AND size>1024000"
   *   "type=image OR type=video"
   *   "path=/documents AND size<500000"
   */
  async searchAdvanced(queryString: string): Promise<SearchResult> {
    const response = await fetch(`${this.baseURL}/api/search/advanced`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
      },
      body: JSON.stringify({ query: queryString }),
    });

    if (!response.ok) {
      throw new Error(`Advanced search failed: ${response.statusText}`);
    }

    return response.json();
  }

  /**
   * Get cache statistics
   */
  async getCacheStats(): Promise<CacheStats> {
    const response = await fetch(`${this.baseURL}/api/search/stats`);

    if (!response.ok) {
      throw new Error(`Get stats failed: ${response.statusText}`);
    }

    return response.json();
  }

  /**
   * Download a file
   */
  getFileDownloadURL(fileId: string): string {
    return `${this.baseURL}/api/files/${encodeURIComponent(fileId)}/download`;
  }

  /**
   * Get file info
   */
  async getFileInfo(fileId: string): Promise<FileMetadata> {
    const response = await fetch(
      `${this.baseURL}/api/files/${encodeURIComponent(fileId)}/info`
    );

    if (!response.ok) {
      throw new Error(`Get file info failed: ${response.statusText}`);
    }

    return response.json();
  }

  /**
   * Bulk fetch file metadata
   */
  async bulkFetchInfo(fileIds: string[]): Promise<Record<string, FileMetadata | { error: string }>> {
    const response = await fetch(`${this.baseURL}/api/files/bulk`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
      },
      body: JSON.stringify({ file_ids: fileIds }),
    });

    if (!response.ok) {
      throw new Error(`Bulk fetch failed: ${response.statusText}`);
    }

    return response.json();
  }

  /**
   * Clear cache
   */
  async clearCache(): Promise<void> {
    const response = await fetch(`${this.baseURL}/api/search/clear`, {
      method: 'POST',
    });

    if (!response.ok) {
      throw new Error(`Clear cache failed: ${response.statusText}`);
    }
  }
}

export const searchAPI = new SearchAPI();
export default searchAPI;
