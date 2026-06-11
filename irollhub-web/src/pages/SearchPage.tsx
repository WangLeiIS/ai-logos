import { useSearchParams } from 'react-router-dom';
import { PackageList } from '../components/package/PackageList';
import { api } from '../api/client';
import { useQuery } from '@tanstack/react-query';
import type { SearchResponse } from '../types';

export function SearchPage() {
  const [searchParams] = useSearchParams();
  const query = searchParams.get('q') || '';

  const { data, isLoading, error } = useQuery<SearchResponse>({
    queryKey: ['search', query],
    queryFn: () => api.search(query),
    enabled: query.length > 0,
  });

  const results = data?.data || [];

  return (
    <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-12">
      <h1 className="text-3xl font-bold text-primary mb-6">
        Search results for "{query}"
      </h1>
      {isLoading ? (
        <p className="text-secondary">Loading...</p>
      ) : error ? (
        <p className="text-red-500">Error searching packages</p>
      ) : results.length === 0 ? (
        <p className="text-secondary">No results found.</p>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
          {results.map(({ org, package: pkg }) => (
            <div key={`${org.id}-${pkg.id}`}>
              <PackageList org={org.name} packages={[pkg]} />
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
