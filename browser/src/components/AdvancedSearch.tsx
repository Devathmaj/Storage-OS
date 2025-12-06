import React, { useState, useEffect } from 'react';
import { Search, Filter, X, Download, Info, Clock, HardDrive, FileIcon } from 'lucide-react';
import { useSearch } from '../hooks/useSearch';
import searchAPI, { FileMetadata, CacheStats } from '../services/searchAPI';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '../components/ui/card';
import { Input } from '../components/ui/input';
import { Button } from '../components/ui/button';
import { Badge } from '../components/ui/badge';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '../components/ui/tabs';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '../components/ui/select';
import { Separator } from '../components/ui/separator';
import { Alert, AlertDescription } from '../components/ui/alert';

export function AdvancedSearch() {
  const { results, isLoading, error, search, searchByName, searchByPath, searchByType, searchBySize, searchAdvanced } = useSearch();
  const [searchMode, setSearchMode] = useState<'simple' | 'advanced'>('simple');
  
  // Simple search fields
  const [nameQuery, setNameQuery] = useState('');
  const [pathQuery, setPathQuery] = useState('');
  const [typeFilter, setTypeFilter] = useState('');
  const [minSize, setMinSize] = useState('');
  const [maxSize, setMaxSize] = useState('');
  
  // Advanced search
  const [advancedQuery, setAdvancedQuery] = useState('');
  
  // Cache stats
  const [cacheStats, setCacheStats] = useState<CacheStats | null>(null);

  // Pagination
  const [currentPage, setCurrentPage] = useState(1);
  const [itemsPerPage] = useState(50);

  // Load cache stats
  useEffect(() => {
    loadCacheStats();
    const interval = setInterval(loadCacheStats, 5000);
    return () => clearInterval(interval);
  }, []);

  const loadCacheStats = async () => {
    try {
      const stats = await searchAPI.getCacheStats();
      setCacheStats(stats);
    } catch (err) {
      console.error('Failed to load cache stats:', err);
    }
  };

  const handleSimpleSearch = () => {
    if (nameQuery) {
      searchByName(nameQuery);
    } else if (pathQuery) {
      searchByPath(pathQuery);
    } else if (typeFilter) {
      searchByType(typeFilter);
    } else if (minSize || maxSize) {
      const min = minSize ? parseInt(minSize) * 1024 : undefined;
      const max = maxSize ? parseInt(maxSize) * 1024 : undefined;
      searchBySize(min, max);
    } else {
      search({});
    }
    setCurrentPage(1);
  };

  const handleAdvancedSearch = () => {
    if (advancedQuery.trim()) {
      searchAdvanced(advancedQuery);
      setCurrentPage(1);
    }
  };

  const handleClearFilters = () => {
    setNameQuery('');
    setPathQuery('');
    setTypeFilter('');
    setMinSize('');
    setMaxSize('');
    setAdvancedQuery('');
  };

  const formatFileSize = (bytes: number): string => {
    if (bytes === 0) return '0 B';
    const k = 1024;
    const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
  };

  const formatDate = (dateString: string): string => {
    return new Date(dateString).toLocaleString();
  };

  const getFileTypeColor = (type: string): string => {
    const colors: Record<string, string> = {
      'pdf': 'bg-red-100 text-red-800',
      'doc': 'bg-blue-100 text-blue-800',
      'docx': 'bg-blue-100 text-blue-800',
      'txt': 'bg-gray-100 text-gray-800',
      'jpg': 'bg-purple-100 text-purple-800',
      'jpeg': 'bg-purple-100 text-purple-800',
      'png': 'bg-purple-100 text-purple-800',
      'gif': 'bg-purple-100 text-purple-800',
      'mp4': 'bg-green-100 text-green-800',
      'avi': 'bg-green-100 text-green-800',
      'mp3': 'bg-yellow-100 text-yellow-800',
      'zip': 'bg-orange-100 text-orange-800',
      'rar': 'bg-orange-100 text-orange-800',
    };
    return colors[type.toLowerCase()] || 'bg-gray-100 text-gray-800';
  };

  // Paginated results
  const paginatedResults = results?.files.slice(
    (currentPage - 1) * itemsPerPage,
    currentPage * itemsPerPage
  ) || [];

  const totalPages = results ? Math.ceil(results.files.length / itemsPerPage) : 0;

  return (
    <div className="container mx-auto p-6 space-y-6">
      {/* Header with Cache Stats */}
      <div className="flex justify-between items-start">
        <div>
          <h1 className="text-3xl font-bold">File Search</h1>
          <p className="text-muted-foreground">Search through metadata cached in memory</p>
        </div>
        {cacheStats && (
          <Card className="w-64">
            <CardHeader className="pb-3">
              <CardTitle className="text-sm">Cache Status</CardTitle>
            </CardHeader>
            <CardContent className="space-y-2">
              <div className="flex justify-between text-sm">
                <span className="text-muted-foreground">Files:</span>
                <span className="font-medium">{cacheStats.file_count.toLocaleString()}</span>
              </div>
              <div className="flex justify-between text-sm">
                <span className="text-muted-foreground">Memory:</span>
                <span className="font-medium">{cacheStats.memory_pct.toFixed(1)}%</span>
              </div>
              <div className="flex justify-between text-sm">
                <span className="text-muted-foreground">Used:</span>
                <span className="font-medium">{formatFileSize(cacheStats.memory_used)}</span>
              </div>
              <div className="w-full bg-gray-200 rounded-full h-2">
                <div
                  className="bg-blue-600 h-2 rounded-full transition-all"
                  style={{ width: `${cacheStats.memory_pct}%` }}
                />
              </div>
            </CardContent>
          </Card>
        )}
      </div>

      {/* Search Interface */}
      <Card>
        <CardHeader>
          <CardTitle>Search Files</CardTitle>
          <CardDescription>
            Use simple filters or advanced query syntax for complex searches
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <Tabs value={searchMode} onValueChange={(v) => setSearchMode(v as 'simple' | 'advanced')}>
            <TabsList>
              <TabsTrigger value="simple">Simple Search</TabsTrigger>
              <TabsTrigger value="advanced">Advanced Query</TabsTrigger>
            </TabsList>

            <TabsContent value="simple" className="space-y-4">
              <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                <div>
                  <label className="text-sm font-medium">File Name</label>
                  <Input
                    placeholder="Enter file name..."
                    value={nameQuery}
                    onChange={(e) => setNameQuery(e.target.value)}
                    onKeyPress={(e) => e.key === 'Enter' && handleSimpleSearch()}
                  />
                </div>
                <div>
                  <label className="text-sm font-medium">Path</label>
                  <Input
                    placeholder="/path/to/files"
                    value={pathQuery}
                    onChange={(e) => setPathQuery(e.target.value)}
                    onKeyPress={(e) => e.key === 'Enter' && handleSimpleSearch()}
                  />
                </div>
                <div>
                  <label className="text-sm font-medium">File Type</label>
                  <Input
                    placeholder="pdf, jpg, txt..."
                    value={typeFilter}
                    onChange={(e) => setTypeFilter(e.target.value)}
                    onKeyPress={(e) => e.key === 'Enter' && handleSimpleSearch()}
                  />
                </div>
                <div className="grid grid-cols-2 gap-2">
                  <div>
                    <label className="text-sm font-medium">Min Size (KB)</label>
                    <Input
                      type="number"
                      placeholder="0"
                      value={minSize}
                      onChange={(e) => setMinSize(e.target.value)}
                      onKeyPress={(e) => e.key === 'Enter' && handleSimpleSearch()}
                    />
                  </div>
                  <div>
                    <label className="text-sm font-medium">Max Size (KB)</label>
                    <Input
                      type="number"
                      placeholder="∞"
                      value={maxSize}
                      onChange={(e) => setMaxSize(e.target.value)}
                      onKeyPress={(e) => e.key === 'Enter' && handleSimpleSearch()}
                    />
                  </div>
                </div>
              </div>
              <div className="flex gap-2">
                <Button onClick={handleSimpleSearch} disabled={isLoading}>
                  <Search className="w-4 h-4 mr-2" />
                  Search
                </Button>
                <Button variant="outline" onClick={handleClearFilters}>
                  <X className="w-4 h-4 mr-2" />
                  Clear
                </Button>
              </div>
            </TabsContent>

            <TabsContent value="advanced" className="space-y-4">
              <div>
                <label className="text-sm font-medium">Query String</label>
                <Input
                  placeholder='name~*.pdf AND size>1024000 OR type=image'
                  value={advancedQuery}
                  onChange={(e) => setAdvancedQuery(e.target.value)}
                  onKeyPress={(e) => e.key === 'Enter' && handleAdvancedSearch()}
                  className="font-mono"
                />
                <p className="text-xs text-muted-foreground mt-2">
                  Examples: <code>name=report.pdf</code>, <code>name~*.jpg AND size&gt;1000000</code>, 
                  <code>path=/documents OR path=/images</code>
                </p>
              </div>
              <div className="flex gap-2">
                <Button onClick={handleAdvancedSearch} disabled={isLoading}>
                  <Search className="w-4 h-4 mr-2" />
                  Search
                </Button>
                <Button variant="outline" onClick={() => setAdvancedQuery('')}>
                  <X className="w-4 h-4 mr-2" />
                  Clear
                </Button>
              </div>
            </TabsContent>
          </Tabs>
        </CardContent>
      </Card>

      {/* Results */}
      {error && (
        <Alert variant="destructive">
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}

      {isLoading && (
        <div className="flex justify-center py-8">
          <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-blue-600"></div>
        </div>
      )}

      {results && !isLoading && (
        <Card>
          <CardHeader>
            <div className="flex justify-between items-center">
              <div>
                <CardTitle>Search Results</CardTitle>
                <CardDescription>
                  Found {results.total_count} files in {results.query_time_ms}ms
                  {results.cache_hit && ' (cached)'}
                </CardDescription>
              </div>
              <div className="flex gap-2 items-center text-sm text-muted-foreground">
                <Clock className="w-4 h-4" />
                <span>{results.query_time_ms}ms</span>
                <Separator orientation="vertical" className="h-4" />
                <HardDrive className="w-4 h-4" />
                <span>{formatFileSize(results.memory_used)}</span>
              </div>
            </div>
          </CardHeader>
          <CardContent>
            <div className="space-y-2">
              {paginatedResults.map((file) => (
                <div
                  key={file.id}
                  className="flex items-center justify-between p-4 border rounded-lg hover:bg-accent transition-colors"
                >
                  <div className="flex items-start gap-4 flex-1">
                    <FileIcon className="w-8 h-8 text-blue-600 flex-shrink-0 mt-1" />
                    <div className="flex-1 min-w-0">
                      <div className="flex items-center gap-2 mb-1">
                        <h3 className="font-medium truncate">{file.name}</h3>
                        <Badge className={getFileTypeColor(file.type)} variant="outline">
                          {file.type}
                        </Badge>
                      </div>
                      <p className="text-sm text-muted-foreground truncate">{file.path}</p>
                      <div className="flex gap-4 mt-2 text-xs text-muted-foreground">
                        <span>Size: {formatFileSize(file.size)}</span>
                        <span>Modified: {formatDate(file.modified)}</span>
                        <span>OS: {file.os_id}</span>
                      </div>
                    </div>
                  </div>
                  <div className="flex gap-2 flex-shrink-0">
                    <Button
                      size="sm"
                      variant="outline"
                      onClick={() => window.open(searchAPI.getFileDownloadURL(file.id), '_blank')}
                    >
                      <Download className="w-4 h-4" />
                    </Button>
                    <Button
                      size="sm"
                      variant="outline"
                      onClick={async () => {
                        try {
                          const info = await searchAPI.getFileInfo(file.id);
                          alert(JSON.stringify(info, null, 2));
                        } catch (err) {
                          alert('Failed to get file info');
                        }
                      }}
                    >
                      <Info className="w-4 h-4" />
                    </Button>
                  </div>
                </div>
              ))}
            </div>

            {/* Pagination */}
            {totalPages > 1 && (
              <div className="flex justify-center items-center gap-2 mt-6">
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => setCurrentPage(p => Math.max(1, p - 1))}
                  disabled={currentPage === 1}
                >
                  Previous
                </Button>
                <span className="text-sm text-muted-foreground">
                  Page {currentPage} of {totalPages}
                </span>
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => setCurrentPage(p => Math.min(totalPages, p + 1))}
                  disabled={currentPage === totalPages}
                >
                  Next
                </Button>
              </div>
            )}
          </CardContent>
        </Card>
      )}
    </div>
  );
}

export default AdvancedSearch;
