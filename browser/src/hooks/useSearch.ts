import { useState, useCallback } from 'react';
import searchAPI, { SearchQuery, SearchResult, FileMetadata } from '../services/searchAPI';

export interface UseSearchReturn {
  results: SearchResult | null;
  isLoading: boolean;
  error: string | null;
  search: (query: SearchQuery) => Promise<void>;
  searchByName: (name: string) => Promise<void>;
  searchByPath: (path: string) => Promise<void>;
  searchByType: (type: string) => Promise<void>;
  searchBySize: (minSize?: number, maxSize?: number) => Promise<void>;
  searchAdvanced: (queryString: string) => Promise<void>;
  clearResults: () => void;
}

export function useSearch(): UseSearchReturn {
  const [results, setResults] = useState<SearchResult | null>(null);
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const search = useCallback(async (query: SearchQuery) => {
    setIsLoading(true);
    setError(null);
    try {
      const result = await searchAPI.search(query);
      setResults(result);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Search failed');
      setResults(null);
    } finally {
      setIsLoading(false);
    }
  }, []);

  const searchByName = useCallback(async (name: string) => {
    setIsLoading(true);
    setError(null);
    try {
      const result = await searchAPI.searchByName(name);
      setResults(result);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Search failed');
      setResults(null);
    } finally {
      setIsLoading(false);
    }
  }, []);

  const searchByPath = useCallback(async (path: string) => {
    setIsLoading(true);
    setError(null);
    try {
      const result = await searchAPI.searchByPath(path);
      setResults(result);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Search failed');
      setResults(null);
    } finally {
      setIsLoading(false);
    }
  }, []);

  const searchByType = useCallback(async (type: string) => {
    setIsLoading(true);
    setError(null);
    try {
      const result = await searchAPI.searchByType(type);
      setResults(result);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Search failed');
      setResults(null);
    } finally {
      setIsLoading(false);
    }
  }, []);

  const searchBySize = useCallback(async (minSize?: number, maxSize?: number) => {
    setIsLoading(true);
    setError(null);
    try {
      const result = await searchAPI.searchBySize(minSize, maxSize);
      setResults(result);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Search failed');
      setResults(null);
    } finally {
      setIsLoading(false);
    }
  }, []);

  const searchAdvanced = useCallback(async (queryString: string) => {
    setIsLoading(true);
    setError(null);
    try {
      const result = await searchAPI.searchAdvanced(queryString);
      setResults(result);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Search failed');
      setResults(null);
    } finally {
      setIsLoading(false);
    }
  }, []);

  const clearResults = useCallback(() => {
    setResults(null);
    setError(null);
  }, []);

  return {
    results,
    isLoading,
    error,
    search,
    searchByName,
    searchByPath,
    searchByType,
    searchBySize,
    searchAdvanced,
    clearResults,
  };
}
